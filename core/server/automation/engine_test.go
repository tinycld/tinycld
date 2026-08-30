// tinycld/core/server/automation/engine_test.go
package automation

import (
	"encoding/json"
	"strings"
	"sync/atomic"
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
	allowAuthedWrites(t, app, col)

	// broadcasts: no user/owner/author relation at all — exercises the
	// unscoped dry-run path (TestDryRunUnscopedRequiresAdmin), which
	// ownerFilterFor can't resolve to a per-caller filter.
	broadcasts := core.NewBaseCollection("broadcasts")
	broadcasts.Fields.Add(&core.TextField{Name: "title"})
	broadcasts.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	broadcasts.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(broadcasts); err != nil {
		t.Fatal(err)
	}

	defs := &Defs{Packages: []PackageDefs{{
		Slug: "tickets",
		Triggers: []TriggerDef{
			{ID: "ticket-created", Label: "created", Collection: "tickets", On: "create"},
			{ID: "broadcast-created", Label: "broadcast", Collection: "broadcasts", On: "create"},
		},
		Actions: []ActionDef{{
			ID: "set-status", Label: "set", Kind: "record-op", Collection: "tickets",
			Op:     RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"status": {Param: "status"}}},
			Params: []ParamDef{{Key: "status", Field: "status"}},
		}, {
			// clone-ticket lets a test build a self-triggering chain: each
			// created ticket's rule creates another ticket owned by the same
			// user (so a personal rule keeps matching each generation),
			// re-firing the create hook — used to exercise the
			// chain-depth-exceeded path.
			ID: "clone-ticket", Label: "clone", Kind: "record-op", Collection: "tickets",
			Op: RecordOp{Type: "create", Set: map[string]SetValue{
				"title": {Literal: "clone"},
				"user":  {Context: "owner"},
			}},
		}, {
			// boom's handler panics, so a test can assert the engine records a
			// failed run rather than taking the process down with it.
			ID: "boom", Label: "boom", Kind: "native",
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

// TestPersonalStopIsScopedToItsOwner is the N5 regression. A shared-audience
// trigger (a team mailbox, a board everyone watches) dispatches ONE event to
// every member's personal rules. A personal stop_processing must therefore halt
// only its own owner's later rules — before the fix it set a global flag, so
// whoever sorted first silently switched off everybody else's automation.
func TestPersonalStopIsScopedToItsOwner(t *testing.T) {
	app, _, u := engineApp(t)

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

	// Both users are in the audience for every ticket — the shared-mailbox
	// shape, where one record legitimately belongs to several people.
	RegisterOwnerResolver("tickets:ticket-created", func(core.App, *core.Record) []string {
		return []string{u.Id, other.Id}
	})

	// u's rule stops processing; it sorts first, so before the fix it halted
	// the loop outright and other's rule never ran.
	stopper := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "from-u"}}}, 0, true)
	uLater := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "u-later"}}}, 1, false)
	otherRule := makeRule(t, app, other.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "from-other"}}}, 2, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "shared")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	waitForRuns(t, app, stopper.Id, 1)
	waitForRuns(t, app, otherRule.Id, 1)

	// u's own later rule is still skipped — that is what stop_processing means.
	time.Sleep(200 * time.Millisecond)
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0,
		map[string]any{"id": uLater.Id})
	if len(runs) != 0 {
		t.Fatalf("a personal stop must still skip its OWN owner's later rules, got %d runs", len(runs))
	}
}

