package automation

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
	"github.com/spf13/cast"
)

// RowValue is one labelled value of a trigger's row: what an action that
// writes a table row (a spreadsheet) puts under a header.
type RowValue struct {
	Label string
	Type  string
	// Value is a string (text, select, relation names, a date as YYYY-MM-DD
	// or RFC3339), a float64, a bool, or nil when empty.
	Value any
}

// TriggerRow returns the trigger record's exposed fields, with the labels
// and types the rule builder shows, then the run's dynamic fields (a web
// form's answers). A relation gives the names of the records it points at
// that the rule owner may see, as a template does.
func TriggerRow(app core.App, req ActionRequest) []RowValue {
	out := []RowValue{}
	if req.Record != nil {
		namer := &recordNamer{env: TemplateEnv{App: app, Labels: defsLabels(app), OwnerID: req.OwnerID}}
		for _, f := range resolveTriggerFields(app, req.Record.Collection(), req.Trigger) {
			out = append(out, RowValue{Label: f.Label, Type: f.Type, Value: recordValue(req.Record, f, namer)})
		}
	}
	for _, f := range req.Fields {
		out = append(out, RowValue{Label: f.Label, Type: f.Type, Value: dynamicValue(f)})
	}
	return out
}

// defsLabels is the record labels of app's engine, for naming relations.
func defsLabels(app core.App) map[string]RecordLabelDecl {
	if d, ok := defsKey.GetOk(app); ok && d != nil {
		return d.RecordLabels
	}
	return nil
}

func recordValue(rec *core.Record, f catalogField, namer *recordNamer) any {
	switch f.Type {
	case "number":
		return rec.GetFloat(f.Key)
	case "boolean":
		return rec.GetBool(f.Key)
	case "date":
		return dateText(rec.GetDateTime(f.Key))
	case "relation":
		if field, ok := rec.Collection().Fields.GetByName(f.Key).(*core.RelationField); ok {
			return emptyToNil(namer.relationText(field, rec))
		}
		return nil
	default:
		return emptyToNil(normalize(rec.Get(f.Key)))
	}
}

// dynamicValue reads one answer. An empty answer, or one that is not a
// number for a number field, is nil, so its cell stays empty rather than
// showing 0 or FALSE.
func dynamicValue(f DynamicField) any {
	switch f.Type {
	case "number":
		if n, ok := toFloat(f.Value); ok {
			return n
		}
		return nil
	case "boolean":
		if f.Value == nil || f.Value == "" {
			return nil
		}
		return cast.ToBool(f.Value)
	default:
		return emptyToNil(normalize(f.Value))
	}
}

func dateText(d types.DateTime) any {
	if d.IsZero() {
		return nil
	}
	t := d.Time().UTC()
	if t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 {
		return t.Format("2006-01-02")
	}
	return t.Format(time.RFC3339)
}

func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
