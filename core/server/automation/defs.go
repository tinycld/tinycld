// tinycld/core/server/automation/defs.go
//
// Wire types for server/automation_defs.json, the generator's materialization
// of every package's automation.ts (plus core's built-ins). JSON-tagged
// mirrors, same rationale as tenantcfg's DAV mirrors: the TS side owns the
// authoring format, Go consumes a stable wire shape.
package automation

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// FieldRef decodes both wire forms: "subject" and {"key":..., "label":...}.
type FieldRef struct {
	Key   string
	Label string
}

func (f *FieldRef) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &f.Key)
	}
	var obj struct {
		Key   string `json:"key"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		return err
	}
	f.Key, f.Label = obj.Key, obj.Label
	return nil
}

// SetValue decodes {param}, {context}, or a bare literal.
type SetValue struct {
	Param   string
	Context string
	Literal any
}

func (s *SetValue) UnmarshalJSON(b []byte) error {
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err == nil {
		if p, ok := obj["param"].(string); ok {
			s.Param = p
			return nil
		}
		if c, ok := obj["context"].(string); ok {
			s.Context = c
			return nil
		}
	}
	return json.Unmarshal(b, &s.Literal)
}

type TriggerDef struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Collection string     `json:"collection"`
	On         string     `json:"on"`
	Watch      []string   `json:"watch"`
	Fields     []FieldRef `json:"fields"`
	OwnerField string     `json:"ownerField"`
	Synthetic  string     `json:"synthetic"`
}

type RecordOp struct {
	Type   string              `json:"type"`
	Target string              `json:"target"`
	Set    map[string]SetValue `json:"set"`
}

type ParamDef struct {
	Key     string   `json:"key"`
	Field   string   `json:"field"`
	Type    string   `json:"type"`
	Label   string   `json:"label"`
	Options []string `json:"options"`
	// RelationTarget names the collection a typed relation param picks from.
	// Column params leave it empty — their target resolves from the column.
	RelationTarget string `json:"relationTarget"`
}

type ActionDef struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Kind       string     `json:"kind"`
	Collection string     `json:"collection"`
	Op         RecordOp   `json:"op"`
	Params     []ParamDef `json:"params"`
}

type PackageDefs struct {
	Slug     string       `json:"slug"`
	Triggers []TriggerDef `json:"triggers"`
	Actions  []ActionDef  `json:"actions"`
}

type Defs struct {
	Packages []PackageDefs `json:"packages"`
}

type QualifiedTrigger struct {
	Ref string
	Pkg string
	Def TriggerDef
}

// LoadDefs reads the materialized defs. A missing file is an inert engine,
// not an error — matches tenantcfg.loadJSON: a workspace with no automation
// packages simply has nothing to do.
func LoadDefs(path string) (*Defs, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Defs{}, nil
		}
		return nil, fmt.Errorf("automation: read defs: %w", err)
	}
	var defs Defs
	if err := json.Unmarshal(raw, &defs); err != nil {
		return nil, fmt.Errorf("automation: parse defs %s: %w", path, err)
	}
	return &defs, nil
}

func splitRef(ref string) (pkg, id string, ok bool) {
	i := strings.IndexByte(ref, ':')
	if i <= 0 || i == len(ref)-1 {
		return "", "", false
	}
	return ref[:i], ref[i+1:], true
}

func (d *Defs) Trigger(ref string) (TriggerDef, string, bool) {
	pkg, id, ok := splitRef(ref)
	if !ok {
		return TriggerDef{}, "", false
	}
	for _, p := range d.Packages {
		if p.Slug != pkg {
			continue
		}
		for _, t := range p.Triggers {
			if t.ID == id {
				return t, p.Slug, true
			}
		}
	}
	return TriggerDef{}, "", false
}

func (d *Defs) Action(ref string) (ActionDef, string, bool) {
	pkg, id, ok := splitRef(ref)
	if !ok {
		return ActionDef{}, "", false
	}
	for _, p := range d.Packages {
		if p.Slug != pkg {
			continue
		}
		for _, a := range p.Actions {
			if a.ID == id {
				return a, p.Slug, true
			}
		}
	}
	return ActionDef{}, "", false
}

func (d *Defs) TriggersFor(collection, op string) []QualifiedTrigger {
	var out []QualifiedTrigger
	for _, p := range d.Packages {
		for _, t := range p.Triggers {
			if t.Synthetic == "" && t.Collection == collection && t.On == op {
				out = append(out, QualifiedTrigger{Ref: p.Slug + ":" + t.ID, Pkg: p.Slug, Def: t})
			}
		}
	}
	return out
}
