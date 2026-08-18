// tinycld/core/server/automation/endpoints_test.go
package automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/notify"
)

// Endpoint auth/validation logic is factored into plain funcs so it tests
// without HTTP scaffolding; the route bindings are thin.
func TestRunEndpointValidation(t *testing.T) {
	app, _, u := scheduleApp(t)
	manual := scheduleRule(t, app, u.Id, "0 8 * * *", true) // trigger core:schedule → synthetic, runnable
	col, _ := app.FindCollectionByNameOrId("rules")
	recordRule := core.NewRecord(col)
	recordRule.Set("name", "rec")
	recordRule.Set("scope", "personal")
	recordRule.Set("owner", u.Id)
	recordRule.Set("trigger", "tickets:ticket-created")
	recordRule.Set("enabled", true)
	if err := app.Save(recordRule); err != nil {
		t.Fatal(err)
	}

	if err := validateManualRun(manual, u); err != nil {
		t.Fatalf("owner running a synthetic-trigger rule must pass: %v", err)
	}
	if err := validateManualRun(recordRule, u); err == nil {
		t.Fatal("record-trigger rules must be rejected")
	}

	users, _ := app.FindCollectionByNameOrId("users")
	stranger := core.NewRecord(users)
	stranger.Set("email", "s@example.com")
	stranger.Set("username", "stranger1")
	stranger.Set("name", "S")
	stranger.Set("role", "member")
	stranger.SetPassword("0123456789")
	if err := app.Save(stranger); err != nil {
		t.Fatal(err)
	}
	if err := validateManualRun(manual, stranger); err == nil {
		t.Fatal("non-owner non-admin must be rejected")
	}
	stranger.Set("role", "admin")
	if err := validateManualRun(manual, stranger); err != nil {
		t.Fatalf("admin may run any rule: %v", err)
	}
}

