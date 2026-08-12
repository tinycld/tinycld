// tinycld/core/server/automation/eval_test.go
package automation

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func evalRecord(t *testing.T) (*tests.TestApp, *core.Record) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	col := core.NewBaseCollection("eval_things")
	col.Fields.Add(&core.TextField{Name: "subject"})
	col.Fields.Add(&core.TextField{Name: "sender"})
	col.Fields.Add(&core.BoolField{Name: "flagged"})
	col.Fields.Add(&core.NumberField{Name: "size"})
	col.Fields.Add(&core.DateField{Name: "happened"})
	col.Fields.Add(&core.SelectField{Name: "tags", Values: []string{"a", "b", "c"}, MaxSelect: 3})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("subject", "Invoice #42 attached")
	r.Set("sender", "billing@ACME.com")
	r.Set("flagged", true)
	r.Set("size", 1500)
	r.Set("happened", time.Now().Add(-48*time.Hour).UTC().Format("2006-01-02 15:04:05.000Z"))
	r.Set("tags", []string{"a", "c"})
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return app, r
}

func cond(field, op string, value any) Condition {
	return Condition{Field: field, Op: op, Value: value}
}

func one(c Condition) ConditionsAST {
	return ConditionsAST{Match: "all", Groups: []ConditionGroup{{Match: "all", Conditions: []Condition{c}}}}
}

func TestOperatorTable(t *testing.T) {
	_, r := evalRecord(t)
	cases := []struct {
		name string
		c    Condition
		want bool
	}{
		{"contains ci", cond("subject", "contains", "invoice"), true},
		{"contains miss", cond("subject", "contains", "receipt"), false},
		{"not_contains", cond("subject", "not_contains", "receipt"), true},
		{"equals ci", cond("sender", "equals", "Billing@acme.com"), true},
		{"starts_with", cond("subject", "starts_with", "invoice #"), true},
		{"eq", cond("size", "eq", 1500), true},
		{"neq", cond("size", "neq", 1500), false},
		{"gt", cond("size", "gt", 1000), true},
		{"lt", cond("size", "lt", 1000), false},
		{"is_true", cond("flagged", "is_true", nil), true},
		{"is_false", cond("flagged", "is_false", nil), false},
		{"within_last_days hit", cond("happened", "within_last_days", 7), true},
		{"within_last_days miss", cond("happened", "within_last_days", 1), false},
		{"is multi-match", cond("tags", "is", "c"), true},
		{"is multi-miss", cond("tags", "is", "b"), false},
		{"is_not", cond("tags", "is_not", "b"), true},
		{"is_empty miss", cond("subject", "is_empty", nil), false},
		{"unknown op", cond("subject", "regex", ".*"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := EvaluateConditions(one(tc.c), r); got != tc.want {
				t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestGroupSemantics(t *testing.T) {
	_, r := evalRecord(t)
	hit := cond("subject", "contains", "invoice")
	miss := cond("subject", "contains", "receipt")

	anyGroup := ConditionGroup{Match: "any", Conditions: []Condition{miss, hit}}
	allGroup := ConditionGroup{Match: "all", Conditions: []Condition{hit, miss}}

	if !EvaluateConditions(ConditionsAST{Match: "all", Groups: []ConditionGroup{anyGroup}}, r) {
		t.Fatal("any-group with one hit must pass")
	}
	if EvaluateConditions(ConditionsAST{Match: "all", Groups: []ConditionGroup{anyGroup, allGroup}}, r) {
		t.Fatal("all-of-groups with a failing group must fail")
	}
	if !EvaluateConditions(ConditionsAST{Match: "any", Groups: []ConditionGroup{anyGroup, allGroup}}, r) {
		t.Fatal("any-of-groups with a passing group must pass")
	}
	if !EvaluateConditions(ConditionsAST{}, r) {
		t.Fatal("empty AST must pass (no conditions = always match)")
	}
}

func TestWatchChanged(t *testing.T) {
	app, r := evalRecord(t)
	r.Set("subject", "Changed subject")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	// After Save, Original() reflects pre-save state only inside hooks; emulate
	// by building the comparison directly: load fresh, mutate in memory.
	fresh, err := app.FindRecordById("eval_things", r.Id)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Set("sender", "other@acme.com")
	if !WatchChanged(fresh, []string{"sender"}) {
		t.Fatal("watched field changed → true")
	}
	if WatchChanged(fresh, []string{"subject"}) {
		t.Fatal("unwatched-change only → false")
	}
	if !WatchChanged(fresh, nil) {
		t.Fatal("empty watch → always true")
	}
}

func TestDecodeConditions(t *testing.T) {
	ast, err := DecodeConditions(map[string]any{
		"match": "all",
		"groups": []any{map[string]any{
			"match":      "any",
			"conditions": []any{map[string]any{"field": "subject", "op": "contains", "value": "x"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ast.Groups[0].Conditions[0].Field != "subject" {
		t.Fatalf("decode failed: %+v", ast)
	}
	empty, err := DecodeConditions(nil)
	if err != nil || len(empty.Groups) != 0 {
		t.Fatalf("nil decodes to empty AST: %+v %v", empty, err)
	}
}
