package reports

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// Spec limits. A spec past one of them is refused before it is compiled.
const (
	maxColumns = 100
	// maxHops counts links; the final field is not a hop, so a path has at
	// most 4 links plus the field.
	maxHops         = 4
	maxSort         = 10
	maxGroups       = 6
	maxLayoutItems  = 200
	maxLabel        = 200
	maxFormat       = 32
	maxPivotColumns = 200
	defaultPivotMax = 50
)

var buckets = []string{"day", "month", "quarter", "year", "fiscalYear"}

// characters counts text as a reader does, one per character (code point),
// not per byte, as the client's spec schema counts it (lib/spec.ts).
func characters(s string) int { return utf8.RuneCountInString(s) }

// plan is a spec bound to the registry: every path resolved to its links
// and field, every column reference to its column.
type plan struct {
	// app is the app the spec was bound for: a later path bind refuses what
	// CheckSources refused for it. Set even for a dry compile, where the
	// compiler has no app.
	app    core.App
	spec   *Spec
	base   *Source
	cols   []*colPlan
	byID   map[string]*colPlan
	groups []*groupPlan
	pivot  *pivotPlan
	sort   []sortPlan
	layout []*LayoutItem
	// nest is the chain of nested child rows, outermost first.
	nest []*nestLevel
	// lookups holds each lookup node's decoded value (lookupOf).
	lookups map[*Node]decodedLookup
	// options binds each choice param whose options come from a source, by
	// param id.
	options map[string]*optionsPlan
	// derived marks the derived columns by id (derivedColumns).
	derived map[string]bool
	// percents binds each percentOfSection column by id (sectionPercents).
	percents map[string]*sectionPercent
}

// optionsPlan is a choice param's options source bound to the registry: its
// value, label and (optional) group paths on src.
type optionsPlan struct {
	def                 *ParamDef
	at                  string
	src                 *Source
	value, label, group *pathPlan
	// orderBy is the group order (GroupOrderLabel when the spec names none),
	// orderPath its bound path when it orders by path.
	orderBy   string
	orderPath *pathPlan
}

type hopPlan struct {
	link     *Link
	from, to *Source
	mode     string // "", "aggregate" or "nest"
	required bool
}

type pathPlan struct {
	hops  []hopPlan
	field *Field
	src   *Source // the source the field is on
	// target is the source a relation value points at: the linked source of
	// a path that ends at a to-one link, or the source whose id it reads.
	target *Source
	// endsAtLink is true for a path whose last step is a link.
	endsAtLink bool
}

// key names a path's links; equal keys share one join.
func hopsKey(hops []hopPlan) string {
	parts := make([]string, len(hops))
	for i, h := range hops {
		parts[i] = h.link.Key
		if h.required {
			parts[i] += "!"
		}
	}
	return strings.Join(parts, "/")
}

type colPlan struct {
	col   *Column
	at    string
	path  *pathPlan // nil for a formula column
	tm    int       // index of the path's to-many aggregate hop, or -1
	nest  int       // nest level, 1 for the first; 0 when not nested
	scale int
}

type groupPlan struct {
	group *Group
	col   *colPlan
	at    string
}

type pivotPlan struct {
	pivot *Pivot
	path  *pathPlan
	cell  *colPlan
	max   int
}

// sortPlan is a sort key: a column (col) or a formula; at is where its
// errors are reported.
type sortPlan struct {
	col     *colPlan
	formula *Node
	at      string
	desc    bool
}

// nestLevel is one level of nested child rows: the hops from its parent's
// source through the nest link, and the columns it shows.
type nestLevel struct {
	hops []hopPlan // every hop from the base through this level's nest link
	cols []*colPlan
}

// binder collects every problem, so the builder can mark every bad part at
// once. Unknown references win: a report that names something missing
// opens in an error state that names it.
type binder struct {
	// app's refused sources are unknown to this bind.
	app     core.App
	invalid map[string]validation.Error
	unknown map[string]validation.Error
}

func (b *binder) bad(at, message string) {
	if _, seen := b.invalid[at]; !seen {
		b.invalid[at] = validation.NewError("invalid", message)
	}
}

func (b *binder) missing(at, code, message string) { b.merge(unknownRef(at, code, message)) }

func (b *binder) err() *SpecError {
	if len(b.unknown) > 0 {
		return &SpecError{Status: 422, Problems: b.unknown}
	}
	if len(b.invalid) > 0 {
		return &SpecError{Status: 400, Problems: b.invalid}
	}
	return nil
}

func (b *binder) merge(err *SpecError) {
	if err == nil {
		return
	}
	target := b.invalid
	if err.Status == 422 {
		target = b.unknown
	}
	for k, v := range err.Problems {
		if _, seen := target[k]; !seen {
			target[k] = v
		}
	}
}

