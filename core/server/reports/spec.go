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
	"unicode"

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
	// Header are the report's own document header lines, printed after the
	// params' header lines (header.go).
	Header []HeaderLine `json:"header,omitempty"`
	// EmptyText is the report's own text for a run with no rows, shown by the
	// view and the PDF and XLSX exports in place of the engine's "No rows"
	// (export.go emptyText; the client's emptyMessage). Optional.
	EmptyText string `json:"emptyText,omitempty"`
	// NoTotal leaves out the "Total" row a report without a layout ends in
	// (assemble.go), for a report whose sum over all rows means nothing; a run
	// with no rows then has zero rows. A layout report has no Total row either
	// way. Optional; the client's reportSpecSchema has the same key.
	NoTotal bool `json:"noTotal,omitempty"`
}

// maxEmptyTextLen is the most characters (code points) of a report's own empty
// text; the client's reportSpecSchema has the same bound and the same ends rule.
const maxEmptyTextLen = 120

// emptyText refuses an empty text with a space at either end (so also a text of
// only spaces) or longer than maxEmptyTextLen code points.
func (b *binder) emptyText(spec *Spec) {
	if spec.EmptyText == "" {
		return
	}
	switch {
	case strings.TrimFunc(spec.EmptyText, isTrimmedSpace) != spec.EmptyText:
		b.bad("emptyText", "the empty text has no space at either end")
	case characters(spec.EmptyText) > maxEmptyTextLen:
		b.bad("emptyText", fmt.Sprintf("the empty text has at most %d characters", maxEmptyTextLen))
	}
}

// isTrimmedSpace is a character that JavaScript's String.prototype.trim
// removes, so the client's schema and this check refuse the same texts: Go's
// white space with U+FEFF, without U+0085, which trim keeps.
func isTrimmedSpace(r rune) bool {
	if r == 0xFEFF {
		return true
	}
	return r != 0x85 && unicode.IsSpace(r)
}

// HeaderLine is one document header line: Text with HeaderValueToken once,
// where the run writes Value, a formula of params, values and lookups that is
// one text, day, number or amount for the whole run (header.go).
type HeaderLine struct {
	Text  string `json:"text"`
	Value *Node  `json:"value"`
}

// HeaderValueToken stands for a header line's value in HeaderLine.Text.
const HeaderValueToken = "{value}"

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
	// OverflowWrap "anywhere" lets a wrapping column break a word wider than
	// its line; only with WhiteSpace "normal".
	OverflowWrap string `json:"overflowWrap,omitempty"`
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
// it (the first column's value without it); Open is a text formula on a
// detail row, the app path under the report's package that the row opens
// (D2). A relation in a text operator inside Open reads as its record's id,
// also inside an aggregate or a lookup there.
type Detail struct {
	Label *Node `json:"label,omitempty"`
	Open  *Node `json:"open,omitempty"`
}

// Drill is a column's drill: the shipped report it opens (Report, a key of
// the report's own package) and the target's param values per drilled
// group level (Levels, by group id, then by target param id). PivotCells,
// on the pivot's cell column only, are laid over a level's values on its
// pivot cells.
type Drill struct {
	Report     string                           `json:"report"`
	Levels     map[string]map[string]DrillValue `json:"levels"`
	PivotCells map[string]DrillValue            `json:"pivotCells,omitempty"`
}

// DrillValue is one of: a formula on the drilled level, a date range from
// two formulas, a whole group of a choice param's options from a formula,
// or a copy of a param of the report as the run was given it (R1, R3, R4).
type DrillValue struct {
	Formula *Node
	Range   *DrillRange
	Group   *Node
	Param   string
}

type DrillRange struct {
	From *Node `json:"from"`
	To   *Node `json:"to"`
}

func (v DrillValue) MarshalJSON() ([]byte, error) {
	switch {
	case v.Range != nil:
		return json.Marshal(struct {
			Range *DrillRange `json:"range"`
		}{v.Range})
	case v.Group != nil:
		return json.Marshal(struct {
			Group *Node `json:"group"`
		}{v.Group})
	case v.Param != "":
		return json.Marshal(struct {
			Param string `json:"param"`
		}{v.Param})
	}
	return json.Marshal(v.Formula)
}

// UnmarshalJSON reads a value's form by its one key (R1): range, group, or
// param as a string; anything else is a formula. Every part decodes
// strictly, as Load decodes the spec: a custom decoder does not inherit
// DisallowUnknownFields.
func (v *DrillValue) UnmarshalJSON(raw []byte) error {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return err
	}
	if len(keys) == 1 {
		switch {
		case keys["range"] != nil:
			return strictJSON(keys["range"], &v.Range)
		case keys["group"] != nil:
			return strictJSON(keys["group"], &v.Group)
		case keys["param"] != nil:
			if err := json.Unmarshal(keys["param"], &v.Param); err == nil {
				return nil
			}
		}
	}
	return strictJSON(raw, &v.Formula)
}

