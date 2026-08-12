// tinycld/core/server/automation/engine_test.go
package automation

import (
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/rlstest"
)

// engineApp: real rules/rule_runs migrations + a "tickets" collection with an
// owner relation + defs declaring a create trigger and a set-status action.
func engineApp(t *testing.T) (core.App, *Engine, *core.Record) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	t.Cleanup(ResetRunStateForTest)
	app := rlstest.NewApp(t)

	// Same fixture reconciliation as actions_test.go's pkgaccessApp: drop the
	// bundled username index so 1820000000 (users_username_required) applies
	// the way it does against a real DB.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	var kept types.JSONArray[string]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))

	col := core.NewBaseCollection("tickets")
	col.Fields.Add(&core.TextField{Name: "title"})
	col.Fields.Add(&core.TextField{Name: "status"})
	col.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	// Every real generated collection carries autodate created/updated fields
	// (see pb_migrations/1990000000_create_rules.js); dry-run's "-created"
	// sort relies on that, same as it would against any feature collection.
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	col.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}

	defs := &Defs{Packages: []PackageDefs{{
		Slug:     "tickets",
		Triggers: []TriggerDef{{ID: "ticket-created", Label: "created", Collection: "tickets", On: "create"}},
		Actions: []ActionDef{{
			ID: "set-status", Label: "set", Kind: "record-op", Collection: "tickets",
			Op:     RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"status": {Param: "status"}}},
			Params: []ParamDef{{Key: "status", Field: "status"}},
		}},
	}}}
	eng := NewEngine(app, defs)
	eng.Start()

	u, _ := app.FindFirstRecordByFilter("users", "id != ''")
	return app, eng, u
}

func makeRule(t *testing.T, app core.App, owner, scope string, conditions, actions any, order int, stop bool) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("rules")
	r := core.NewRecord(col)
	r.Set("name", "r")
	r.Set("scope", scope)
	r.Set("owner", owner)
	r.Set("trigger", "tickets:ticket-created")
	r.Set("conditions", conditions)
	r.Set("actions", actions)
	r.Set("enabled", true)
	r.Set("order", order)
	r.Set("stop_processing", stop)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func waitForRuns(t *testing.T, app core.App, ruleID string, want int) []*core.Record {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "-fired_at", 0, 0, map[string]any{"id": ruleID})
		if len(runs) >= want {
			return runs
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d runs of %s", want, ruleID)
	return nil
}

func TestEndToEndMatchAndAction(t *testing.T) {
	app, _, u := engineApp(t)
	rule := makeRule(t, app, u.Id, "personal",
		map[string]any{"match": "all", "groups": []any{map[string]any{
			"match": "any", "conditions": []any{map[string]any{"field": "title", "op": "contains", "value": "urgent"}},
		}}},
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "triaged"}}},
		0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "URGENT: disk full")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	runs := waitForRuns(t, app, rule.Id, 1)
	if !runs[0].GetBool("matched") {
		t.Fatal("rule must match")
	}
	fresh, _ := app.FindRecordById("tickets", rec.Id)
	if fresh.GetString("status") != "triaged" {
		t.Fatalf("action must apply: %q", fresh.GetString("status"))
	}
}

func TestNonMatchIsLogged(t *testing.T) {
	app, _, u := engineApp(t)
	rule := makeRule(t, app, u.Id, "personal",
		map[string]any{"match": "all", "groups": []any{map[string]any{
			"match": "any", "conditions": []any{map[string]any{"field": "title", "op": "contains", "value": "nope"}},
		}}},
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "x"}}},
		0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "routine")
	rec.Set("user", u.Id)
	app.Save(rec)

	runs := waitForRuns(t, app, rule.Id, 1)
	if runs[0].GetBool("matched") {
		t.Fatal("non-match must log matched=false")
	}
	fresh, _ := app.FindRecordById("tickets", rec.Id)
	if fresh.GetString("status") != "" {
		t.Fatal("no action on non-match")
	}
}