// Validate checks a loaded spec against the registry without running it.
// It returns nil, or a *SpecError: 422 when the spec names a source, link,
// field or column that does not exist, else 400 listing what is invalid.
func Validate(app core.App, spec *Spec) error {
	p, err := bind(app, spec)
	if err != nil {
		return err
	}
	return compileDry(p)
}

func bind(app core.App, spec *Spec) (*plan, *SpecError) {
	b := &binder{app: app, invalid: map[string]validation.Error{}, unknown: map[string]validation.Error{}}
	if spec.Version != CurrentVersion {
		b.bad("version", fmt.Sprintf("version must be %d; load the spec first", CurrentVersion))
	}
	base := lookupSource(app, spec.Source)
	if base == nil {
		b.missing("source", "unknown_source", fmt.Sprintf("the source %q does not exist", spec.Source))
		return nil, b.err()
	}
	p := &plan{app: app, spec: spec, base: base, byID: map[string]*colPlan{}, lookups: map[*Node]decodedLookup{}}
	b.merge(checkParamDefs(spec.Params))
	b.optionSources(p)
	b.columns(p)
	b.derivedColumns(p)
	b.lookups(p)
	b.formulaSizes(p)
	b.nestChain(p)
	b.nestLabelBlanks(p)
	b.sorts(p)
	b.groupsAndPivot(p)
	b.layout(p)
	b.sectionPercents(p)
	switch d := spec.Detail; {
	case d == nil:
	case d.Label == nil:
		b.bad("detail", "detail has a label")
	case spec.Pivot != nil:
		b.bad("detail.label", "a pivot report has no detail rows to label")
	}
	b.display(p)
	b.hiddenColumns(p)
	if err := b.err(); err != nil {
		return nil, err
	}
	return p, nil
}

// hiddenColumns refuses a hidden column the display arranges or fits, and a
// hidden pivot cell column: each would show a column the result leaves out.
func (b *binder) hiddenColumns(p *plan) {
	// As the first column labels the rows, each level's first column labels
	// its child rows (nestedRows), so it is shown.
	for _, lv := range p.nest {
		if len(lv.cols) > 0 && lv.cols[0].col.Hidden {
			b.bad(lv.cols[0].at+".hidden", "the first column of child rows labels the child rows, so it is shown")
		}
	}
	for i, c := range p.spec.Columns {
		if !c.Hidden {
			continue
		}
		if pv := p.spec.Pivot; pv != nil && pv.Cell == c.ID {
			b.bad(fmt.Sprintf("columns.%d.hidden", i), "a pivot's cell column shows as the pivot's columns, so it is not hidden")
		}
		d := p.spec.Display
		if d == nil {
			continue
		}
		if _, ok := d.Columns[c.ID]; ok {
			b.bad("display.columns."+c.ID, "a hidden column is never shown, so it has no fit")
		}
		for li, l := range d.Layouts {
			for ci, cell := range l.Cells {
				if cell.Col == c.ID {
					b.bad(fmt.Sprintf("display.layouts.%d.cells.%d.col", li, ci), "a hidden column is never shown, so no layout places it")
				}
			}
		}
	}
}

// optionSources binds each choice param's options source: its value, label,
// group and group order paths, to-one only.
func (b *binder) optionSources(p *plan) {
	p.options = map[string]*optionsPlan{}
	for i := range p.spec.Params {
		d := &p.spec.Params[i]
		if d.Type != ParamChoice || d.OptionsFrom == nil {
			continue
		}
		at := fmt.Sprintf("params.%d.optionsFrom", i)
		of := d.OptionsFrom
		src := lookupSource(b.app, of.Source)
		if src == nil {
			b.missing(at+".source", "unknown_source", fmt.Sprintf("the source %q does not exist", of.Source))
			continue
		}
		op := &optionsPlan{def: d, at: at, src: src, orderBy: GroupOrderLabel}
		op.value = b.path(src, of.Value, at+".value", pathValue, "")
		op.label = b.path(src, of.Label, at+".label", pathValue, "")
		if of.Group != nil {
			op.group = b.path(src, of.Group, at+".group", pathValue, "")
		}
		if op.group != nil && of.GroupOrder != nil {
			b.groupOrder(src, op, of.GroupOrder, at+".groupOrder")
		}
		if op.value != nil && op.label != nil {
			p.options[d.ID] = op
		}
	}
}

// groupOrder binds a grouped list's group order; checkGroupOrder has
// already checked its shape.
func (b *binder) groupOrder(src *Source, op *optionsPlan, o *GroupOrder, at string) {
	switch o.By {
	case GroupOrderDeclared:
		if op.group.field.Type != Select {
			b.bad(at, "declared order needs a select group field")
			return
		}
	case GroupOrderPath:
		if op.orderPath = b.path(src, o.Path, at+".path", pathValue, ""); op.orderPath == nil {
			return
		}
	case GroupOrderLabel:
	default:
		return
	}
	op.orderBy = o.By
}

