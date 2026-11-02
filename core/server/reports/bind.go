package reports

import (
	"errors"
	"fmt"
	"slices"
	"strings"

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
}

// optionsPlan is a choice param's options source bound to the registry: its
// value, label and (optional) group paths on src.
type optionsPlan struct {
	def                 *ParamDef
	at                  string
	src                 *Source
	value, label, group *pathPlan
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
	b.lookups(p)
	b.formulaSizes(p)
	b.nestChain(p)
	b.sorts(p)
	b.groupsAndPivot(p)
	b.layout(p)
	switch d := spec.Detail; {
	case d == nil:
	case d.Label == nil:
		b.bad("detail", "detail has a label")
	case spec.Pivot != nil:
		b.bad("detail.label", "a pivot report has no detail rows to label")
	}
	b.display(p)
	if err := b.err(); err != nil {
		return nil, err
	}
	return p, nil
}

const (
	maxDisplayTracks = 12
	maxDisplayLines  = 8
)

var displayWidths = []string{"mobile", "tablet", "desktop"}

func (b *binder) display(p *plan) {
	d := p.spec.Display
	if d == nil {
		return
	}
	if len(d.Layouts) > len(displayWidths) {
		b.bad("display.layouts", fmt.Sprintf("a report has at most %d display layouts, one per width", len(displayWidths)))
		return
	}
	seen := map[string]bool{}
	for i, l := range d.Layouts {
		at := fmt.Sprintf("display.layouts.%d", i)
		switch {
		case !slices.Contains(displayWidths, l.Width):
			b.bad(at+".width", "width is mobile, tablet or desktop")
		case seen[l.Width]:
			b.bad(at+".width", "one layout per width")
		}
		seen[l.Width] = true
		if l.Tracks < 1 || l.Tracks > maxDisplayTracks {
			b.bad(at+".tracks", fmt.Sprintf("tracks is 1 to %d", maxDisplayTracks))
			continue
		}
		if len(l.Cells) > maxColumns {
			b.bad(at+".cells", fmt.Sprintf("a layout places at most %d columns", maxColumns))
			continue
		}
		cols := map[string]bool{}
		for j, cell := range l.Cells {
			cat := fmt.Sprintf("%s.cells.%d", at, j)
			if b.column(p, cell.Col, cat+".col") != nil && cols[cell.Col] {
				b.bad(cat+".col", "a column is placed once per layout")
			}
			cols[cell.Col] = true
			if cell.Justify != "" && cell.Justify != "start" && cell.Justify != "center" && cell.Justify != "end" {
				b.bad(cat+".justify", "justify is start, center or end")
			}
		}
		if _, err := placeCells(l.Tracks, l.Cells); err != nil {
			b.merge(prefixProblem(at, err))
		}
	}
}

// placed is where a display cell sits: its first track, tracks, line and
// lines, all from 1.
type placed struct{ start, span, line, lines int }

// placeCells lays cells out in order: a cell with a start sits there; one
// without takes the first tracks of its line that are free on every line it
// spans; then the cells must cut down by straight lines (no pinwheel, R28).
// lib/table-lines.ts places and cuts cells by the same rules.
func placeCells(tracks int, cells []DisplayCell) ([]placed, error) {
	taken := map[[2]int]bool{}
	out := make([]placed, 0, len(cells))
	for j, c := range cells {
		at := fmt.Sprintf("cells.%d", j)
		span, lines := max(c.Span, 1), max(c.Lines, 1)
		switch {
		case c.Line < 1 || c.Line > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("line is 1 to %d", maxDisplayLines))
		case c.Lines < 0 || lines > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("lines is 1 to %d", maxDisplayLines))
		case c.Line+lines-1 > maxDisplayLines:
			return nil, invalid(at, fmt.Sprintf("a cell ends by line %d", maxDisplayLines))
		case c.Span < 0 || span > tracks:
			return nil, invalid(at, fmt.Sprintf("span is 1 to %d", tracks))
		case c.Start < 0 || c.Start > tracks || (c.Start > 0 && c.Start+span-1 > tracks):
			return nil, invalid(at, "the cell does not fit its line")
		}
		free := func(start int) bool {
			for line := c.Line; line < c.Line+lines; line++ {
				for t := start; t < start+span; t++ {
					if taken[[2]int{line, t}] {
						return false
					}
				}
			}
			return true
		}
		start := c.Start
		if start == 0 {
			for s := 1; s+span-1 <= tracks && start == 0; s++ {
				if free(s) {
					start = s
				}
			}
			if start == 0 {
				return nil, invalid(at, fmt.Sprintf("no room on line %d", c.Line))
			}
		} else if !free(start) {
			return nil, invalid(at, "the cell overlaps another")
		}
		for line := c.Line; line < c.Line+lines; line++ {
			for t := start; t < start+span; t++ {
				taken[[2]int{line, t}] = true
			}
		}
		out = append(out, placed{start, span, c.Line, lines})
	}
	// The client draws an arrangement as blocks cut by straight lines (R28): a
	// pinwheel no straight cut can split cannot be drawn.
	if j := uncuttable(out, tracks); j >= 0 {
		return nil, invalid(fmt.Sprintf("cells.%d", j), pinwheelMessage)
	}
	return out, nil
}

