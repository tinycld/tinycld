package forms

import (
	"encoding/json"
	"fmt"
	"os"
)

// Wire mirror of core/lib/forms/host.ts FormHostDef, as the generator emits
// it into server/form_hosts.json. Core never names a package: every host
// here comes from a package's own declaration.
type Builtin struct {
	Key           string `json:"key"`
	Label         string `json:"label"`
	Kind          string `json:"kind"`
	Storage       string `json:"storage"`
	Required      bool   `json:"required"`
	Multiple      bool   `json:"multiple"`
	Collaborative bool   `json:"collaborative"`
	Component     string `json:"component,omitempty"`
	// Locked is an optional built-in the record depends on: a definition may
	// move and relabel it, never hide it (ValidateDefinition refuses).
	Locked bool `json:"locked"`
	// Chrome is presentation only ("cell" or "bare"); the server decodes it
	// so the host file round-trips, and never reads it.
	Chrome string `json:"chrome,omitempty"`
	// Expression, Decimals and Columns mirror the computed and table built-ins
	// of core/lib/forms/host.ts.
	Expression any      `json:"expression,omitempty"`
	Decimals   int      `json:"decimals,omitempty"`
	Columns    []Column `json:"columns,omitempty"`
	// MaxLength is a text built-in's most characters (core/lib/forms/host.ts
	// refuses it on any other kind). validateHostRecord checks a changed value
	// against it, so every writer gets the refusal the form shows.
	MaxLength *int `json:"maxLength,omitempty"`
	// RowKinds and RowShortcuts are a table built-in's line kinds and row
	// keys. ParseHosts checks that each kind names the table's own columns
	// and requires only its own columns; RowError reads the kinds to check a
	// row. RowShortcuts is presentation only.
	RowKinds     []RowKind `json:"rowKinds,omitempty"`
	RowShortcuts bool      `json:"rowShortcuts,omitempty"`
	// InsertShortcuts is presentation only: the keys a host binds for a
	// table's insert actions, shown on its row menu. The server decodes it so
	// the host file round-trips, and never reads it.
	InsertShortcuts *InsertShortcuts `json:"insertShortcuts,omitempty"`
}

// InsertShortcuts mirrors insertShortcutsSchema in core/lib/forms/host.ts:
// the key combos of a table's "Insert line above" and "Insert line below".
type InsertShortcuts struct {
	Above string `json:"above,omitempty"`
	Below string `json:"below,omitempty"`
}

// RowKind mirrors rowKindSchema in core/lib/forms/host.ts: a kind of line in a
// table built-in, stored in the row's `kind` cell. Label, AddLabel, Span,
// ColumnAt and Shortcut are presentation only. Columns and Required are read
// by RowError: Required lists the columns a row of this kind must fill,
// beside each column's own Required.
type RowKind struct {
	Key      string            `json:"key"`
	Label    string            `json:"label"`
	AddLabel string            `json:"addLabel"`
	Columns  []string          `json:"columns"`
	Required []string          `json:"required,omitempty"`
	Span     bool              `json:"span,omitempty"`
	ColumnAt map[string]string `json:"columnAt,omitempty"`
	Shortcut string            `json:"shortcut,omitempty"`
}

type Files struct {
	Collection     string `json:"collection"`
	RecordColumn   string `json:"recordColumn"`
	FieldColumn    string `json:"fieldColumn"`
	SizeColumn     string `json:"sizeColumn"`
	UploaderColumn string `json:"uploaderColumn"`
	// NameColumn holds the file's display name; defaults to "name".
	NameColumn string `json:"nameColumn"`
	// FileColumn is the PocketBase file field; defaults to "file".
	FileColumn string `json:"fileColumn"`
	// CopyColumns maps attachment column to host record column; only the client reads it.
	CopyColumns map[string]string `json:"copyColumns,omitempty"`
}

type Collab struct {
	Fragment string `json:"fragment"`
}

type Host struct {
	PkgSlug      string    `json:"pkgSlug"`
	Key          string    `json:"key"`
	Label        string    `json:"label"`
	Collection   string    `json:"collection"`
	FormColumn   string    `json:"formColumn"`
	FieldsColumn string    `json:"fieldsColumn"`
	Columns      []string  `json:"columns"`
	Builtins     []Builtin `json:"builtin"`
	// Kinds lists the custom field kinds the host allows; empty allows all.
	Kinds  []string `json:"kinds,omitempty"`
	Files  *Files   `json:"files,omitempty"`
	Collab *Collab  `json:"collaborative,omitempty"`

	InsertCustomBefore string `json:"insertCustomBefore,omitempty"`
	// Layout is the layout of the host's default form: the first seeded
	// form, and every new form an admin starts from Settings → Forms → New
	// form (the client starts it from defaultDefinitionFor). Nil means one
	// full-width page.
	Layout *HostLayout `json:"layout,omitempty"`
}

// HostLayout mirrors hostLayoutSchema in core/lib/forms/host.ts. The client
// schema checks every key it names when the generator merges the hosts.
type HostLayout struct {
	Pages  []Page                 `json:"pages"`
	Fields map[string]LayoutField `json:"fields"`
	Rules  []Rule                 `json:"rules"`
}

// LayoutField places one built-in; an empty value means the first page, or
// full width. Description is the built-in's help text on the seeded field;
// empty means none.
type LayoutField struct {
	Page        string `json:"page,omitempty"`
	Width       string `json:"width,omitempty"`
	Description string `json:"description,omitempty"`
}