func (b *binder) columns(p *plan) {
	cols := p.spec.Columns
	if len(cols) == 0 || len(cols) > maxColumns {
		b.bad("columns", fmt.Sprintf("a report has 1 to %d columns", maxColumns))
	}
	for i := range cols {
		c := &cols[i]
		at := fmt.Sprintf("columns.%d", i)
		cp := &colPlan{col: c, at: at, tm: -1}
		switch {
		case !idRe.MatchString(c.ID):
			b.bad(at+".id", "a column needs an id of letters, digits and _")
		case p.byID[c.ID] != nil:
			b.bad(at+".id", fmt.Sprintf("the id %q is used twice", c.ID))
		default:
			p.byID[c.ID] = cp
		}
		if characters(c.Label) > maxLabel || characters(c.Format) > maxFormat {
			b.bad(at, fmt.Sprintf("a label has at most %d characters and a format %d", maxLabel, maxFormat))
		}
		if c.Total != "" && c.Total != TotalRecompute && c.Total != TotalRows {
			b.bad(at+".total", "total is recompute or rows")
		}
		b.detailBlank(p.spec, i)
		if c.Hidden && i == 0 {
			b.bad(at+".hidden", "the first column labels the rows, so it is shown")
		}
		if (c.Path == nil) == (c.Formula == nil) {
			b.bad(at, "a column has a path or a formula")
		}
		p.cols = append(p.cols, cp)
		if c.Path != nil {
			b.pathColumn(p, cp)
		} else if c.Formula != nil && c.Aggregate != "" {
			b.scaleOf(cp, c.Aggregate)
		}
		if c.Rollup != nil {
			switch {
			case c.Total == TotalRows:
				b.bad(at+".rollup", `a rollup replaces the column's totals: it does not go with total "rows"`)
			case cp.nest > 0:
				b.bad(at+".rollup", "a child row column has no totals to replace")
			}
		}
	}
}

// detailBlank refuses a column blank on detail rows where its value shows
// nowhere else (a report with no groups), where there are no detail rows (a
// pivot report) or where it names the detail rows: a blank label would leave
// a row with no name.
func (b *binder) detailBlank(spec *Spec, i int) {
	c := &spec.Columns[i]
	at := fmt.Sprintf("columns.%d.detail", i)
	switch {
	case c.Detail == "":
	case c.Detail != DetailBlank:
		b.bad(at, `detail is "blank"`)
	case len(spec.Groups) == 0:
		b.bad(at, "a column can be blank on detail rows only in a report with groups")
	case spec.Pivot != nil:
		b.bad(at, "a pivot report has no detail rows")
	case c.ID != "" && c.ID == detailLabelColumn(spec):
		b.bad(at, "this column labels the detail rows, so it cannot be blank on them")
	}
}

// nestLabelBlanks refuses a blank on the first column of a nest level: that
// column labels the level's child rows (nest.go), so a blank changes nothing.
// It runs after nestChain, which records the levels.
func (b *binder) nestLabelBlanks(p *plan) {
	for _, level := range p.nest {
		if len(level.cols) == 0 {
			continue
		}
		if first := level.cols[0]; first.col.Detail == DetailBlank {
			b.bad(first.at+".detail", "this column labels the child rows, so it cannot be blank on them")
		}
	}
}

// detailLabelColumn is the column whose cell labels each detail row: the first
// column, or the one a detail label of exactly {col} repeats. "" for none.
func detailLabelColumn(spec *Spec) string {
	if d := spec.Detail; d != nil && d.Label != nil {
		return repeatedColumn(d.Label)
	}
	if len(spec.Columns) == 0 {
		return ""
	}
	return spec.Columns[0].ID
}

func (b *binder) pathColumn(p *plan, cp *colPlan) {
	c := cp.col
	path := b.path(p.base, c.Path, cp.at+".path", pathColumn, c.Aggregate)
	if path == nil {
		return
	}
	cp.path = path
	for i, h := range path.hops {
		switch h.mode {
		case "aggregate":
			cp.tm = i
		case "nest":
			cp.nest++
		}
	}
	at := cp.at + ".aggregate"
	switch {
	case c.Aggregate == "":
		if cp.tm >= 0 {
			b.bad(at, "a total across a to-many link needs an aggregate")
		}
	case cp.nest > 0:
		b.bad(at, "child rows are not totalled; add the link as a total instead")
	case !path.field.allows(c.Aggregate):
		b.bad(at, fmt.Sprintf("%s does not offer %s", path.field.Label, c.Aggregate))
	case cp.tm >= 0 && c.Aggregate == CountDistinct:
		b.bad(at, "a distinct count across a to-many link cannot be totalled; count the linked records")
	case cp.tm >= 0 && path.field.Currency != "":
		b.bad(at, "a total across a to-many link would add amounts of different currencies")
	default:
		b.scaleOf(cp, c.Aggregate)
	}
}

