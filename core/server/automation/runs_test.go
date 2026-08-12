// tinycld/core/server/automation/runs_test.go
package automation

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/notify"
	"tinycld.org/core/rlstest"
)

func runsApp(t *testing.T) (core.App, *core.Record) {
	t.Helper()
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

	u, err := app.FindFirstRecordByFilter("users", "id != ''")
	if err != nil {
		t.Fatal(err)
	}
	rulesCol, err := app.FindCollectionByNameOrId("rules")
	if err != nil {
		t.Fatal(err)
	}
	rule := core.NewRecord(rulesCol)
	rule.Set("name", "Test rule")
	rule.Set("scope", "personal")
	rule.Set("owner", u.Id)
	rule.Set("trigger", "core:manual")
	rule.Set("enabled", true)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	return app, rule
}

func TestWriteRunAndPrune(t *testing.T) {
	app, rule := runsApp(t)
	outcome := RunOutcome{
		Matched:  true,
		Results:  []ActionResult{{Ref: "core:notify", Status: "ok"}},
		Duration: 12 * time.Millisecond,
	}
	WriteRun(app, rule, nil, TriggerDef{}, outcome)

	runs, err := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "-fired_at", 0, 0, map[string]any{"id": rule.Id})
	if err != nil || len(runs) != 1 {
		t.Fatalf("expected 1 run: %v %v", len(runs), err)
	}
	if !runs[0].GetBool("matched") || runs[0].GetInt("duration_ms") != 12 {
		t.Fatalf("run fields: %+v", runs[0].PublicExport())
	}
}

func TestPruneKeeps200(t *testing.T) {
	app, rule := runsApp(t)
	col, _ := app.FindCollectionByNameOrId("rule_runs")
	for i := 0; i < 205; i++ {
		r := core.NewRecord(col)
		r.Set("rule", rule.Id)
		r.Set("fired_at", time.Now().Add(-time.Duration(i)*time.Minute).UTC().Format("2006-01-02 15:04:05.000Z"))
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	WriteRun(app, rule, nil, TriggerDef{}, RunOutcome{Matched: false})
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": rule.Id})
	if len(runs) != keepRunsPerRule {
		t.Fatalf("prune: got %d want %d", len(runs), keepRunsPerRule)
	}
}

func TestAutoDisableAfterConsecutiveFailures(t *testing.T) {
	app, rule := runsApp(t)

	notified := make(chan notify.NotifyParams, 1)
	orig := notifyUser
	notifyUser = func(app core.App, p notify.NotifyParams) {
		select {
		case notified <- p:
		default:
		}
	}
	t.Cleanup(func() { notifyUser = orig })

	for i := 0; i < autoDisableAfter-1; i++ {
		recordRunResult(app, rule, true)
	}
	fresh, _ := app.FindRecordById("rules", rule.Id)
	if !fresh.GetBool("enabled") {
		t.Fatal("must not disable before the threshold")
	}
	recordRunResult(app, rule, false) // success resets the streak
	for i := 0; i < autoDisableAfter; i++ {
		recordRunResult(app, rule, true)
	}
	fresh, _ = app.FindRecordById("rules", rule.Id)
	if fresh.GetBool("enabled") {
		t.Fatal(fmt.Sprintf("must disable after %d consecutive failures", autoDisableAfter))
	}

	select {
	case p := <-notified:
		if p.Type != "automation_disabled" {
			t.Fatalf("notification type: %q", p.Type)
		}
		if p.UserID != rule.GetString("owner") {
			t.Fatalf("notification UserID: got %q want %q", p.UserID, rule.GetString("owner"))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auto-disable notification was not sent")
	}
}