func TestDryRunScoping(t *testing.T) {
	app, eng, u := engineApp(t) // tickets collection + trigger defs from Task 7's helper
	col, _ := app.FindCollectionByNameOrId("tickets")
	for _, title := range []string{"urgent: a", "routine b", "URGENT c"} {
		r := core.NewRecord(col)
		r.Set("title", title)
		r.Set("user", u.Id)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	ast := ConditionsAST{Match: "all", Groups: []ConditionGroup{{
		Match:      "any",
		Conditions: []Condition{{Field: "title", Op: "contains", Value: "urgent"}},
	}}}
	res, err := eng.dryRun(u, "tickets:ticket-created", ast)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 3 || len(res.Matches) != 2 {
		t.Fatalf("dry run: total=%d matches=%d", res.Total, len(res.Matches))
	}
}

// TestDryRunZeroMatchesMarshalsToEmptyArray pins the wire shape, not just the
// Go value: the client maps over `matches`, so a nil slice serializing to
// `null` crashes the dry-run panel on the ordinary "nothing matches yet" case.
func TestDryRunZeroMatchesMarshalsToEmptyArray(t *testing.T) {
	app, eng, u := engineApp(t)
	col, _ := app.FindCollectionByNameOrId("tickets")
	r := core.NewRecord(col)
	r.Set("title", "routine")
	r.Set("user", u.Id)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	ast := ConditionsAST{Match: "all", Groups: []ConditionGroup{{
		Match:      "any",
		Conditions: []Condition{{Field: "title", Op: "contains", Value: "matches-nothing"}},
	}}}

	res, err := eng.dryRun(u, "tickets:ticket-created", ast)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Matches) != 0 {
		t.Fatalf("expected no matches, got %d", len(res.Matches))
	}
	b, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"matches":[]`) {
		t.Fatalf("zero matches must marshal to [], got %s", b)
	}

	// An error return must not emit `null` either — the client parses the body
	// on the failure path too.
	bad, err := eng.dryRun(u, "tickets:no-such-trigger", ast)
	if err == nil {
		t.Fatal("unknown trigger must error")
	}
	if b, _ := json.Marshal(bad); !strings.Contains(string(b), `"matches":[]`) {
		t.Fatalf("error result must still marshal matches as [], got %s", b)
	}
}

// TestDryRunUnscopedRequiresAdmin covers the "collection has no resolvable
// owner field" branch: ownerFilterFor returns ok=false for "broadcasts" (no
// user/owner/author relation), so dry-run can't scope results to the caller.
// A non-admin caller must be rejected outright; an admin caller gets results
// across every record, unscoped.
func TestDryRunUnscopedRequiresAdmin(t *testing.T) {
	app, eng, u := engineApp(t)
	col, _ := app.FindCollectionByNameOrId("broadcasts")
	for _, title := range []string{"alert one", "routine note", "ALERT two"} {
		r := core.NewRecord(col)
		r.Set("title", title)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	ast := ConditionsAST{Match: "all", Groups: []ConditionGroup{{
		Match:      "any",
		Conditions: []Condition{{Field: "title", Op: "contains", Value: "alert"}},
	}}}

	if _, err := eng.dryRun(u, "tickets:broadcast-created", ast); err == nil {
		t.Fatal("non-admin caller against an unscoped collection must be rejected")
	}

	u.Set("role", "admin")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	res, err := eng.dryRun(u, "tickets:broadcast-created", ast)
	if err != nil {
		t.Fatalf("admin caller must be allowed against an unscoped collection: %v", err)
	}
	if res.Total != 3 || len(res.Matches) != 2 {
		t.Fatalf("admin dry run: total=%d matches=%d", res.Total, len(res.Matches))
	}
}

// TestCoreNotifyHandler exercises the real core:notify handler as Register
// installs it — not a test-local stub (actions_test.go's
// TestNativeDispatchAndMissingHandler registers its own) — and asserts it
// goes through the deliverNotification seam rather than calling
// notify.DeliverToUser directly, so a test can observe it without racing real
// push I/O.
func TestCoreNotifyHandler(t *testing.T) {
	t.Cleanup(ResetRegistriesForTest)
	registerCoreNativeActions()

	captured := make(chan notify.NotifyParams, 1)
	original := deliverNotification
	deliverNotification = func(app core.App, params notify.NotifyParams) error {
		captured <- params
		return nil
	}
	t.Cleanup(func() { deliverNotification = original })

	app, rule := runsApp(t)
	defs := &Defs{Packages: []PackageDefs{{
		Slug: "core",
		Actions: []ActionDef{{
			ID: "notify", Kind: "native",
			Params: []ParamDef{{Key: "title"}, {Key: "body"}, {Key: "url"}},
		}},
	}}}
	rawParams := map[string]any{"title": "Hello {{title}}", "body": "a body", "url": "/somewhere"}
	scratchCol := core.NewBaseCollection("scratch")
	scratchCol.Fields.Add(&core.TextField{Name: "title"})
	rec := core.NewRecord(scratchCol)
	rec.Set("title", "World")

	if err := ExecuteAction(app, defs, "core:notify", rawParams, rule, TriggerDef{}, rec, 0); err != nil {
		t.Fatal(err)
	}

	// Synchronous: the handler has returned, so the notification is already
	// delivered. It used to run on a detached goroutine, which is why the
	// action recorded "ok" before delivery had even been attempted.
	select {
	case params := <-captured:
		if params.Type != "automation" || params.Package != "core" {
			t.Fatalf("notification shape: %+v", params)
		}
		if params.Title != "Hello World" {
			t.Fatalf("substituted title: %q", params.Title)
		}
		if params.UserID != rule.GetString("owner") {
			t.Fatalf("UserID must be the rule owner: got %q want %q", params.UserID, rule.GetString("owner"))
		}
	default:
		t.Fatal("core:notify must deliver before returning, so its run result reflects the outcome")
	}
}

// A failed delivery must surface as a failed action, not a cheerful "ok".
// Before the handler ran synchronously, a notification that never landed left
// run history looking healthy — and auto-disable could never see it failing.
func TestCoreNotifyReportsDeliveryFailure(t *testing.T) {
	t.Cleanup(ResetRegistriesForTest)
	registerCoreNativeActions()

	original := deliverNotification
	deliverNotification = func(core.App, notify.NotifyParams) error {
		return fmt.Errorf("notifications collection unavailable")
	}
	t.Cleanup(func() { deliverNotification = original })

	app, rule := runsApp(t)
	defs := &Defs{Packages: []PackageDefs{{
		Slug: "core",
		Actions: []ActionDef{{
			ID: "notify", Kind: "native",
			Params: []ParamDef{{Key: "title"}},
		}},
	}}}

	err := ExecuteAction(app, defs, "core:notify",
		map[string]any{"title": "x"}, rule, TriggerDef{}, nil, 0)
	if err == nil {
		t.Fatal("a failed notification must fail the action")
	}
	if !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("the reason must reach run history: %v", err)
	}
}