// An org rule's stop is deliberately global: it speaks for the deployment, not
// for one person, so it halts every owner's personal rules downstream.
func TestOrgStopHaltsEveryOwner(t *testing.T) {
	app, _, u := engineApp(t)

	users, _ := app.FindCollectionByNameOrId("users")
	other := core.NewRecord(users)
	other.Set("email", "other2@example.com")
	other.Set("username", "otheruser2")
	other.Set("name", "Other Two")
	other.Set("role", "member")
	other.SetPassword("0123456789")
	if err := app.Save(other); err != nil {
		t.Fatal(err)
	}
	RegisterOwnerResolver("tickets:ticket-created", func(core.App, *core.Record) []string {
		return []string{u.Id, other.Id}
	})

	orgStop := makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "org"}}}, 0, true)
	personal := makeRule(t, app, other.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "personal"}}}, 1, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	waitForRuns(t, app, orgStop.Id, 1)
	time.Sleep(200 * time.Millisecond)
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0,
		map[string]any{"id": personal.Id})
	if len(runs) != 0 {
		t.Fatalf("an org stop must halt other owners' rules too, got %d runs", len(runs))
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

// TestChainDepthExceededRunRow drives a genuine chain of ticket creates past
// maxChainDepth, then asserts the resulting run row: written with an error,
// and — the fix under test — trigger_summary is empty rather than a snapshot
// of every column, because the chain-depth-exceeded WriteRun call passes a
// nil record instead of the real one under an empty TriggerDef (which would
// otherwise fall into the "expose everything" branch).
func TestChainDepthExceededRunRow(t *testing.T) {
	app, _, u := engineApp(t)
	// A single rule can't chain past depth 1: the "a rule never re-fires on
	// its own write" guard (dispatch's rule.Id == ev.SourceRule check) blocks
	// it. Two rules that both clone on every ticket-created ping-pong past
	// each other's sentinel instead, so depth keeps climbing past
	// maxChainDepth.
	makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:clone-ticket"}}, 0, false)
	makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:clone-ticket"}}, 1, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "seed")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	// Each generation's rule run enqueues another create from the OTHER
	// rule's next hop; wait for the chain to run past maxChainDepth (3) and
	// produce the depth-exceeded run row — distinguished from ordinary
	// matched runs by having a non-empty error. Either rule can end up as the
	// row's owner depending on ping-pong parity, so query across both.
	deadline := time.Now().Add(5 * time.Second)
	var errRun *core.Record
	for time.Now().Before(deadline) {
		runs, _ := app.FindRecordsByFilter("rule_runs", "error != ''", "", 0, 0, nil)
		if len(runs) > 0 {
			errRun = runs[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if errRun == nil {
		t.Fatal("timed out waiting for the chain-depth-exceeded run row")
	}
	if errRun.GetString("error") != "chain-depth-exceeded" {
		t.Fatalf("error string: %q", errRun.GetString("error"))
	}
	summary, _ := errRun.Get("trigger_summary").(map[string]any)
	if len(summary) != 0 {
		t.Fatalf("chain-depth-exceeded row must have an empty trigger_summary, got %+v", summary)
	}
}

// TestNativeWriteProvenanceStopsChain is the B2 regression: a native handler
// that creates a record in the collection its own trigger watches must stamp
// provenance via MarkEngineWrite so the chain terminates at maxChainDepth.
// Without the stamp every generation looks like a fresh user write (depth 0)
// and the handler recurses until the process dies.
func TestNativeWriteProvenanceStopsChain(t *testing.T) {
	app, _, u := engineApp(t)
	col, _ := app.FindCollectionByNameOrId("tickets")

	// Atomic: the handler runs on the engine's goroutine while the poll loop
	// below reads from the test's, so a plain int is a data race.
	var created atomic.Int64
	RegisterAction("tickets:boom", func(app core.App, req ActionRequest) error {
		if created.Add(1) > 50 {
			return nil // safety valve: without the fix this never terminates
		}
		made := core.NewRecord(col)
		made.Set("id", core.GenerateDefaultRandomId())
		made.Set("title", "native clone")
		made.Set("user", u.Id)
		return MarkEngineWrite(req, made.Id, func() error { return app.Save(made) })
	})
	makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:boom"}}, 0, false)

	seed := core.NewRecord(col)
	seed.Set("title", "seed")
	seed.Set("user", u.Id)
	if err := app.Save(seed); err != nil {
		t.Fatal(err)
	}

	// The chain must go quiet on its own. Poll until the handler stops being
	// invoked, then assert it stopped because of the depth cap rather than the
	// safety valve.
	stable, last := 0, int64(-1)
	for range 100 {
		if now := created.Load(); now == last {
			if stable++; stable >= 5 {
				break
			}
		} else {
			stable, last = 0, now
		}
		time.Sleep(50 * time.Millisecond)
	}
	total := created.Load()
	if total == 0 {
		t.Fatal("the native action never ran")
	}
	if total > int64(maxChainDepth+1) {
		t.Fatalf("native writes must stop at the depth cap, handler ran %d times", total)
	}
}

// TestActionPanicRecordsFailedRun proves a panicking native handler becomes a
// failed action result instead of a dead process. The handler runs on its own
// goroutine, so only its own recover can contain it.
func TestActionPanicRecordsFailedRun(t *testing.T) {
	app, _, u := engineApp(t)
	RegisterAction("tickets:boom", func(core.App, ActionRequest) error {
		panic("handler exploded")
	})
	rule := makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:boom"}}, 0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	runs := waitForRuns(t, app, rule.Id, 1)
	if !runs[0].GetBool("matched") {
		t.Fatal("the rule matched, so the run row must say so")
	}
	var results []ActionResult
	if err := json.Unmarshal([]byte(runs[0].GetString("results")), &results); err != nil {
		t.Fatalf("decode results: %v", err)
	}
	if len(results) != 1 || results[0].Status != "error" {
		t.Fatalf("panicking action must record an error result, got %+v", results)
	}
	if !strings.Contains(results[0].Message, "panicked") {
		t.Fatalf("result message should name the panic, got %q", results[0].Message)
	}
}

// TestDispatchPanicDoesNotKillWorker proves the worker survives a panic raised
// in dispatch itself (outside the action goroutine) and keeps serving later
// events. Before the recover, one such panic unwound the only queue consumer
// and every subsequent trigger went unserved.
func TestDispatchPanicDoesNotKillWorker(t *testing.T) {
	app, eng, u := engineApp(t)

	// An owner resolver runs inside dispatch, on the worker's goroutine.
	panicOnce := true
	RegisterOwnerResolver("tickets:ticket-created", func(core.App, *core.Record) []string {
		if panicOnce {
			panicOnce = false
			panic("resolver exploded")
		}
		return []string{u.Id}
	})

	trigger, _, _ := eng.defs.Trigger("tickets:ticket-created")
	rule := makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "after-panic"}}}, 0, false)

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	// The Save above enqueued the panicking dispatch. If the worker died, this
	// second event is never served and waitForRuns times out.
	eng.enqueue(event{TriggerRef: "tickets:ticket-created", Trigger: trigger, Record: rec})
	waitForRuns(t, app, rule.Id, 1)
}

