// tinycld/core/server/automation/template.go
package automation

import (
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

var placeholderRe = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_]+)\s*\}\}`)

// exposedFields returns the set of field keys rule templates/conditions may
// see for this trigger: the declared allowlist, or (when the declaration
// omitted fields) every non-system column plus created/updated — mirroring
// the Phase 1 contract ("fields omitted = expose every schema column").
func exposedFields(record *core.Record, trigger TriggerDef) map[string]bool {
	out := map[string]bool{}
	if len(trigger.Fields) > 0 {
		for _, f := range trigger.Fields {
			out[f.Key] = true
		}
		return out
	}
	for _, field := range record.Collection().Fields {
		name := field.GetName()
		if name == "id" || strings.HasPrefix(name, "_") {
			continue
		}
		out[name] = true
	}
	return out
}

// SubstituteTemplates fills {{field}} placeholders from the trigger record.
// Non-exposed and unknown fields become empty strings: a template must never
// leak a column the trigger's declaration curated away.
func SubstituteTemplates(s string, record *core.Record, trigger TriggerDef) string {
	if record == nil || !strings.Contains(s, "{{") {
		return s
	}
	exposed := exposedFields(record, trigger)
	return placeholderRe.ReplaceAllStringFunc(s, func(m string) string {
		key := placeholderRe.FindStringSubmatch(m)[1]
		if !exposed[key] {
			return ""
		}
		return normalize(record.Get(key))
	})
}
