package forms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/core"
)

// DefaultFormID returns the id of a host's default form, or "" when none.
func DefaultFormID(app core.App, hostKey string) (string, error) {
	r, err := app.FindFirstRecordByFilter(formsCollection, "host = {:host} && is_default = true", dbx.Params{"host": hostKey})
	return defaultFormIDFrom(r, err)
}

// defaultFormIDFrom treats only "no row" as "no default"; any other error
// must stop the write, or validation would be skipped on a database fault.
func defaultFormIDFrom(r *core.Record, err error) (string, error) {
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("forms: default form lookup: %w", err)
	}
	return r.Id, nil
}

func hostHasForms(app core.App, hostKey string) (bool, error) {
	_, err := app.FindFirstRecordByFilter(formsCollection, "host = {:host}", dbx.Params{"host": hostKey})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("forms: host form lookup: %w", err)
	}
	return true, nil
}

func loadForm(app core.App, host *Host, id string) (*core.Record, *Definition, error) {
	rec, err := app.FindRecordById(formsCollection, id)
	if err != nil {
		return nil, nil, err
	}
	if rec.GetString("host") != host.Key {
		return nil, nil, fmt.Errorf("form %s belongs to %s", id, rec.GetString("host"))
	}
	def, err := ParseDefinition([]byte(rec.GetString("definition")))
	return rec, def, err
}

func fieldsOf(record *core.Record, column string) (map[string]any, error) {
	raw, err := json.Marshal(record.Get(column))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

// recordValues assembles the values the rules read: builtin columns from the
// record, custom keys from the JSON column. Evaluate needs JSON-decoded types,
// while record.Get returns ints, slices and DateTime, so the map is
// round-tripped through JSON.
func recordValues(host *Host, def *Definition, record *core.Record) (map[string]any, error) {
	fields, err := fieldsOf(record, host.FieldsColumn)
	if err != nil {
		return nil, err
	}
	values := map[string]any{}
	for _, f := range def.Fields {
		if f.Kind != "builtin" {
			values[f.Key] = fields[f.Key]
			continue
		}
		if b := host.Builtin(f.Key); b != nil && b.Storage == "column" {
			values[f.Key] = record.Get(f.Key)
		}
	}
	raw, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	normalized := map[string]any{}
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// otherFormField finds a custom field of another form of the same host, so a
// value kept across a type change is still checked against its own kind.
func otherFormField(ctx context.Context, app core.App, host *Host, key string) *Field {
	forms, err := app.FindRecordsByFilter(formsCollection, "host = {:host}", "", 0, 0, dbx.Params{"host": host.Key})
	if err != nil {
		log.WarnContext(ctx, "other form lookup failed", "host", host.Key, "err", err)
		return nil
	}
	for _, rec := range forms {
		odef, err := ParseDefinition([]byte(rec.GetString("definition")))
		if err != nil {
			log.WarnContext(ctx, "stored form definition does not parse", "form", rec.Id, "err", err)
			continue
		}
		if f := odef.Field(key); f != nil {
			return f
		}
	}
	return nil
}

// checkTable checks a table's rows. prev is the stored value (nil on
// create): like a field, a cell a write leaves as it was in the stored row
// with the same id is not checked again, so editing the columns (removing
// one, making one required) never blocks later writes. Computed cells are the
// server's, never the client's, so they are not checked.
func checkTable(f *Field, v, prev any) string {
	rows, ok := v.([]any)
	if !ok {
		return f.Label + " must be a list of rows"
	}
	cols := map[string]Column{}
	for _, c := range f.Columns {
		cols[c.Key] = c
	}
	storedRows := map[string]map[string]any{}
	for _, r := range rowsOf(prev) {
		if id, ok := r["id"].(string); ok {
			storedRows[id] = r
		}
	}
	for _, r := range rows {
		m, ok := r.(map[string]any)
		if !ok {
			return f.Label + " has a bad row"
		}
		id, ok := m["id"].(string)
		if !ok || id == "" {
			return f.Label + " has a row without an id"
		}
		old, isOld := storedRows[id]
		unchanged := func(key string) bool {
			if !isOld {
				return false
			}
			ov, had := old[key]
			nv, has := m[key]
			return had == has && reflect.DeepEqual(ov, nv)
		}
		for k, cv := range m {
			if k == "id" || unchanged(k) {
				continue
			}
			c, known := cols[k]
			if !known {
				return f.Label + " has an unknown column " + k
			}
			if c.Kind == "computed" {
				continue
			}
			cf := Field{Key: c.Key, Kind: c.Kind, Label: c.Label, Options: c.Options, Min: c.Min, Max: c.Max, MaxLength: c.MaxLength}
			if msg := checkKind(&cf, cv); msg != "" {
				return f.Label + ": " + msg
			}
		}
		for _, c := range f.Columns {
			if !c.Required || c.Hidden || c.Kind == "computed" || c.Kind == "toggle" || unchanged(c.Key) {
				continue
			}
			if blankCell(m[c.Key]) {
				return f.Label + ": " + c.Label + " is required"
			}
		}
	}
	return ""
}

// blankCell mirrors a required column in values.ts: text is trimmed first,
// and a toggle is never required (off is a value).
func blankCell(v any) bool {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s) == ""
	}
	return v == nil
}