// formulaSize is a formula's nodes and levels as the compiler builds it:
// every formula column it reads is compiled into it, so a column read twice
// counts twice.
type formulaSize struct{ nodes, depth int }

// formulaSizes refuses a formula column, a filter or a group label that
// passes the formula limits once the formula columns it reads are inlined.
// Each column's size is counted once, so a chain of columns that each read
// the one before twice is refused without compiling it.
func (b *binder) formulaSizes(p *plan) {
	memo := map[string]formulaSize{}
	check := func(at string, sz formulaSize) {
		if sz.nodes > maxFormulaNodes || sz.depth > maxFormulaDepth {
			b.bad(at, fmt.Sprintf("with the formula columns it reads, a formula has at most %d levels and %d nodes", maxFormulaDepth, maxFormulaNodes))
		}
	}
	for _, cp := range p.cols {
		if cp.col.Formula != nil {
			check(cp.at, p.columnSize(cp, memo, map[string]bool{}))
		}
		if r := cp.col.Rollup; r != nil {
			check(cp.at+".rollup", p.nodeSize(r, memo, map[string]bool{}, true))
		}
	}
	if f := p.spec.Filter; f != nil {
		check("filter", p.nodeSize(f, memo, map[string]bool{}, true))
	}
	for i := range p.spec.Groups {
		if l := p.spec.Groups[i].Label; l != nil {
			check(fmt.Sprintf("groups.%d.label", i), p.nodeSize(l, memo, map[string]bool{}, true))
		}
		if f := p.spec.Groups[i].Fill; f != nil && f.Where != nil {
			check(fmt.Sprintf("groups.%d.fill.where", i), p.nodeSize(f.Where, memo, map[string]bool{}, true))
		}
		if h := p.spec.Groups[i].Having; h != nil {
			check(fmt.Sprintf("groups.%d.having", i), p.nodeSize(h, memo, map[string]bool{}, true))
		}
		if o := p.spec.Groups[i].Opening; o != nil {
			for _, id := range o.cellIDs() {
				if n := o.Cells[id]; n != nil {
					check(fmt.Sprintf("groups.%d.opening.cells.%s", i, id), p.nodeSize(n, memo, map[string]bool{}, true))
				}
			}
		}
	}
	for i, s := range p.spec.Sort {
		if s.Formula != nil {
			check(fmt.Sprintf("sort.%d.formula", i), p.nodeSize(s.Formula, memo, map[string]bool{}, true))
		}
	}
	if d := p.spec.Detail; d != nil && d.Label != nil {
		check("detail.label", p.nodeSize(d.Label, memo, map[string]bool{}, true))
	}
}

// columnSize is a column's inlined size: 1 for a path column.
func (p *plan) columnSize(cp *colPlan, memo map[string]formulaSize, visiting map[string]bool) formulaSize {
	id := cp.col.ID
	if cp.col.Formula == nil || visiting[id] {
		// A column that reads itself is refused when it compiles.
		return formulaSize{nodes: 1}
	}
	if sz, ok := memo[id]; ok {
		return sz
	}
	visiting[id] = true
	sz := p.nodeSize(cp.col.Formula, memo, visiting, true)
	delete(visiting, id)
	memo[id] = sz
	return sz
}

// nodeSize counts a node's tree. The count stops just past the limit, so a
// long doubling chain never overflows into a small number.
func (p *plan) nodeSize(n *Node, memo map[string]formulaSize, visiting map[string]bool, inline bool) formulaSize {
	if cp := p.byID[n.Col]; cp != nil && inline {
		return p.columnSize(cp, memo, visiting)
	}
	sz := formulaSize{nodes: 1}
	add := func(child *Node, inline bool) {
		if child == nil {
			return
		}
		c := p.nodeSize(child, memo, visiting, inline)
		sz.nodes = min(sz.nodes+c.nodes, maxFormulaNodes+1)
		sz.depth = max(sz.depth, c.depth+1)
	}
	for _, a := range n.Args {
		add(a, inline)
	}
	// A lookup's where and value read report columns through the row the
	// lookup sits on, and so does an exists inside a lookup's where, so the
	// columns any where reads count inlined. (An exists in the filter is
	// refused a report column when it compiles.)
	add(n.Where, inline)
	if n.Op == "lookup" {
		// bind has refused a value that does not decode.
		if v, err := p.lookupOf(n, ""); err == nil {
			add(v, inline)
		}
	}
	return sz
}

func (b *binder) scaleOf(cp *colPlan, agg Aggregate) {
	scale := cp.col.Scale
	if agg != Avg {
		if scale != nil {
			b.bad(cp.at+".scale", "only an avg column has a scale")
		}
		return
	}
	if scale == nil || *scale < 0 || *scale > MaxScale {
		b.bad(cp.at+".scale", fmt.Sprintf("an avg column needs a scale from 0 to %d", MaxScale))
		return
	}
	cp.scale = *scale
}