// TestManualRunOfDisabledRuleStillExecutes proves the manual-run endpoint's
// promise: it replies {"queued": true} regardless of the rule's enabled
// state, so dispatch must honor that for a RuleID-targeted event even though
// the rule is disabled — the auto-disable scenario being the motivating case.
// Before the fix, dispatch loaded only `enabled = true` rules and then
// filtered by RuleID, so a disabled rule silently vanished and no run row
// was ever written.
func TestManualRunOfDisabledRuleStillExecutes(t *testing.T) {
	app, eng, u := engineApp(t)
	rule := makeRule(t, app, u.Id, "personal", nil,
		[]any{map[string]any{"ref": "tickets:set-status", "params": map[string]any{"status": "ran-anyway"}}}, 0, false)
	rule.Set("enabled", false) // auto-disable-style: the rule a manual run is meant to bypass
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}

	col, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(col)
	rec.Set("title", "t")
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	trigger, _, _ := eng.defs.Trigger("tickets:ticket-created")
	eng.DispatchForTest(event{TriggerRef: "tickets:ticket-created", Trigger: trigger, Record: rec, RuleID: rule.Id})

	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": rule.Id})
	if len(runs) != 1 {
		t.Fatalf("manual-run of a disabled rule must still write a run row: got %d", len(runs))
	}
	if !runs[0].GetBool("matched") {
		t.Fatal("disabled rule targeted by RuleID must still evaluate and match")
	}
	fresh, _ := app.FindRecordById("tickets", rec.Id)
	if fresh.GetString("status") != "ran-anyway" {
		t.Fatalf("disabled rule's action must still execute: %q", fresh.GetString("status"))
	}
}
