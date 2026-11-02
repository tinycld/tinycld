// tinycld/core/server/automation/runs_test.go
package automation

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/notify"
	"tinycld.org/core/rlstest"
)

func runsApp(t *testing.T) (core.App, *core.Record) {
	t.Helper()
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))

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
	notifyUser = func(_ context.Context, _ core.App, p notify.NotifyParams) {
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

func TestWriteRunRecordsTheTriggerRecord(t *testing.T) {
	app, _, u := engineApp(t)
	tickets, _ := app.FindCollectionByNameOrId("tickets")
	rec := core.NewRecord(tickets)
	rec.Set("title", "hello")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	rule := makeRule(t, app, u.Id, "org", nil, nil, 0, false)
	trigger := TriggerDef{ID: "ticket-created", Collection: "tickets", On: "create"}

	WriteRun(app, rule, rec, trigger, RunOutcome{Matched: true, Fields: map[string]any{"fields.email": "a@b.c"}})

	run, err := app.FindFirstRecordByFilter("rule_runs", "rule = {:r}", map[string]any{"r": rule.Id})
	if err != nil {
		t.Fatal(err)
	}
	if run.GetString("record_id") != rec.Id || run.GetString("collection") != "tickets" {
		t.Fatalf("record not stored: %v", run.FieldsData())
	}
	var summary map[string]any
	_ = json.Unmarshal([]byte(run.GetString("trigger_summary")), &summary)
	if summary["fields.email"] != "a@b.c" || summary["title"] != "hello" {
		t.Fatalf("summary must hold columns and dynamic fields: %v", summary)
	}
}