const pinwheelMessage = "These cells overlap in a way that can't be laid out; move one cell."

// piece is a placed cell as a box: tracks [x0, x1) and lines [y0, y1), from 0.
type piece struct{ index, x0, x1, y0, y1 int }

// uncuttable is the index of the first cell of a part no straight cut between
// lines or tracks can split into two non-empty sides, or -1 when the cells cut
// down to one each (guillotine cuts; lib/table-lines.ts cuts the same way).
// Any straight cut that splits a part is as good as another: a part that cuts
// down keeps cutting down after any one cut, so the first cut found is taken.
func uncuttable(ps []placed, tracks int) int {
	pieces := make([]piece, len(ps))
	lines := 0
	for i, p := range ps {
		pieces[i] = piece{i, p.start - 1, p.start - 1 + p.span, p.line - 1, p.line - 1 + p.lines}
		lines = max(lines, p.line-1+p.lines)
	}
	return cutDown(pieces, 0, tracks, 0, lines)
}

func cutDown(pieces []piece, x0, x1, y0, y1 int) int {
	if len(pieces) <= 1 {
		return -1
	}
	split := func(at int, across bool) ([]piece, []piece, bool) {
		var lo, hi []piece
		for _, p := range pieces {
			a0, a1 := p.x0, p.x1
			if across {
				a0, a1 = p.y0, p.y1
			}
			switch {
			case a1 <= at:
				lo = append(lo, p)
			case a0 >= at:
				hi = append(hi, p)
			default:
				return nil, nil, false
			}
		}
		return lo, hi, len(lo) > 0 && len(hi) > 0
	}
	for y := y0 + 1; y < y1; y++ {
		if lo, hi, ok := split(y, true); ok {
			if j := cutDown(lo, x0, x1, y0, y); j >= 0 {
				return j
			}
			return cutDown(hi, x0, x1, y, y1)
		}
	}
	for x := x0 + 1; x < x1; x++ {
		if lo, hi, ok := split(x, false); ok {
			if j := cutDown(lo, x0, x, y0, y1); j >= 0 {
				return j
			}
			return cutDown(hi, x, x1, y0, y1)
		}
	}
	first := pieces[0].index
	for _, p := range pieces {
		first = min(first, p.index)
	}
	return first
}

// prefixProblem moves an error's problem keys under at.
func prefixProblem(at string, err error) *SpecError {
	var se *SpecError
	if !errors.As(err, &se) {
		return invalid(at, err.Error())
	}
	out := &SpecError{Status: se.Status, Problems: map[string]validation.Error{}}
	for k, v := range se.Problems {
		out.Problems[at+"."+k] = v
	}
	return out
}

// optionSources binds each choice param's options source: its value, label
// and group paths, to-one only.
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
		op := &optionsPlan{def: d, at: at, src: src}
		op.value = b.path(src, of.Value, at+".value", pathValue, "")
		op.label = b.path(src, of.Label, at+".label", pathValue, "")
		if of.Group != nil {
			op.group = b.path(src, of.Group, at+".group", pathValue, "")
		}
		if op.value != nil && op.label != nil {
			p.options[d.ID] = op
		}
	}
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
		if len(c.Label) > maxLabel || len(c.Format) > maxFormat {
			b.bad(at, fmt.Sprintf("a label has at most %d characters and a format %d", maxLabel, maxFormat))
		}
		if c.Total != "" && c.Total != TotalRecompute && c.Total != TotalRows {
			b.bad(at+".total", "total is recompute or rows")
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
	}
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
		p.groups = append(p.groups, &groupPlan{group: g, col: cp, at: at})
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
	pp.cell = b.column(p, pv.Cell, "pivot.cell")
	if pp.cell != nil && (pp.cell.col.Aggregate == "" || pp.cell.tm >= 0) {
		b.bad("pivot.cell", "the pivot cell is a column with an aggregate, not a total across a to-many link")
	}
	p.pivot = pp
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
	seen := map[string]bool{}
	keys := map[string]bool{}
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
		case isSection && (item.Key == nil || item.Formula != nil || item.Label != ""):
			b.bad(at, "a section has a title and the key of a first-level group")
		case !isSection && (item.Formula == nil || item.Key != nil || item.Title != ""):
			b.bad(at, "a computed row has a label and a formula")
		case len(item.Title) > maxLabel || len(item.Label) > maxLabel:
			b.bad(at, fmt.Sprintf("a title or label has at most %d characters", maxLabel))
		}
		if isSection && item.Key != nil {
			b.layoutSectionKey(at+".key", item, keys)
		}
		seen[id] = true
		p.layout = append(p.layout, item)
	}
}