func strictJSON(raw []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
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
	// shows on the group row only. Such a column whose group cell is not a
	// figure (not a number, money or percent) is group-only
	// (ResultColumn.GroupOnly): it shows on the group row and not on the
	// group's subtotal row. Empty: the column shows on detail rows.
	Detail string `json:"detail,omitempty"`
	// Rollup is the column's cell on group, subtotal and total rows in place
	// of its total: a formula on the row's level. Detail rows keep Path or
	// Formula. With Total "rows" it is the cell of the innermost group's rows
	// only: every row above them adds the rows shown under it, so a rollup
	// that reads the group's record (a looked-up balance) still totals. A
	// blank cell adds 0; a cell that is blank for two currencies, or is in
	// another currency than the sum's, makes the sum blank. Such a report has
	// a group. With a layout it has a group below the section group: a section
	// and a computed row that reads it add the rows shown under the section.
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
	// BlankAbove names a group: the column totals on that group's level and the
	// levels inside it, and leaves its cell blank on the group and subtotal
	// rows of outer levels, layout sections and the Total (no currency cell
	// either). For a figure the outer levels would count twice.
	BlankAbove string `json:"blankAbove,omitempty"`
	// Drill opens a shipped report of the same package from this column's
	// figures on group and subtotal rows, filtered to the rows behind each
	// figure (D1).
	Drill *Drill `json:"drill,omitempty"`
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
	// GroupKey reads, in a drill value only, the key of a group at or above
	// the drilled level (a bucket's key as text, a link's record id).
	GroupKey string `json:"groupKey,omitempty"`
	// PivotBound reads, in a drill's pivotCells only, the first ("from") or
	// last ("to") day of the pivot cell's bucket, cut to the pivot's range.
	PivotBound string `json:"pivotBound,omitempty"`
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
	// Inline is a condition on the group's level, as Having is; a group that
	// meets it draws no group row and no subtotal: its child rows take its
	// place, at its depth, and count in the totals above as before. Not on
	// the last level, a bucketed level or a layout's first group.
	Inline *Node `json:"inline,omitempty"`
	// Muted is a condition on the group's level, as Having is; a group row
	// that meets it carries Row.Muted, which a view draws in muted text.
	Muted *Node `json:"muted,omitempty"`
}

// GroupBlankSkip is the one Group.Blank value.
const GroupBlankSkip = "skip"

// Opening is a group level's opening row: its label, and the cells it shows
// by column id, each a formula on the group's level. A column it does not
// name is blank on the row.
type Opening struct {
	Label string `json:"label"`
	// LabelValue is a value of the run the label names, a formula of params
	// and values only (as Column.Heading): a text, a day or a number. Label
	// then holds LabelValueToken once, where the value goes. The row carries
	// the value typed (Row.LabelTemplate), so a view writes it in the reader's
	// form. A value that is blank on a run leaves the token out of the label.
	LabelValue *Node `json:"labelValue,omitempty"`
	// LabelBucket cuts a LabelValue that is a day to its day, month, quarter,
	// year or fiscalYear, as Group.Bucket cuts a group's key: the label then
	// names the period ("Jul 2026"), not the day.
	LabelBucket string           `json:"labelBucket,omitempty"`
	Cells       map[string]*Node `json:"cells,omitempty"`
}

// LabelValueToken stands for the label value in Opening.Label.
const LabelValueToken = "{value}"

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
	// Header is a choice param's line under the report's period, with
	// HeaderLabelToken where the picked options' labels go, joined by ", "
	// ("{label} basis", "Customer: {label}"); a whole-group pick reads as its
	// group pick text. A run gives the line in Result.HeaderLines.
	Header string `json:"header,omitempty"`
}

// HeaderLabelToken stands for the picked options' labels in ParamDef.Header.
const HeaderLabelToken = "{label}"

// OptionsFrom reads a choice param's options from a source, under the
// reader's list rules: Value is what the param holds, Label what the picker
// shows, Group (optional) how it groups them, Where which rows count.
// GroupLabel (optional, with Group) is the text of a whole-group pick, with
// GroupLabelToken where the group's label goes; empty means "All {group}".
// The client writes the text; the engine only checks it. GroupOrder
// (optional, with Group) is the order of the groups; nil orders them by
// label. LabelFormula, in place of Label, is a text formula over the
// source's paths (and params) that labels each option, so a label can join
// fields ("1000 Business Checking"). Order (optional) is a to-one path whose
// value orders the options (inside each group): a blank value comes after
// the others, and a tie, or no Order, goes by label, then value.
type OptionsFrom struct {
	Source       string      `json:"source"`
	Value        Path        `json:"value"`
	Label        Path        `json:"label,omitempty"`
	LabelFormula *Node       `json:"labelFormula,omitempty"`
	Order        Path        `json:"order,omitempty"`
	Group        Path        `json:"group,omitempty"`
	GroupOrder   *GroupOrder `json:"groupOrder,omitempty"`
	GroupLabel   string      `json:"groupLabel,omitempty"`
	Where        *Node       `json:"where,omitempty"`
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
	if err := checkHeaderShape(raw); err != nil {
		return nil, nil, err
	}
	var spec Spec
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&spec); err != nil {
		return nil, nil, invalid("spec", "the spec is not valid: "+err.Error())
	}
	return &spec, &LoadInfo{Aliased: resolveAliases(&spec)}, nil
}
