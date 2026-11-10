package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// CurrentVersion is the one spec version this engine reads and writes.
const CurrentVersion = 4

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

// Display is how the report table lays its columns out (E14): the width from
// which one line is tried, each column's fit, and the arrangements per width.
// The engine checks it and otherwise leaves it to the client
// (core/reports/lib/column-measure.ts, table-lines.ts).
type Display struct {
	OneLine *DisplayOneLine      `json:"oneLine,omitempty"`
	Columns map[string]ColumnFit `json:"columns,omitempty"`
	Layouts []DisplayLayout      `json:"layouts,omitempty"`
}

// DisplayOneLine gates the one-line layout: it is tried only where the rows
// are at least MinWidth (px) wide. The columns must still fit.
type DisplayOneLine struct {
	MinWidth string `json:"minWidth"`
}

// ColumnFit is a column's CSS-like fit (column-fit-design.md §3.1). Lengths
// are the text room: the client adds the cell's padding.
type ColumnFit struct {
	Width        string `json:"width,omitempty"`
	MinWidth     string `json:"minWidth,omitempty"`
	MaxWidth     string `json:"maxWidth,omitempty"`
	WhiteSpace   string `json:"whiteSpace,omitempty"`
	LineClamp    int    `json:"lineClamp,omitempty"`
	TextOverflow string `json:"textOverflow,omitempty"`
	// FlexGrow is a pointer: 0 (no share of the spare room) is a setting.
	FlexGrow *int `json:"flexGrow,omitempty"`
}

// DisplayLayout places columns on lines of Tracks equal tracks at one width
// (mobile, tablet or desktop), tried only where the rows are at least
// MinWidth (px) wide. A column it does not list is not shown there.
type DisplayLayout struct {
	Width    string        `json:"width"`
	MinWidth string        `json:"minWidth,omitempty"`
	Tracks   int           `json:"tracks"`
	Cells    []DisplayCell `json:"cells"`
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
	// Detail "blank" leaves the column's cells out of every detail row, so a
	// column that names the open group (a value read from the group's record)
	// shows on the group row only. Empty: the column shows on detail rows.
	Detail string `json:"detail,omitempty"`
	// Rollup is the column's cell on group, subtotal and total rows in place
	// of its total: a formula on the row's level. Detail rows keep Path or
	// Formula.
	Rollup *Node `json:"rollup,omitempty"`
	// Hidden computes the column (a group key, a sort key, a {col} read, a
	// section key) but never shows it: the result leaves the column and its
	// cells out, so it has no heading, cell, Customize entry, arrangement or
	// export column.
	Hidden bool `json:"hidden,omitempty"`
	// Heading is a formula over params and values, computed once per run,
	// that the column's heading shows in place of Label (a date in the
	// reader's short day form). Label stays the column's name in Customize
	// and the builder. A heading that is blank (an unset param) shows Label.
	Heading *Node `json:"heading,omitempty"`
}

// DetailBlank is the one Column.Detail value.
const DetailBlank = "blank"

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
	// default), "max", "sum" or "count" of the matching rows' value. A
	// lookup's Value is a formula node over that source, not a literal.
	Source string `json:"source,omitempty"`
	Pick   string `json:"pick,omitempty"`
	// Outer is a path on the report's row, read from inside a lookup: the
	// row the outermost lookup sits on. It correlates the lookup.
	Outer Path `json:"outer,omitempty"`
	// Bound reads a date range param's first ("from") or last ("to") day.
	Bound string `json:"bound,omitempty"`
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
	// Opening is a row shown first under each open group of this level.
	Opening *Opening `json:"opening,omitempty"`
	// Blank "skip": a row whose key at this level is blank skips the level:
	// no group row, no subtotal; its rows sit directly under the parent group
	// (one depth less per skipped level) and count in the parent's totals.
	// Absent: a blank key forms a (none) group.
	Blank string `json:"blank,omitempty"`
	// HideEmpty hides a group whose child groups are all hidden and that has
	// no shown row passed up from a skipped level below it, with its subtotal,
	// as a having would hide it.
	HideEmpty bool `json:"hideEmpty,omitempty"`
}

// GroupBlankSkip is the one Group.Blank value.
const GroupBlankSkip = "skip"