type pathKind int

const (
	pathColumn pathKind = iota // a column: to-many hops need a mode
	pathValue                  // a filter, formula or pivot value: to-one only
	pathLinks                  // an exists link: links only, to-many allowed
)

// path binds a path to the registry. It returns nil when the path is broken,
// after recording why.
func (b *binder) path(src *Source, path Path, at string, kind pathKind, agg Aggregate) *pathPlan {
	if len(path) == 0 {
		b.bad(at, "a path has at least one step")
		return nil
	}
	out := &pathPlan{}
	links, modes := 0, map[string]bool{}
	for j, hop := range path {
		hat := fmt.Sprintf("%s.%d", at, j)
		last := j == len(path)-1
		if (hop.Link == "") == (hop.Field == "") {
			b.bad(hat, "a step is a link or a field")
			return nil
		}
		if hop.Field != "" {
			if !last || kind == pathLinks {
				b.bad(hat, "a field is the last step of a path; an exists path has links only")
				return nil
			}
			out.field, out.src = src.field(hop.Field), src
			if out.field == nil {
				b.missing(hat+".field", "unknown_field", fmt.Sprintf("the source %q has no field %q", src.Key, hop.Field))
				return nil
			}
			if hop.Mode != "" || hop.With != "" {
				b.bad(hat, "mode and with belong to a link step")
				return nil
			}
			if out.field.Key == idField.Key {
				out.target = src
			}
			return out
		}
		links++
		if links > maxHops {
			b.bad(hat, fmt.Sprintf("a path follows at most %d links", maxHops))
			return nil
		}
		link := src.link(hop.Link)
		if link == nil {
			b.missing(hat+".link", "unknown_link", fmt.Sprintf("the source %q has no link %q", src.Key, hop.Link))
			return nil
		}
		target := lookupSource(b.app, link.Target)
		if target == nil {
			b.missing(hat+".link", "unknown_source", fmt.Sprintf("the link %q leads to the source %q, which does not exist", hop.Link, link.Target))
			return nil
		}
		if hop.With != "" && hop.With != "required" && hop.With != "optional" {
			b.bad(hat+".with", "with is required or optional")
			return nil
		}
		if kind == pathLinks && hop.With != "" {
			b.bad(hat, "with does not apply to an exists link")
			return nil
		}
		h := hopPlan{link: link, from: src, to: target, mode: hop.Mode, required: hop.With == "required"}
		if msg := hopProblem(h, kind, agg, modes); msg != "" {
			b.bad(hat, msg)
			return nil
		}
		if h.mode != "" {
			modes[h.mode] = true
		}
		out.hops = append(out.hops, h)
		src = target
	}
	if kind != pathLinks {
		last := out.hops[len(out.hops)-1]
		if last.link.Cardinality != ToOne {
			b.bad(at, "a path ends with a field, or with a link to one row")
			return nil
		}
		// A path that ends at a to-one link reads the linked row's id through
		// the join: a relation to the target, blank when the reader cannot
		// see that row.
		out.field, out.src, out.target, out.endsAtLink = src.field(idField.Key), src, src, true
		return out
	}
	out.src = src
	return out
}

func hopProblem(h hopPlan, kind pathKind, agg Aggregate, seen map[string]bool) string {
	toMany := h.link.Cardinality == ToMany
	switch {
	case h.mode != "" && kind != pathColumn:
		return "mode belongs to a column's to-many link"
	case h.mode != "" && !toMany:
		return "mode is for a to-many link"
	case h.mode != "" && h.mode != "aggregate" && h.mode != "nest":
		return "mode is aggregate or nest"
	case h.mode == "nest" && h.required:
		return "child rows show the linked rows there are; with does not apply"
	case !toMany:
		return ""
	case kind == pathValue:
		return "a value reads a to-many link only through exists or a total column"
	case kind == pathLinks:
		return ""
	case h.mode == "" && agg != "":
		return fmt.Sprintf("a %s across a to-many link would double-count: add the link as a total (mode aggregate)", agg)
	case h.mode == "":
		return "a to-many link needs mode aggregate or nest"
	case seen["aggregate"]:
		return "a total crosses one to-many link"
	case h.mode == "aggregate" && seen["nest"], h.mode == "nest" && seen["aggregate"]:
		return "a path either totals or nests a to-many link, not both"
	}
	return ""
}

