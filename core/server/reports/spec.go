package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// CurrentVersion is the spec version this engine writes. Load upgrades an
// older spec one version at a time with upgradeSteps.
const CurrentVersion = 2

// upgradeSteps[v] turns a version v document into version v+1. upgrade sets
// the version, not each step. Add a step whenever the spec format changes,
// and raise CurrentVersion with it.
//
// 1 → 2 adds only optional parts (column totals, group having, labels, fill
// and open, formula sort keys, the detail label, options from a source, the
// record and lookup formulas, display layouts), so a version 1 document is
// already a valid version 2 document.
var upgradeSteps = map[int]upgradeStep{
	1: func(map[string]any) error { return nil },
}

type upgradeStep func(doc map[string]any) error

// Spec is a report: versioned JSON, never SQL.
type Spec struct {
	Version int          `json:"version"`
	Source  string       `json:"source"`
	Params  []ParamDef   `json:"params,omitempty"`
	Columns []Column     `json:"columns"`
	Filter  *Node        `json:"filter,omitempty"`
	Sort    []SortKey    `json:"sort,omitempty"`
	Groups  []Group      `json:"groups,omitempty"`
	Pivot   *Pivot       `json:"pivot,omitempty"`
	Layout  []LayoutItem `json:"layout,omitempty"`
	Detail  *Detail      `json:"detail,omitempty"`
	Display *Display     `json:"display,omitempty"`
}

// Display is how the report table lays its columns out per width (E14).
// The engine checks it and otherwise leaves it to the client.
type Display struct {
	Layouts []DisplayLayout `json:"layouts,omitempty"`
}

// DisplayLayout places columns on lines of Tracks equal tracks at one width
// (mobile, tablet or desktop). A column it does not list is not shown there.
type DisplayLayout struct {
	Width  string        `json:"width"`
	Tracks int           `json:"tracks"`
	Cells  []DisplayCell `json:"cells"`
}

// DisplayCell puts a column on Line (from 1), at Start (0 places it in the
// first free tracks), Span tracks wide (1) and Lines lines tall (1).
type DisplayCell struct {
	Col     string `json:"col"`
	Line    int    `json:"line"`
	Start   int    `json:"start,omitempty"`
	Span    int    `json:"span,omitempty"`
	Lines   int    `json:"lines,omitempty"`
	Justify string `json:"justify,omitempty"`
}

// Detail shapes detail rows: Label is a formula on a detail row that labels
// it (the first column's value without it).
type Detail struct {
	Label *Node `json:"label,omitempty"`
}

// Hop is one step of a path: a link to follow, or the field to read last.
type Hop struct {
	Link  string `json:"link,omitempty"`
	Field string `json:"field,omitempty"`
	// Mode, on a to-many hop: "aggregate" (a total per row) or "nest"
	// (child rows).
	Mode string `json:"mode,omitempty"`
	// With is "required" (inner join) or "optional" (left join, the default).
	With string `json:"with,omitempty"`
}

// Path is links then, for a value, one final field hop.
type Path []Hop

// Column is a path to a field, or a formula over other columns.
type Column struct {
	ID        string    `json:"id"`
	Label     string    `json:"label,omitempty"`
	Path      Path      `json:"path,omitempty"`
	Formula   *Node     `json:"formula,omitempty"`
	Aggregate Aggregate `json:"aggregate,omitempty"`
	// Scale is the decimal places of an avg aggregate.
	Scale *int `json:"scale,omitempty"`
	// Format is the client's display format, returned as given.
	Format string `json:"format,omitempty"`
	// Total is how the column's group, subtotal and total cells are made:
	// "recompute" (the default) from every base row under them, or "rows",
	// the sum of the cells of the rows the result shows directly under them.
	Total string `json:"total,omitempty"`
	// LabelField is the field a column whose values point at records shows
	// each record by, in place of its id. Empty: the record's first text
	// field, else its first other field but its id, else its source's name.
	// A formula that is a column shows that column's label field.
	LabelField string `json:"labelField,omitempty"`
}

