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
	// Chrome is presentation only ("cell" or "bare"); the server decodes it
	// so the host file round-trips, and never reads it.
	Chrome string `json:"chrome,omitempty"`
	// Expression, Decimals and Columns mirror the computed and table built-ins
	// of core/lib/forms/host.ts.
	Expression any      `json:"expression,omitempty"`
	Decimals   int      `json:"decimals,omitempty"`
	Columns    []Column `json:"columns,omitempty"`
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
// full width.
type LayoutField struct {
	Page  string `json:"page,omitempty"`
	Width string `json:"width,omitempty"`
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

func (d *Defs) Host(key string) *Host {
	for i := range d.Hosts {
		if d.Hosts[i].Key == key {
			return &d.Hosts[i]
		}
	}
	return nil
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