// nestChain checks that every nested column lies on one chain of nest
// links, and records the chain.
func (b *binder) nestChain(p *plan) {
	var longest []hopPlan
	var longestCol *colPlan
	for _, cp := range p.cols {
		if cp.nest == 0 || cp.path == nil {
			continue
		}
		prefix := nestPrefix(cp.path.hops)
		short, long := prefix, longest
		if len(short) > len(long) {
			short, long = long, short
		}
		if hopsKey(long[:len(short)]) != hopsKey(short) || !sameNests(long[:len(short)], short) {
			b.bad(cp.at+".path", fmt.Sprintf("a report nests one chain of child rows; this column leaves the chain of %q", longestCol.col.ID))
			continue
		}
		if len(prefix) > len(longest) {
			longest, longestCol = prefix, cp
		}
	}
	for i, h := range longest {
		if h.mode == "nest" {
			p.nest = append(p.nest, &nestLevel{hops: longest[:i+1]})
		}
	}
	for _, cp := range p.cols {
		if cp.nest > 0 && cp.nest <= len(p.nest) {
			p.nest[cp.nest-1].cols = append(p.nest[cp.nest-1].cols, cp)
		}
	}
}

// nestPrefix is the hops up to and including the last nest hop.
func nestPrefix(hops []hopPlan) []hopPlan {
	last := -1
	for i, h := range hops {
		if h.mode == "nest" {
			last = i
		}
	}
	return hops[:last+1]
}

func sameNests(a, b []hopPlan) bool {
	for i := range a {
		if a[i].mode != b[i].mode {
			return false
		}
	}
	return true
}

func (b *binder) column(p *plan, id, at string) *colPlan {
	cp := p.byID[id]
	if cp == nil {
		b.merge(unknownColumn(at, id))
	}
	return cp
}

func (b *binder) sorts(p *plan) {
	if len(p.spec.Sort) > maxSort {
		b.bad("sort", fmt.Sprintf("a report sorts by at most %d columns", maxSort))
	}
	for i, s := range p.spec.Sort {
		at := fmt.Sprintf("sort.%d", i)
		if s.Dir != "" && s.Dir != "asc" && s.Dir != "desc" {
			b.bad(at+".dir", "dir is asc or desc")
		}
		desc := s.Dir == "desc"
		if (s.Col == "") == (s.Formula == nil) {
			b.bad(at, "a sort key is a column or a formula")
			continue
		}
		if s.Formula != nil {
			p.sort = append(p.sort, sortPlan{formula: s.Formula, at: at + ".formula", desc: desc})
			continue
		}
		cp := b.column(p, s.Col, at+".col")
		if cp == nil {
			continue
		}
		if cp.nest > 0 {
			b.bad(at+".col", "child rows keep their own order")
			continue
		}
		p.sort = append(p.sort, sortPlan{col: cp, at: at + ".col", desc: desc})
	}
}

