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
	// tokenKey stands in for a curated-away/hidden secret column: rule
	// conditions must never be able to probe it via match/no-match.
	col.Fields.Add(&core.TextField{Name: "tokenKey", Hidden: true})
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
	r.Set("tokenKey", "super-secret-value")
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

// openEvalTrigger declares no Fields allowlist, so exposedFields exposes every
// non-system, non-hidden column of eval_things — the pre-existing behavior
// TestOperatorTable/TestGroupSemantics rely on.
var openEvalTrigger = TriggerDef{}

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
		{"eq partial-parse string rejected", cond("size", "eq", "1500abc"), false},
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
			if got := EvaluateConditions(one(tc.c), r, openEvalTrigger); got != tc.want {
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

	if !EvaluateConditions(ConditionsAST{Match: "all", Groups: []ConditionGroup{anyGroup}}, r, openEvalTrigger) {
		t.Fatal("any-group with one hit must pass")
	}
	if EvaluateConditions(ConditionsAST{Match: "all", Groups: []ConditionGroup{anyGroup, allGroup}}, r, openEvalTrigger) {
		t.Fatal("all-of-groups with a failing group must fail")
	}
	if !EvaluateConditions(ConditionsAST{Match: "any", Groups: []ConditionGroup{anyGroup, allGroup}}, r, openEvalTrigger) {
		t.Fatal("any-of-groups with a passing group must pass")
	}
	if !EvaluateConditions(ConditionsAST{}, r, openEvalTrigger) {
		t.Fatal("empty AST must pass (no conditions = always match)")
	}
}

// TestConditionsFailClosedOnNonExposedFields proves a rule condition can't be
// used as a match/no-match oracle to probe a field the trigger doesn't expose
// — hidden columns, and columns curated away by an explicit Fields allowlist.
func TestConditionsFailClosedOnNonExposedFields(t *testing.T) {
	_, r := evalRecord(t)

	// Hidden field: even under the open trigger (no Fields declared, so every
	// non-hidden column is exposed), tokenKey is hidden and must never match —
	// regardless of the actual value or operator outcome.
	hiddenHit := cond("tokenKey", "equals", "super-secret-value")
	if EvaluateConditions(one(hiddenHit), r, openEvalTrigger) {
		t.Fatal("condition on a hidden field must evaluate false even when the value would otherwise match")
	}
	hiddenMiss := cond("tokenKey", "equals", "definitely-not-it")
	if EvaluateConditions(one(hiddenMiss), r, openEvalTrigger) {
		t.Fatal("condition on a hidden field must stay false on the non-matching branch too (no oracle either direction)")
	}

	// Curated-away field: an allowlisted trigger that only exposes "subject"
	// must reject a condition on "sender", a real, non-hidden column that
	// simply isn't in the allowlist.
	curatedTrigger := TriggerDef{Fields: []FieldRef{{Key: "subject"}}}
	curatedAwayHit := cond("sender", "equals", "billing@acme.com")
	if EvaluateConditions(one(curatedAwayHit), r, curatedTrigger) {
		t.Fatal("condition on a field curated away by the trigger's allowlist must evaluate false")
	}

	// Exposed field: behavior for an allowlisted, actually-exposed field is
	// unchanged.
	exposedHit := cond("subject", "contains", "invoice")
	if !EvaluateConditions(one(exposedHit), r, curatedTrigger) {
		t.Fatal("condition on an exposed, allowlisted field must evaluate normally")
	}
}

// TestConditionsWithoutRecordFailClosed covers the scheduled and manual
// "run now" dispatch paths, which both fire with Record == nil. A rule saved
// with conditions still reaches EvaluateConditions on those paths, so every
// operator must fail closed instead of dereferencing the nil record.
func TestConditionsWithoutRecordFailClosed(t *testing.T) {
	if !EvaluateConditions(ConditionsAST{}, nil, openEvalTrigger) {
		t.Fatal("empty AST must still match without a record")
	}

	// One case per branch of evalCondition's operator switch: each of these
	// reads the record, so a missing nil guard panics rather than not-matching.
	for _, c := range []Condition{
		cond("subject", "contains", "invoice"),
		cond("subject", "not_contains", "invoice"),
		cond("subject", "equals", "invoice"),
		cond("subject", "starts_with", "invoice"),
		cond("size", "eq", 1500),
		cond("size", "neq", 1500),
		cond("size", "gt", 1),
		cond("size", "lt", 9999),
		cond("flagged", "is_true", nil),
		cond("flagged", "is_false", nil),
		cond("happened", "before", "2030-01-01"),
		cond("happened", "after", "2000-01-01"),
		cond("happened", "within_last_days", 7),
		cond("tags", "is", "a"),
		cond("tags", "is_not", "a"),
		cond("subject", "is_empty", nil),
	} {
		if EvaluateConditions(one(c), nil, openEvalTrigger) {
			t.Fatalf("op %q must fail closed without a record, got a match", c.Op)
		}
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