func checkKind(f *Field, v any) string {
	if v == nil {
		return ""
	}
	switch f.Kind {
	case "date":
		s, ok := v.(string)
		if !ok || !dateRe.MatchString(s) {
			return f.Label + " must be a date (YYYY-MM-DD)"
		}
	case "text", "textarea", "richtext", "markdown":
		s, ok := v.(string)
		if !ok {
			return f.Label + " must be text"
		}
		if f.MaxLength != nil && len([]rune(s)) > *f.MaxLength {
			return f.Label + " is too long"
		}
	case "number":
		n, ok := v.(float64)
		if !ok {
			return f.Label + " must be a number"
		}
		if f.Min != nil && n < *f.Min {
			return f.Label + " is below its minimum"
		}
		if f.Max != nil && n > *f.Max {
			return f.Label + " is above its maximum"
		}
	case "toggle":
		if _, ok := v.(bool); !ok {
			return f.Label + " must be on or off"
		}
	case "select":
		allowed := map[string]bool{}
		for _, o := range f.Options {
			allowed[o.Value] = true
		}
		check := func(x any) bool { s, ok := x.(string); return ok && allowed[s] }
		if f.Multiple {
			arr, ok := v.([]any)
			if !ok {
				return f.Label + " must be a list"
			}
			for _, x := range arr {
				if !check(x) {
					return f.Label + " has a value that is not an option"
				}
			}
		} else if !check(v) {
			return f.Label + " is not one of its options"
		}
	case "user", "file":
		if f.Multiple {
			if _, ok := v.([]any); !ok {
				return f.Label + " must be a list"
			}
		} else if _, ok := v.(string); !ok {
			return f.Label + " must be an id"
		}
	case "computed":
		if _, ok := v.(float64); !ok {
			return f.Label + " must be a number"
		}
	case "checklist":
		arr, ok := v.([]any)
		if !ok {
			return f.Label + " must be a list"
		}
		for _, it := range arr {
			m, ok := it.(map[string]any)
			if !ok {
				return f.Label + " has a bad item"
			}
			if _, ok := m["id"].(string); !ok {
				return f.Label + " has an item without an id"
			}
		}
	}
	return ""
}

func evaluateAt(def *Definition, values map[string]any, today string) (State, error) {
	return Evaluate(def, values, Context{Today: today})
}

// withRuleOptions returns the field as the rules see it: a select whose
// options a set_options rule replaced is checked against that list.
func withRuleOptions(f *Field, state State) *Field {
	opts := state.Fields[f.Key].Options
	if f.Kind != "select" || opts == nil {
		return f
	}
	cp := *f
	cp.Options = opts
	return &cp
}

// enforcedBuiltin reports whether a required built-in is checked here: only
// column built-ins carry a value in the write. Rows, sections and
// collaborative text live elsewhere and may be empty in the record.
func enforcedBuiltin(host *Host, key string) bool {
	b := host.Builtin(key)
	return b != nil && b.Storage == "column" && !b.Collaborative && b.Kind != "section"
}

// checkBuiltinLimits refuses a column text built-in whose value the write
// changes to more characters than its MaxLength, keyed by the column. A host's
// text column has a length the database enforces; the form says so before
// the save, and every other writer gets the same refusal. As with the custom
// fields, an update that leaves a stored value as it was is not checked again.
func checkBuiltinLimits(host *Host, def *Definition, values, prevValues map[string]any, isCreate bool, errs validation.Errors) {
	for _, f := range def.Fields {
		if f.Kind != "builtin" {
			continue
		}
		b := host.Builtin(f.Key)
		if b == nil || b.Kind != "text" || b.Storage != "column" || b.MaxLength == nil {
			continue
		}
		v := values[f.Key]
		if !isCreate && reflect.DeepEqual(prevValues[f.Key], v) {
			continue
		}
		if msg := checkKind(&Field{Key: b.Key, Kind: "text", Label: f.Label, MaxLength: b.MaxLength}, v); msg != "" {
			errs[f.Key] = validation.NewError("validation_wrong_kind", msg)
		}
	}
}