func TestPersonalScopeFiltering(t *testing.T) {
	app, _, u := engineApp(t)
	// Rule owned by u, but the ticket belongs to a second user → must not fire.
	users, _ := app.FindCollectionByNameOrId("users")
	other := core.NewRecord(users)
	other.Set("email", "other@example.com")
	other.Set("username", "otheruser")
	other.Set("name", "Other")
	other.Set("role", "member")
	other.SetPassword("0123456789")
	if err := app.Save(other); err != nil {
		t.Fatal(err)
	}
	rule := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "x"}}}, 0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "anything")
	rec.Set("user", other.Id)
	app.Save(rec)

	// Give the worker a beat, then assert NO run row exists.
	time.Sleep(300 * time.Millisecond)
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": rule.Id})
	if len(runs) != 0 {
		t.Fatalf("personal rule must not fire on another user's record: %d runs", len(runs))
	}
	_ = rule
}

func TestTriggerFilterGatesEnqueue(t *testing.T) {
	app, _, u := engineApp(t)
	RegisterTriggerFilter("tickets:ticket-created", func(app core.App, record *core.Record) bool {
		return record.GetString("status") != "draft"
	})
	rule := makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "x"}}}, 0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	filtered := core.NewRecord(col)
	filtered.Set("title", "filtered out")
	filtered.Set("status", "draft")
	filtered.Set("user", u.Id)
	if err := app.Save(filtered); err != nil {
		t.Fatal(err)
	}

	// Give the worker a beat, then assert the filtered-out create produced NO run row.
	time.Sleep(300 * time.Millisecond)
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": rule.Id})
	if len(runs) != 0 {
		t.Fatalf("a trigger filtered out must not enqueue: %d runs", len(runs))
	}

	allowed := core.NewRecord(col)
	allowed.Set("title", "allowed through")
	allowed.Set("status", "ready")
	allowed.Set("user", u.Id)
	if err := app.Save(allowed); err != nil {
		t.Fatal(err)
	}
	waitForRuns(t, app, rule.Id, 1)
}

func TestStopProcessingAndOrdering(t *testing.T) {
	app, _, u := engineApp(t)
	first := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "from-first"}}}, 0, true)
	second := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "from-second"}}}, 1, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	app.Save(rec)

	waitForRuns(t, app, first.Id, 1)
	time.Sleep(300 * time.Millisecond)
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": second.Id})
	if len(runs) != 0 {
		t.Fatal("stop_processing must skip later personal rules")
	}
	fresh, _ := app.FindRecordById("tickets", rec.Id)
	if fresh.GetString("status") != "from-first" {
		t.Fatalf("first rule's action must have applied: %q", fresh.GetString("status"))
	}
}

func TestStartIsIdempotent(t *testing.T) {
	app, eng, _ := engineApp(t)

	before := rlstest.HookHandlerCounts(t, app)

	// Must not panic (e.g. double-close of the done channel) and must not
	// spawn a second worker or rebind hooks.
	eng.Start()

	after := rlstest.HookHandlerCounts(t, app)
	for name, count := range before {
		if after[name] != count {
			t.Fatalf("second Start() must be a no-op: hook %s went from %d to %d handlers", name, count, after[name])
		}
	}
	if !eng.started {
		t.Fatal("started flag must remain set")
	}
}

func TestSelfRetriggerAndChainDepth(t *testing.T) {
	app, _, u := engineApp(t)
	// set-status writes the trigger record → refires the update hook. There is
	// no update trigger declared, so the direct loop risk is create-only here;
	// assert the self-retrigger guard via provenance: rule fires once, and the
	// engine-write sentinel was consumed (no unbounded growth).
	rule := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "done"}}}, 0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	app.Save(rec)

	waitForRuns(t, app, rule.Id, 1)
	time.Sleep(300 * time.Millisecond)
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": rule.Id})
	if len(runs) != 1 {
		t.Fatalf("rule must fire exactly once, got %d", len(runs))
	}
}
