package reports

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
)

// CurrentVersion is the spec version this engine writes. Load upgrades an
// older spec one version at a time with upgradeSteps.
const CurrentVersion = 1

// upgradeSteps[v] turns a version v document into version v+1. upgrade sets
// the version, not each step. Add a step whenever the spec format changes,
// and raise CurrentVersion with it.
var upgradeSteps = map[int]upgradeStep{}

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
}

// Node is a formula or filter tree: an operator with arguments, or a leaf
// (col, path, param, value, subtotal). exists and notExists carry a link path
// and an optional where.
type Node struct {
	Op       string          `json:"op,omitempty"`
	Args     []*Node         `json:"args,omitempty"`
	Scale    *int            `json:"scale,omitempty"`
	Col      string          `json:"col,omitempty"`
	Path     Path            `json:"path,omitempty"`
	Param    string          `json:"param,omitempty"`
	Value    json.RawMessage `json:"value,omitempty"`
	Subtotal string          `json:"subtotal,omitempty"`
	Link     Path            `json:"link,omitempty"`
	Where    *Node           `json:"where,omitempty"`
}

type SortKey struct {
	Col string `json:"col"`
	Dir string `json:"dir,omitempty"` // "asc" (default) or "desc"
}

type Group struct {
	ID        string `json:"id"`
	Col       string `json:"col"`
	Bucket    string `json:"bucket,omitempty"` // a date column's bucket
	Subtotals bool   `json:"subtotals,omitempty"`
}

type Pivot struct {
	Path       Path   `json:"path"`
	Bucket     string `json:"bucket,omitempty"`
	Cell       string `json:"cell"`
	MaxColumns int    `json:"maxColumns,omitempty"`
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
	ID       string          `json:"id"`
	Type     string          `json:"type"`
	Label    string          `json:"label,omitempty"`
	Default  json.RawMessage `json:"default,omitempty"`
	Options  []Option        `json:"options,omitempty"`
	Multiple bool            `json:"multiple,omitempty"`
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
	if from != target {
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
	var out []Alias
	for i := range spec.Columns {
		aliasPath(base, spec.Columns[i].Path, &out)
		aliasNode(base, spec.Columns[i].Formula, &out)
	}
	aliasNode(base, spec.Filter, &out)
	if spec.Pivot != nil {
		aliasPath(base, spec.Pivot.Path, &out)
	}
	return out
}

// aliasPath resolves a path's keys in place and returns the source the path
// ends on, or nil when a hop cannot be followed.
func aliasPath(src *Source, path Path, out *[]Alias) *Source {
	for i := range path {
		if src == nil {
			return nil
		}
		hop := &path[i]
		if hop.Link != "" {
			hop.Link = aliasKey(src, hop.Link, out)
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
			hop.Field = aliasKey(src, hop.Field, out)
		}
	}
	return src
}

func aliasKey(src *Source, key string, out *[]Alias) string {
	if src.field(key) != nil || src.link(key) != nil {
		return key
	}
	to, ok := src.Aliases[key]
	if !ok {
		return key
	}
	*out = append(*out, Alias{Source: src.Key, From: key, To: to})
	return to
}

func aliasNode(src *Source, n *Node, out *[]Alias) {
	if n == nil {
		return
	}
	aliasPath(src, n.Path, out)
	for _, a := range n.Args {
		aliasNode(src, a, out)
	}
	if n.Link != nil {
		// A where clause reads the source the link path ends on.
		aliasNode(aliasPath(src, n.Link, out), n.Where, out)
	}
}