func (b *binder) groupsAndPivot(p *plan) {
	spec := p.spec
	if len(spec.Groups) > maxGroups {
		b.bad("groups", fmt.Sprintf("a report has at most %d group levels", maxGroups))
	}
	seen := map[string]bool{}
	groupedRollup := map[*colPlan]bool{}
	for i := range spec.Groups {
		g := &spec.Groups[i]
		at := fmt.Sprintf("groups.%d", i)
		if !idRe.MatchString(g.ID) || seen[g.ID] {
			b.bad(at+".id", "a group needs a unique id of letters, digits and _")
		}
		seen[g.ID] = true
		cp := b.column(p, g.Col, at+".col")
		if cp == nil {
			continue
		}
		if cp.nest > 0 {
			b.bad(at+".col", "a report does not group by child rows")
			continue
		}
		if g.Bucket != "" {
			if !slices.Contains(buckets, g.Bucket) {
				b.bad(at+".bucket", fmt.Sprintf("bucket is one of %v", buckets))
			} else if cp.path != nil && cp.path.field.Type != Date && cp.path.field.Type != DateTime {
				b.bad(at+".bucket", "only a date column has a bucket")
			}
		}
		switch {
		case g.Blank == "":
		case g.Blank != GroupBlankSkip:
			b.bad(at+".blank", `blank is "skip"`)
		case i == len(spec.Groups)-1:
			b.bad(at+".blank", "the last level has no level below for its rows to pass to")
		case i == 0:
			b.bad(at+".blank", "the first level has no group above for its rows to sit under")
		case g.Bucket != "":
			b.bad(at+".blank", "a bucketed level has no blank key to skip")
		}
		if g.HideEmpty && i == len(spec.Groups)-1 {
			b.bad(at+".hideEmpty", "the last level has no child groups to be empty of")
		}
		// One refusal per column, however many levels group by it.
		if g.Bucket == "" && cp.col.Rollup != nil && !groupedRollup[cp] {
			groupedRollup[cp] = true
			b.bad(cp.at+".rollup", "a column a group is grouped by cannot have a rollup")
		}
		p.groups = append(p.groups, &groupPlan{group: g, col: cp, at: at})
		if g.Opening != nil {
			b.opening(p, g.Opening, at+".opening")
		}
	}
	pv := spec.Pivot
	if pv == nil {
		return
	}
	if len(spec.Groups) == 0 {
		b.bad("pivot", "a pivot needs at least one group")
	}
	pp := &pivotPlan{pivot: pv, max: pv.MaxColumns}
	if pp.max == 0 {
		pp.max = defaultPivotMax
	}
	if pp.max < 1 || pp.max > maxPivotColumns {
		b.bad("pivot.maxColumns", fmt.Sprintf("maxColumns is 1 to %d", maxPivotColumns))
	}
	pp.path = b.path(p.base, pv.Path, "pivot.path", pathValue, "")
	if pp.path != nil && pv.LabelField != "" {
		if pp.path.target == nil {
			b.bad("pivot.labelField", "only a pivot over a link to a record shows a label field")
		} else {
			b.merge(checkLabel(pp.path.target, pv.LabelField, "pivot.labelField"))
		}
	}
	if pp.path != nil && pv.Bucket != "" {
		if !slices.Contains(buckets, pv.Bucket) {
			b.bad("pivot.bucket", fmt.Sprintf("bucket is one of %v", buckets))
		} else if t := pp.path.field.Type; t != Date && t != DateTime {
			b.bad("pivot.bucket", "only a date has a bucket")
		}
	}
	if pv.Range != "" {
		b.pivotRange(p, pp)
	}
	pp.cell = b.column(p, pv.Cell, "pivot.cell")
	if pp.cell != nil && (pp.cell.col.Aggregate == "" || pp.cell.tm >= 0) {
		b.bad("pivot.cell", "the pivot cell is a column with an aggregate, not a total across a to-many link")
	}
	if pp.cell != nil && pp.cell.col.Rollup != nil {
		b.bad(pp.cell.at+".rollup", "a pivot's cell column is totalled by its pivot columns")
	}
	switch {
	case pv.Order == "":
	case pv.Order != PivotOrderSpec:
		b.bad("pivot.order", `order is "spec"`)
	case len(spec.Columns) > 0 && spec.Columns[0].ID == pv.Cell:
		b.bad("pivot.order", "the pivot cell is the first column, the label: no pivot column may come before it")
	}
	p.pivot = pp
	// A pivot's innermost level never opens (its groups are the pivot's rows),
	// so an opening row there could never show (R5e).
	if last := len(spec.Groups) - 1; last >= 0 && spec.Groups[last].Opening != nil {
		b.bad(fmt.Sprintf("groups.%d.opening", last), "this level never opens, so it has no opening row")
	}
}

// pivotRange refuses a range that names no date range param, or a pivot
// whose columns cannot be the range's buckets: one not over a date, or with
// no bucket.
func (b *binder) pivotRange(p *plan, pp *pivotPlan) {
	pv := pp.pivot
	i := slices.IndexFunc(p.spec.Params, func(d ParamDef) bool { return d.ID == pv.Range })
	switch {
	case i < 0:
		b.missing("pivot.range", "unknown_param", fmt.Sprintf("the report has no param %q", pv.Range))
	case p.spec.Params[i].Type != ParamDateRange:
		b.bad("pivot.range", fmt.Sprintf("range is the id of a date range param, but %q is a %s param", pv.Range, p.spec.Params[i].Type))
	case pp.path != nil && pp.path.field.Type != Date && pp.path.field.Type != DateTime:
		b.bad("pivot.range", "only a pivot over a date has a range")
	case pv.Bucket == "":
		b.bad("pivot.range", "a range pivot needs a bucket: its columns are the range's days, months, quarters or years")
	}
}

// opening checks a group's opening row: a label of 1 to maxLabel characters,
// and cells only in the report's columns after the first, whose cell the
// label fills, and not in a child row column. A cell's type and currency are
// checked where it compiles, on its level (level.openingCells).
func (b *binder) opening(p *plan, op *Opening, at string) {
	if n := characters(op.Label); n < 1 || n > maxLabel {
		b.bad(at+".label", fmt.Sprintf("an opening row has a label of 1 to %d characters", maxLabel))
	}
	for _, id := range op.cellIDs() {
		cellAt := at + ".cells." + id
		switch cp := b.column(p, id, cellAt); {
		case cp == nil:
		case cp == p.cols[0]:
			b.bad(cellAt, "the opening row's label fills the first column")
		case cp.nest > 0:
			b.bad(cellAt, "a child row column has no opening cell")
		case cp.col.Hidden:
			b.bad(cellAt, "a hidden column is never shown, so its opening cell would not show")
		}
	}
}

// layoutSectionKey refuses a section key that is not a scalar, or that
// another section already shows.
func (b *binder) layoutSectionKey(at string, item *LayoutItem, seen map[string]bool) {
	key, err := layoutKey(item.Key)
	if err != nil {
		b.bad(at, errLayoutKey)
		return
	}
	id := keyString([]any{key})
	if seen[id] {
		b.bad(at, "two sections show the same group")
	}
	seen[id] = true
}