func validateHostRecord(app core.App, host *Host, e *core.RecordEvent, isCreate bool) error {
	formID := e.Record.GetString(host.FormColumn)
	if formID == "" {
		id, err := DefaultFormID(app, host.Key)
		if err != nil {
			return err
		}
		if id == "" {
			has, err := hostHasForms(app, host.Key)
			if err != nil {
				return err
			}
			if !has {
				return nil // no forms seeded yet; nothing to validate against
			}
			return fieldErr(host.FormColumn, "validation_form_required", "Choose a form for this record.")
		}
		e.Record.Set(host.FormColumn, id)
		formID = id
	}
	_, def, err := loadForm(app, host, formID)
	if err != nil {
		return fieldErr(host.FormColumn, "validation_unknown_form", "This form does not belong to this record type.")
	}
	fields, err := fieldsOf(e.Record, host.FieldsColumn)
	if err != nil {
		return fieldErr(host.FieldsColumn, "validation_invalid_json", "Custom fields are not valid JSON.")
	}
	rulesFailed := fieldErr(host.FieldsColumn, "validation_rules_failed", "The form rules could not be evaluated.")
	today := time.Now().Format("2006-01-02")
	values, err := recordValues(host, def, e.Record)
	if err != nil {
		return rulesFailed
	}
	computed := ApplyComputed(def, host, values)
	storeComputed(host, def, e.Record, fields, computed)
	values = computed
	state, err := evaluateAt(def, values, today)
	if err != nil {
		return rulesFailed
	}
	// On update the stored row is the baseline: a value the write leaves as
	// it was is not checked again, so editing the form (deleting a field,
	// removing an option, tightening a limit) never blocks later writes.
	// Spec Part 7: "Field deleted: values stay".
	var prev, prevValues map[string]any
	var wasRequired map[string]bool
	if !isCreate {
		stored, err := app.FindRecordById(host.Collection, e.Record.Id)
		if err != nil {
			return fmt.Errorf("forms: read stored record: %w", err)
		}
		if prev, err = fieldsOf(stored, host.FieldsColumn); err != nil {
			return fmt.Errorf("forms: read stored record: %w", err)
		}
		if prevValues, err = recordValues(host, def, stored); err != nil {
			return rulesFailed
		}
		prevState, err := evaluateAt(def, prevValues, today)
		if err != nil {
			return rulesFailed
		}
		wasRequired = map[string]bool{}
		for key, fs := range prevState.Fields {
			wasRequired[key] = fs.Required
		}
	}
	errs := validation.Errors{}
	for key, v := range fields {
		if old, had := prev[key]; !isCreate && had && reflect.DeepEqual(old, v) {
			continue
		}
		f := def.Field(key)
		if f == nil {
			f = otherFormField(e.Context, app, host, key)
			if f == nil {
				errs[host.FieldsColumn+"."+key] = validation.NewError("validation_unknown_field", "Unknown field "+key)
				continue
			}
		} else {
			f = withRuleOptions(f, state)
		}
		if f.Kind == "builtin" {
			errs[host.FieldsColumn+"."+key] = validation.NewError("validation_builtin_in_json", key+" is a built-in field and is not stored here")
			continue
		}
		var msg string
		if f.Kind == "table" {
			msg = checkTable(f, v, prev[key])
		} else {
			msg = checkKind(f, v)
		}
		if msg != "" {
			errs[host.FieldsColumn+"."+key] = validation.NewError("validation_wrong_kind", msg)
		}
	}
	checkBuiltinLimits(host, def, values, prevValues, isCreate, errs)
	if len(errs) > 0 {
		return errs
	}
	for _, f := range def.Fields {
		if !state.Fields[f.Key].Required {
			continue
		}
		// Built-in values are columns, keyed by their own name; custom values
		// live in the JSON column. Collaborative values live in a Yjs
		// document and may legitimately be empty here.
		errKey, now, before := host.FieldsColumn+"."+f.Key, fields[f.Key], prev[f.Key]
		if f.Kind == "builtin" {
			if !enforcedBuiltin(host, f.Key) {
				continue
			}
			errKey, now, before = f.Key, values[f.Key], prevValues[f.Key]
		} else if f.Collaborative {
			continue
		}
		if !isEmpty(now) {
			continue
		}
		// An update may leave empty a field that was already required and
		// empty (the form was edited after the record was created).
		if isCreate || !wasRequired[f.Key] || !isEmpty(before) {
			errs[errKey] = validation.NewError("validation_required", f.Label+" is required")
		}
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// keepRoomOwnedValues puts back the stored value of every collaborative
// custom field of the record's current form. The collaboration room is the
// only writer of those keys (its flush is a plain app.Save, which never
// reaches this request hook); a client patch carries a copy that may be older
// than the room's last flush and must not overwrite it.
func keepRoomOwnedValues(e *core.RecordRequestEvent, host *Host) error {
	stored := e.Record.Original()
	formID := stored.GetString(host.FormColumn)
	if formID == "" {
		return nil
	}
	_, def, err := loadForm(e.App, host, formID)
	if err != nil {
		// Without the stored form there is no way to tell which keys the room
		// owns; the write goes on and validateHostRecord checks it against
		// the form it names.
		log.WarnContext(e.Request.Context(), "stored form unreadable; collaborative keys unguarded", "form", formID, "err", err)
		return nil
	}
	prev, err := fieldsOf(stored, host.FieldsColumn)
	if err != nil {
		return fmt.Errorf("forms: read stored record: %w", err)
	}
	incoming, err := fieldsOf(e.Record, host.FieldsColumn)
	if err != nil {
		return fieldErr(host.FieldsColumn, "validation_invalid_json", "Custom fields are not valid JSON.")
	}
	changed := false
	for _, f := range def.Fields {
		if f.Kind == "builtin" || !f.Collaborative {
			continue
		}
		old, had := prev[f.Key]
		now, has := incoming[f.Key]
		if had == has && reflect.DeepEqual(old, now) {
			continue
		}
		changed = true
		if had {
			incoming[f.Key] = old
		} else {
			delete(incoming, f.Key)
		}
	}
	if changed {
		e.Record.Set(host.FieldsColumn, incoming)
	}
	return nil
}

// keepHostOwnedValues undoes a request's write to every computed value the
// server cannot compute because it reads rows of a host table (directly or
// through another such value). The host's own code owns those values and sets
// them with app.Save, which never reaches this request hook. On create they
// start empty; on update they keep the stored value.
func keepHostOwnedValues(e *core.RecordRequestEvent, host *Host, isCreate bool) error {
	formID := e.Record.GetString(host.FormColumn)
	if formID == "" {
		id, err := DefaultFormID(e.App, host.Key)
		if err != nil {
			return err
		}
		formID = id
	}
	if formID == "" {
		return nil
	}
	_, def, err := loadForm(e.App, host, formID)
	if err != nil {
		// validateHostRecord rejects a form that does not load.
		return nil
	}
	invalidJSON := fieldErr(host.FieldsColumn, "validation_invalid_json", "Custom fields are not valid JSON.")
	values, err := recordValues(host, def, e.Record)
	if err != nil {
		return invalidJSON
	}
	_, skipped := applyComputed(def, host, values)
	if len(skipped) == 0 {
		return nil
	}
	incoming, err := fieldsOf(e.Record, host.FieldsColumn)
	if err != nil {
		return invalidJSON
	}
	stored := e.Record.Original()
	prev := map[string]any{}
	if !isCreate {
		if prev, err = fieldsOf(stored, host.FieldsColumn); err != nil {
			return fmt.Errorf("forms: read stored record: %w", err)
		}
	}
	changed := false
	for key := range skipped {
		if def.Field(key).Kind == "builtin" {
			if isCreate {
				e.Record.Set(key, nil)
			} else {
				e.Record.Set(key, stored.Get(key))
			}
			continue
		}
		old, had := prev[key]
		now, has := incoming[key]
		if had == has && reflect.DeepEqual(old, now) {
			continue
		}
		changed = true
		if had {
			incoming[key] = old
		} else {
			delete(incoming, key)
		}
	}
	if changed {
		e.Record.Set(host.FieldsColumn, incoming)
	}
	return nil
}

func bindHostHooks(app core.App, defs *Defs) {
	for i := range defs.Hosts {
		host := &defs.Hosts[i]
		app.OnRecordCreateRequest(host.Collection).BindFunc(func(e *core.RecordRequestEvent) error {
			if err := keepHostOwnedValues(e, host, true); err != nil {
				return err
			}
			return e.Next()
		})
		app.OnRecordUpdateRequest(host.Collection).BindFunc(func(e *core.RecordRequestEvent) error {
			if err := keepRoomOwnedValues(e, host); err != nil {
				return err
			}
			if err := keepHostOwnedValues(e, host, false); err != nil {
				return err
			}
			return e.Next()
		})
		app.OnRecordCreate(host.Collection).BindFunc(func(e *core.RecordEvent) error {
			if err := validateHostRecord(e.App, host, e, true); err != nil {
				return err
			}
			return e.Next()
		})
		app.OnRecordUpdate(host.Collection).BindFunc(func(e *core.RecordEvent) error {
			if err := validateHostRecord(e.App, host, e, false); err != nil {
				return err
			}
			return e.Next()
		})
	}
}