// Opening is a group level's opening row: its label, and the cells it shows
// by column id, each a formula on the group's level. A column it does not
// name is blank on the row.
type Opening struct {
	Label string           `json:"label"`
	Cells map[string]*Node `json:"cells,omitempty"`
}

// cellIDs are the opening's cell column ids in order, so every walk over its
// cells reports problems in one order.
func (o *Opening) cellIDs() []string { return slices.Sorted(maps.Keys(o.Cells)) }

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
	// Range names a dateRange param: when it resolves to two days, the
	// pivot's columns are every bucket from..to touches, rows or not.
	Range string `json:"range,omitempty"`
	// Order "spec" (PivotOrderSpec) puts the pivot columns just before their
	// cell column, so its total reads after them; absent, they come after
	// every column.
	Order string `json:"order,omitempty"`
}

// PivotOrderSpec is the one Pivot.Order value.
const PivotOrderSpec = "spec"

// LayoutItem is a section (section, title, key: the first group's key it
// shows) or a computed row (row, label, formula over {subtotal} leaves, and
// within: the section it sits inside, if any).
type LayoutItem struct {
	Section string          `json:"section,omitempty"`
	Title   string          `json:"title,omitempty"`
	Key     json.RawMessage `json:"key,omitempty"`
	Row     string          `json:"row,omitempty"`
	Label   string          `json:"label,omitempty"`
	Formula *Node           `json:"formula,omitempty"`
	// Within names a section earlier in the layout that this computed row
	// sits inside: shown after the section's rows and before its total while
	// the section is open, and added to the section's total.
	Within string `json:"within,omitempty"`
	// Cells is the computed row's cell per column id, each a formula like
	// Formula; a number column it does not name takes Formula.
	Cells map[string]*Node `json:"cells,omitempty"`
}

// cellIDs are the computed row's cell column ids in order, so every walk over
// its cells reports problems in one order.
func (l *LayoutItem) cellIDs() []string { return slices.Sorted(maps.Keys(l.Cells)) }

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
// GroupLabel (optional, with Group) is the text of a whole-group pick, with
// GroupLabelToken where the group's label goes; empty means "All {group}".
// The client writes the text; the engine only checks it. GroupOrder
// (optional, with Group) is the order of the groups; nil orders them by
// label.
type OptionsFrom struct {
	Source     string      `json:"source"`
	Value      Path        `json:"value"`
	Label      Path        `json:"label"`
	Group      Path        `json:"group,omitempty"`
	GroupOrder *GroupOrder `json:"groupOrder,omitempty"`
	GroupLabel string      `json:"groupLabel,omitempty"`
	Where      *Node       `json:"where,omitempty"`
}

// GroupLabelToken stands for the group's label in OptionsFrom.GroupLabel.
const GroupLabelToken = "{group}"

// GroupOrder orders a choice param's option groups. The blank group always
// comes first, and groups that tie come by label.
//   - label: by the group's label (a number by its value).
//   - declared: in the order the group's select field declares its options;
//     a value no option names comes after the named ones.
//   - path: by the smallest value of Path (a to-one path read from the
//     options source, as Group is) over the group's options; a group with no
//     value comes after the groups that have one.
type GroupOrder struct {
	By   string `json:"by"`
	Path Path   `json:"path,omitempty"`
}

const (
	GroupOrderLabel    = "label"
	GroupOrderDeclared = "declared"
	GroupOrderPath     = "path"
)

// LoadInfo says what Load changed: every alias it resolved.
type LoadInfo struct {
	Aliased []Alias
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

// Load reads a stored or posted spec: one JSON object of version
// CurrentVersion, decoded strictly (an unknown key is an error), with every
// aliased key replaced by its current key. A later bind step checks the
// result. A spec saved again is saved as Load returns it.
func Load(raw []byte) (*Spec, *LoadInfo, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return nil, nil, invalid("spec", "the spec is not a single JSON object")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, nil, invalid("spec", "the spec is not a single JSON object")
	}
	if n, ok := doc["version"].(json.Number); !ok || n.String() != strconv.Itoa(CurrentVersion) {
		return nil, nil, invalid("version", fmt.Sprintf("version must be %d", CurrentVersion))
	}
	var spec Spec
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&spec); err != nil {
		return nil, nil, invalid("spec", "the spec is not valid: "+err.Error())
	}
	return &spec, &LoadInfo{Aliased: resolveAliases(&spec)}, nil
}