// Column.Total values.
const (
	TotalRecompute = "recompute"
	TotalRows      = "rows"
)

// Node is a formula or filter tree: an operator with arguments, or a leaf
// (col, path, param, value, subtotal, record). exists and notExists carry a
// link path and an optional where; lookup carries a source, an optional
// where, a value and a pick.
type Node struct {
	Op       string          `json:"op,omitempty"`
	Args     []*Node         `json:"args,omitempty"`
	Scale    *int            `json:"scale,omitempty"`
	Col      string          `json:"col,omitempty"`
	Path     Path            `json:"path,omitempty"`
	Param    string          `json:"param,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	Subtotal string          `json:"subtotal,omitempty"`
	// Record names a group whose key is a link: the leaf reads Path from the
	// linked record, on that group's rows and every row below them.
	Record string `json:"record,omitempty"`
	Link   Path   `json:"link,omitempty"`
	Where  *Node  `json:"where,omitempty"`
	// Source and Pick belong to lookup: the source it reads, and "min" (the
	// default) or "max" of the matching rows' value. A lookup's Value is a
	// formula node over that source, not a literal.
	Source string `json:"source,omitempty"`
	Pick   string `json:"pick,omitempty"`
}

// SortKey sorts by a column or by a formula ("asc" by default). On a group
// level a formula is a cell of that level; on detail rows a row value.
type SortKey struct {
	Col     string `json:"col,omitempty"`
	Formula *Node  `json:"formula,omitempty"`
	Dir     string `json:"dir,omitempty"`
}

type Group struct {
	ID        string `json:"id"`
	Col       string `json:"col"`
	Bucket    string `json:"bucket,omitempty"` // a date column's bucket
	Subtotals bool   `json:"subtotals,omitempty"`
	// Open opens the level in a run with no expansion state (the first
	// open); once a run sends keys, its list wins. It opens only when every
	// shallower level is open too.
	Open bool `json:"open,omitempty"`
	// Label is a formula on the group's level that labels its rows (a
	// record's label when its value points at one). Without it a record
	// group is labelled as its column shows the record (labelFieldOf), any
	// other group by its key.
	Label *Node `json:"label,omitempty"`
	// Fill adds the linked records that have no rows, under each parent group.
	Fill *Fill `json:"fill,omitempty"`
	// Having is a condition on the group's level; a group that fails it is
	// hidden with every row under it.
	Having *Node `json:"having,omitempty"`
}

// Fill is a record group's "show every record": the target's records that
// match Where (all it may list when Where is nil) and have no base rows, under
// each parent group and currency.
type Fill struct {
	Where *Node `json:"where,omitempty"`
}

type Pivot struct {
	Path       Path   `json:"path"`
	Bucket     string `json:"bucket,omitempty"`
	Cell       string `json:"cell"`
	MaxColumns int    `json:"maxColumns,omitempty"`
	// LabelField is the field a pivot over a link names its columns by, as
	// Column.LabelField.
	LabelField string `json:"labelField,omitempty"`
}

// LayoutItem is a section (section, title, key: the first group's key it
// shows) or a computed row (row, label, formula over {subtotal} leaves).
type LayoutItem struct {
	Section string          `json:"section,omitempty"`
	Title   string          `json:"title,omitempty"`
	Key     json.RawMessage `json:"key,omitempty"`
	Row     string          `json:"row,omitempty"`
	Label   string          `json:"label,omitempty"`
	Formula *Node           `json:"formula,omitempty"`
}

type ParamDef struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Label   string          `json:"label,omitempty"`
	Default json.RawMessage `json:"default,omitempty"`
	Options []Option        `json:"options,omitempty"`
	// OptionsFrom names the source a choice param's options come from, in
	// place of Options.
	OptionsFrom *OptionsFrom `json:"optionsFrom,omitempty"`
	Multiple    bool         `json:"multiple,omitempty"`
}

// OptionsFrom reads a choice param's options from a source, under the
// reader's list rules: Value is what the param holds, Label what the picker
// shows, Group (optional) how it groups them, Where which rows count.
type OptionsFrom struct {
	Source string `json:"source"`
	Value  Path   `json:"value"`
	Label  Path   `json:"label"`
	Group  Path   `json:"group,omitempty"`
	Where  *Node  `json:"where,omitempty"`
}

// LoadInfo says what Load changed: the version it upgraded from (0 when it
// did not upgrade) and every alias it resolved.
type LoadInfo struct {
	UpgradedFrom int
	Aliased      []Alias
}

// Alias is one old key Load replaced with its new key.
type Alias struct {
	Source string `json:"source"`
	From   string `json:"from"`
	To     string `json:"to"`
}

// SpecError lists what is wrong with a spec, keyed by the dotted path of the
// part (`columns.2.path.1.link`). Status is 400 for an invalid spec and 422
// when it names a source, link, field or column that does not exist.
type SpecError struct {
	Status   int
	Problems map[string]validation.Error
}

func (e *SpecError) Error() string {
	keys := make([]string, 0, len(e.Problems))
	for k := range e.Problems {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+": "+e.Problems[k].Error())
	}
	return "reports: " + strings.Join(parts, "; ")
}

func invalid(at, message string) *SpecError {
	return &SpecError{Status: 400, Problems: map[string]validation.Error{at: validation.NewError("invalid", message)}}
}

// unknownRef is the 422 for a spec part that names a source, link, field,
// column, param or layout item that does not exist; code says which.
func unknownRef(at, code, message string) *SpecError {
	return &SpecError{Status: 422, Problems: map[string]validation.Error{at: validation.NewError(code, message)}}
}

// Load reads a stored or posted spec: it upgrades an older version to
// CurrentVersion, decodes it strictly (an unknown key is an error), and
// replaces every aliased key with its current key. A later bind step checks
// the result. A spec saved again is saved as Load returns it.
func Load(raw []byte) (*Spec, *LoadInfo, error) {
	return loadWith(raw, upgradeSteps, CurrentVersion)
}

func loadWith(raw []byte, steps map[int]upgradeStep, target int) (*Spec, *LoadInfo, error) {
	upgraded, from, err := upgrade(raw, steps, target)
	if err != nil {
		return nil, nil, err
	}
	var spec Spec
	dec := json.NewDecoder(bytes.NewReader(upgraded))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&spec); err != nil {
		return nil, nil, invalid("spec", "the spec is not valid: "+err.Error())
	}
	info := &LoadInfo{Aliased: resolveAliases(&spec)}
	// The notice says the report changed; a step that changed nothing (1 → 2)
	// gives none.
	if from != target && !sameExceptVersion(raw, upgraded) {
		info.UpgradedFrom = from
	}
	return &spec, info, nil
}

// upgrade runs steps from the document's version up to target. Numbers stay
// json.Number so an integer above 2^53 survives the re-encode unchanged.
func upgrade(raw []byte, steps map[int]upgradeStep, target int) ([]byte, int, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, 0, invalid("spec", "the spec is not a single JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, 0, invalid("spec", "the spec is not a single JSON object")
	}
	n, ok := doc["version"].(json.Number)
	if !ok {
		return nil, 0, invalid("version", "version must be a whole number from 1")
	}
	v, err := n.Int64()
	if err != nil || v < 1 {
		return nil, 0, invalid("version", "version must be a whole number from 1")
	}
	from := int(v)
	if from > target {
		return nil, 0, invalid("version", fmt.Sprintf("version %d is newer than this server reads (%d)", from, target))
	}
	if from == target {
		return raw, from, nil
	}
	for at := from; at < target; at++ {
		step := steps[at]
		if step == nil {
			return nil, 0, fmt.Errorf("reports: no upgrade from spec version %d", at)
		}
		if err := step(doc); err != nil {
			return nil, 0, invalid("version", fmt.Sprintf("version %d cannot be upgraded: %v", at, err))
		}
		doc["version"] = at + 1
	}
	out, err := json.Marshal(doc)
	return out, from, err
}

// sameExceptVersion reports whether two spec documents differ only in their
// version. Numbers stay json.Number, so a large integer compares exactly.
func sameExceptVersion(a, b []byte) bool {
	read := func(raw []byte) (map[string]any, bool) {
		var doc map[string]any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if dec.Decode(&doc) != nil {
			return nil, false
		}
		delete(doc, "version")
		return doc, true
	}
	da, okA := read(a)
	db, okB := read(b)
	return okA && okB && reflect.DeepEqual(da, db)
}

// resolveAliases replaces an aliased link or field key on every path with
// its current key. It reads every registered source, refused or not: Load
// has no app, and bind refuses a source the app refused. A key it cannot
// place (an unknown source, a link that does not exist) is left for Validate
// to report.
func resolveAliases(spec *Spec) []Alias {
	base := lookupRegistered(spec.Source)
	if base == nil {
		return nil
	}
	a := &aliaser{targets: map[string]*Source{}}
	cols := map[string]*Column{}
	for i := range spec.Columns {
		c := &spec.Columns[i]
		cols[c.ID] = c
		// A label field is a field of the record the path points at.
		if t := a.path(base, c.Path); t != nil && c.LabelField != "" {
			c.LabelField = a.key(t, c.LabelField)
		}
	}
	// Group targets come before formulas, since a formula's record leaf reads
	// one; a group column's formula is resolved later, so pathTarget reads
	// links by old keys too. A record leaf may name an earlier group, so
	// groups are read in order.
	for _, g := range spec.Groups {
		if c := cols[g.Col]; c != nil && a.targets[g.ID] == nil {
			a.targets[g.ID] = a.targetOf(base, c, cols, map[string]bool{})
		}
	}
	for i := range spec.Columns {
		a.node(base, spec.Columns[i].Formula)
	}
	a.node(base, spec.Filter)
	for i := range spec.Groups {
		g := &spec.Groups[i]
		a.node(base, g.Label)
		a.node(base, g.Having)
		if g.Fill != nil {
			// A fill's where reads the records the group links to.
			a.node(a.targets[g.ID], g.Fill.Where)
		}
	}
	for i := range spec.Sort {
		a.node(base, spec.Sort[i].Formula)
	}
	if spec.Detail != nil {
		a.node(base, spec.Detail.Label)
	}
	for i := range spec.Layout {
		a.node(base, spec.Layout[i].Formula)
	}
	if pv := spec.Pivot; pv != nil {
		if t := a.path(base, pv.Path); t != nil && pv.LabelField != "" {
			pv.LabelField = a.key(t, pv.LabelField)
		}
	}
	for i := range spec.Params {
		// An options source's paths and where read that source, not base.
		of := spec.Params[i].OptionsFrom
		if of == nil {
			continue
		}
		if src := lookupRegistered(of.Source); src != nil {
			a.path(src, of.Value)
			a.path(src, of.Label)
			a.path(src, of.Group)
			a.node(src, of.Where)
		}
	}
	return a.out
}

// aliaser resolves aliased keys in place and records each one. targets maps
// a group id to the source its key links to, for record leaves and fill.
type aliaser struct {
	targets map[string]*Source
	out     []Alias
}

// path resolves a path's keys in place and returns the source the path ends
// on, or nil when a hop cannot be followed.
func (a *aliaser) path(src *Source, path Path) *Source {
	for i := range path {
		if src == nil {
			return nil
		}
		hop := &path[i]
		if hop.Link != "" {
			hop.Link = a.key(src, hop.Link)
			l := src.link(hop.Link)
			if l == nil {
				return nil
			}
			src = lookupRegistered(l.Target)
			if src == nil {
				return nil
			}
		}
		if hop.Field != "" {
			hop.Field = a.key(src, hop.Field)
		}
	}
	return src
}

func (a *aliaser) key(src *Source, key string) string {
	if src.field(key) != nil || src.link(key) != nil {
		return key
	}
	to, ok := src.Aliases[key]
	if !ok {
		return key
	}
	a.out = append(a.out, Alias{Source: src.Key, From: key, To: to})
	return to
}

// node resolves a formula's paths on src. A record leaf's path is on its
// group's target, a lookup's on the source it looks up, and an exists where's
// on the source its link path ends on.
func (a *aliaser) node(src *Source, n *Node) {
	if n == nil {
		return
	}
	switch {
	case n.Op == "lookup":
		a.lookup(n)
		return
	case n.Record != "":
		a.path(a.targets[n.Record], n.Path)
	case src != nil:
		a.path(src, n.Path)
	}
	for _, arg := range n.Args {
		a.node(src, arg)
	}
	if n.Link != nil && src != nil {
		a.node(a.path(src, n.Link), n.Where)
	}
}

// lookup resolves a lookup's value and where against the source it looks
// up. The value is kept as raw JSON, so it is encoded again only when a key
// in it changed.
func (a *aliaser) lookup(n *Node) {
	src := lookupRegistered(n.Source)
	if v, err := lookupValue(n, ""); err == nil {
		before := len(a.out)
		a.node(src, v)
		if len(a.out) > before {
			if raw, err := json.Marshal(v); err == nil {
				n.Value = raw
			}
		}
	}
	a.node(src, n.Where)
}

// targetOf is the source a group column's value links to, as the compiler
// finds it (expr.target): a path that ends at a to-one link or at id, a
// column it names, a record leaf, a lookup's value, or the results of if,
// case and coalesce. nil when it links nowhere. Keys are current here.
func (a *aliaser) targetOf(base *Source, c *Column, cols map[string]*Column, seen map[string]bool) *Source {
	if seen[c.ID] {
		return nil
	}
	seen[c.ID] = true
	if c.Path != nil {
		return pathTarget(base, c.Path)
	}
	return a.nodeTarget(base, c.Formula, cols, seen)
}

func (a *aliaser) nodeTarget(src *Source, n *Node, cols map[string]*Column, seen map[string]bool) *Source {
	switch {
	case n == nil:
		return nil
	case n.Record != "":
		return pathTarget(a.targets[n.Record], n.Path)
	case n.Path != nil:
		return pathTarget(src, n.Path)
	case n.Col != "":
		if c := cols[n.Col]; c != nil {
			return a.targetOf(src, c, cols, seen)
		}
	case n.Op == "lookup":
		if v, err := lookupValue(n, ""); err == nil {
			return a.nodeTarget(lookupRegistered(n.Source), v, cols, seen)
		}
	case n.Op == "if":
		// The first argument is the condition, which links nowhere.
		if len(n.Args) > 1 {
			return a.firstTarget(src, n.Args[1:], cols, seen)
		}
	case n.Op == "case" || n.Op == "coalesce":
		return a.firstTarget(src, n.Args, cols, seen)
	}
	return nil
}

// firstTarget is the first target among results; the compiler refuses
// results that link to different sources.
func (a *aliaser) firstTarget(src *Source, args []*Node, cols map[string]*Column, seen map[string]bool) *Source {
	for _, arg := range args {
		if t := a.nodeTarget(src, arg, cols, seen); t != nil {
			return t
		}
	}
	return nil
}

// pathTarget is the source a value path points at, as bind finds it
// (pathPlan.target): the end of to-one links, or the source whose id it
// reads. A to-many hop makes a total, which points nowhere. A link may still
// have its old key.
func pathTarget(src *Source, path Path) *Source {
	for _, hop := range path {
		if src == nil {
			return nil
		}
		if hop.Link != "" {
			l := src.link(hop.Link)
			if l == nil {
				l = src.link(src.Aliases[hop.Link])
			}
			if l == nil || l.Cardinality != ToOne {
				return nil
			}
			src = lookupRegistered(l.Target)
			continue
		}
		if hop.Field == idField.Key {
			return src
		}
		return nil
	}
	return src
}