// layoutCells refuses a computed row's cells on a column it cannot fill: an
// unknown one, the first (the row's label is there) or a pivot's cell column
// (it shows as the pivot's columns, which the row's formula fills).
func (b *binder) layoutCells(p *plan, at string, item *LayoutItem) {
	for _, id := range item.cellIDs() {
		cellAt := at + ".cells." + id
		cp := b.column(p, id, cellAt)
		switch {
		case cp == nil:
		case len(p.cols) > 0 && cp == p.cols[0]:
			b.bad(cellAt, "the row's label fills the first column")
		case p.spec.Pivot != nil && p.spec.Pivot.Cell == id:
			b.bad(cellAt, "a pivot's cell column is filled by the row's formula")
		}
	}
}

// derivedColumns marks each derived column: a formula column with no
// aggregate and no rollup that reads only columns, params, values and scalar
// operators. On a computed row its cell is its formula over the row's own
// cells (G8).
func (b *binder) derivedColumns(p *plan) {
	p.derived = map[string]bool{}
	for _, cp := range p.cols {
		c := cp.col
		if c.Formula != nil && c.Aggregate == "" && c.Rollup == nil && scalarOnly(c.Formula) {
			p.derived[c.ID] = true
		}
	}
}

// scalarOnly is true for a formula of columns, params, values and scalar
// operators.
func scalarOnly(n *Node) bool {
	if n == nil {
		return true
	}
	if n.Path != nil || n.Record != "" || n.Subtotal != "" || n.Outer != nil || n.Link != nil || n.Where != nil || n.Source != "" {
		return false
	}
	switch {
	case n.Op == "lookup", n.Op == "exists", n.Op == "notExists", aggregateOp(n), windowOp(n):
		return false
	}
	for _, a := range n.Args {
		if !scalarOnly(a) {
			return false
		}
	}
	return true
}

func (b *binder) layout(p *plan) {
	items := p.spec.Layout
	if len(items) == 0 {
		return
	}
	if len(items) > maxLayoutItems {
		b.bad("layout", fmt.Sprintf("a layout has at most %d items", maxLayoutItems))
	}
	if len(p.spec.Groups) == 0 {
		b.bad("layout", "a layout selects rows by the first group, so it needs a group")
	}
	if len(p.spec.Groups) > 0 && p.spec.Groups[0].Having != nil {
		b.bad("groups.0.having", "a layout's sections are fixed rows: hide rows on a deeper group")
	}
	// The last-level refusal (groupsAndPivot) already names a one-level spec.
	if len(p.spec.Groups) > 1 && p.spec.Groups[0].HideEmpty {
		b.bad("groups.0.hideEmpty", "a layout's sections are fixed rows: hide empty groups on a deeper group")
	}
	seen := map[string]bool{}
	keys := map[string]bool{}
	sections := map[string]int{}
	for i := range items {
		item := &items[i]
		at := fmt.Sprintf("layout.%d", i)
		id, isSection := item.Section, item.Section != ""
		if !isSection {
			id = item.Row
		}
		switch {
		case isSection == (item.Row != ""):
			b.bad(at, "a layout item is a section or a computed row")
			continue
		case !idRe.MatchString(id) || seen[id]:
			b.bad(at, "a layout item needs a unique id of letters, digits and _")
		case isSection && (item.Key == nil || item.Formula != nil || item.Label != "" || item.Within != "" || len(item.Cells) > 0):
			b.bad(at, "a section has a title and the key of a first-level group")
		case !isSection && ((item.Formula == nil && len(item.Cells) == 0) || item.Key != nil || item.Title != ""):
			b.bad(at, "a computed row has a label and a formula or cells")
		case characters(item.Title) > maxLabel || characters(item.Label) > maxLabel:
			b.bad(at, fmt.Sprintf("a title or label has at most %d characters", maxLabel))
		}
		if isSection && item.Key != nil {
			b.layoutSectionKey(at+".key", item, keys)
		}
		if !isSection {
			b.layoutCells(p, at, item)
		}
		if !isSection && item.Within != "" {
			at := at + ".within"
			switch idx, ok := sections[item.Within]; {
			case ok && idx < i:
			case ok || seen[item.Within] || slices.ContainsFunc(items, func(o LayoutItem) bool { return o.Section == item.Within || o.Row == item.Within }):
				b.bad(at, "within names a section earlier in the layout")
			default:
				b.missing(at, "unknown_section", fmt.Sprintf("the layout has no section %q", item.Within))
			}
		}
		if isSection {
			if _, ok := sections[id]; !ok {
				sections[id] = i
			}
		}
		seen[id] = true
		p.layout = append(p.layout, item)
	}
}