type Defs struct {
	Hosts []Host `json:"hosts"`
}

func LoadHosts(path string) (*Defs, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Defs{}, nil
		}
		return nil, fmt.Errorf("forms: read hosts: %w", err)
	}
	return ParseHosts(raw)
}

func ParseHosts(raw []byte) (*Defs, error) {
	var defs Defs
	if err := json.Unmarshal(raw, &defs); err != nil {
		return nil, fmt.Errorf("forms: parse hosts: %w", err)
	}
	for i := range defs.Hosts {
		if err := rowKindsError(&defs.Hosts[i]); err != nil {
			return nil, err
		}
		if f := defs.Hosts[i].Files; f != nil {
			if f.NameColumn == "" {
				f.NameColumn = "name"
			}
			if f.FileColumn == "" {
				f.FileColumn = "file"
			}
		}
	}
	return &defs, nil
}

// rowKindsError refuses row kinds the client schema (rowKindIssues in
// core/lib/forms/host.ts) refuses: kinds on a built-in that is not a table, a
// repeated kind key, a kind column the table lacks, a span kind with other
// than one column, and a required key that is not one of the kind's columns
// or is named twice.
func rowKindsError(h *Host) error {
	for _, b := range h.Builtins {
		if len(b.RowKinds) == 0 {
			continue
		}
		if b.Kind != "table" {
			return fmt.Errorf("forms: host %s: %s: only a table built-in has row kinds", h.Key, b.Key)
		}
		columns := map[string]bool{}
		for _, c := range b.Columns {
			columns[c.Key] = true
		}
		seen := map[string]bool{}
		for _, k := range b.RowKinds {
			if seen[k.Key] {
				return fmt.Errorf("forms: host %s: %s: duplicate row kind key %s", h.Key, b.Key, k.Key)
			}
			seen[k.Key] = true
			for _, c := range k.Columns {
				if !columns[c] {
					return fmt.Errorf("forms: host %s: %s: row kind %s: %s is not a column of this table", h.Key, b.Key, k.Key, c)
				}
			}
			if k.Span && len(k.Columns) != 1 {
				return fmt.Errorf("forms: host %s: %s: row kind %s: a span row kind has exactly one column", h.Key, b.Key, k.Key)
			}
			own := map[string]bool{}
			for _, c := range k.Columns {
				own[c] = true
			}
			required := map[string]bool{}
			for _, c := range k.Required {
				if !own[c] {
					return fmt.Errorf("forms: host %s: %s: row kind %s: %s is not a column of this kind", h.Key, b.Key, k.Key, c)
				}
				if required[c] {
					return fmt.Errorf("forms: host %s: %s: row kind %s: duplicate required column %s", h.Key, b.Key, k.Key, c)
				}
				required[c] = true
			}
		}
	}
	return nil
}

// RowError checks one row of a table built-in, for the host package's Go
// hooks: core never sees host-table rows, which live in the host's own
// collection. The row's `kind` cell picks its RowKind; a table without row
// kinds checks every column. It returns "<Label> is required" for the first
// column, in the kind's column order, that the row must fill and leaves
// blank, and "" when the row is complete. A column must be filled when it is
// table-wide Required or in the kind's Required, and is never required when
// it is Hidden, a toggle or computed (the same rules as the client's row
// schema in core/lib/forms/values.ts). A row whose kind is missing or not one
// of the table's kinds gets "<Label>: unknown row kind <kind>".
func (b *Builtin) RowError(row map[string]any) string {
	byKey := make(map[string]*Column, len(b.Columns))
	for i := range b.Columns {
		byKey[b.Columns[i].Key] = &b.Columns[i]
	}
	keys := make([]string, 0, len(b.Columns))
	requiredHere := map[string]bool{}
	if len(b.RowKinds) == 0 {
		for _, c := range b.Columns {
			keys = append(keys, c.Key)
		}
	} else {
		kindKey, _ := row["kind"].(string)
		kind := b.rowKind(kindKey)
		if kind == nil {
			return fmt.Sprintf("%s: unknown row kind %q", b.Label, kindKey)
		}
		keys = kind.Columns
		for _, c := range kind.Required {
			requiredHere[c] = true
		}
	}
	for _, key := range keys {
		c := byKey[key]
		if c == nil || c.Hidden || c.Kind == "toggle" || c.Kind == "computed" {
			continue
		}
		if !c.Required && !requiredHere[key] {
			continue
		}
		if blankCell(row[key]) {
			return c.Label + " is required"
		}
	}
	return ""
}

func (b *Builtin) rowKind(key string) *RowKind {
	for i := range b.RowKinds {
		if b.RowKinds[i].Key == key {
			return &b.RowKinds[i]
		}
	}
	return nil
}

func (d *Defs) Host(key string) *Host {
	for i := range d.Hosts {
		if d.Hosts[i].Key == key {
			return &d.Hosts[i]
		}
	}
	return nil
}

// BuiltinKeys lists the host's built-in field keys in declaration order.
func (h *Host) BuiltinKeys() []string {
	out := make([]string, 0, len(h.Builtins))
	for _, b := range h.Builtins {
		out = append(out, b.Key)
	}
	return out
}

func (h *Host) Builtin(key string) *Builtin {
	for i := range h.Builtins {
		if h.Builtins[i].Key == key {
			return &h.Builtins[i]
		}
	}
	return nil
}

func (h *Host) HasColumn(name string) bool {
	for _, c := range h.Columns {
		if c == name {
			return true
		}
	}
	return false
}
