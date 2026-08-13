// tinycld/core/server/automation/runs.go
package automation

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/notify"
)

const (
	keepRunsPerRule  = 200
	autoDisableAfter = 20
)

// notifyUser is an indirection so tests can intercept the auto-disable
// notification without racing the real push I/O against app teardown.
var notifyUser = notify.NotifyUser

// appIsLive reports whether app still has a usable DB connection pool. The
// auto-disable notification fires from a background goroutine that can
// outlive the request/test that triggered it; checking this before touching
// app avoids a nil-pointer panic against a torn-down app. engine.go's worker
// (Task 7) reuses this same helper rather than redefining it.
func appIsLive(app core.App) bool {
	return app != nil && app.ConcurrentDB() != nil
}

type ActionResult struct {
	Ref     string `json:"ref"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type RunOutcome struct {
	Matched  bool
	Results  []ActionResult
	Err      string
	Duration time.Duration
}

func triggerSummary(record *core.Record, trigger TriggerDef) map[string]any {
	if record == nil {
		return nil
	}
	out := map[string]any{}
	for key := range exposedFields(record, trigger) {
		out[key] = normalize(record.Get(key))
	}
	return out
}

// WriteRun logs one engine run — matched or not: "why didn't it fire" is
// debugged from non-match rows. Failures to log are reported to the app
// logger, never propagated: the run already happened.
func WriteRun(app core.App, rule *core.Record, record *core.Record, trigger TriggerDef, outcome RunOutcome) {
	col, err := app.FindCollectionByNameOrId("rule_runs")
	if err != nil {
		app.Logger().Error("automation: rule_runs collection missing", "err", err)
		return
	}
	run := core.NewRecord(col)
	run.Set("rule", rule.Id)
	run.Set("fired_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	run.Set("matched", outcome.Matched)
	run.Set("trigger_summary", triggerSummary(record, trigger))
	if len(outcome.Results) > 0 {
		b, _ := json.Marshal(outcome.Results)
		run.Set("results", json.RawMessage(b))
	}
	run.Set("error", outcome.Err)
	run.Set("duration_ms", outcome.Duration.Milliseconds())
	if err := app.Save(run); err != nil {
		app.Logger().Error("automation: write rule_run", "rule", rule.Id, "err", err)
		return
	}
	pruneRuns(app, rule.Id)
}

func pruneRuns(app core.App, ruleID string) {
	for {
		extra, err := app.FindRecordsByFilter(
			"rule_runs", "rule = {:id}", "-fired_at", 50, keepRunsPerRule,
			map[string]any{"id": ruleID},
		)
		if err != nil || len(extra) == 0 {
			return
		}
		for _, r := range extra {
			if err := app.Delete(r); err != nil {
				app.Logger().Error("automation: prune rule_run", "err", err)
				return
			}
		}
		if len(extra) < 50 {
			return
		}
	}
}

// failureStreaks is in-memory by design: a restart resets the streak, and
// rule_runs is the durable record. Good enough to stop a hot broken rule.
var failureStreaks sync.Map

func ResetRunStateForTest() {
	failureStreaks = sync.Map{}
}

func recordRunResult(app core.App, rule *core.Record, fullyFailed bool) {
	if !fullyFailed {
		failureStreaks.Delete(rule.Id)
		return
	}
	n := 1
	if v, ok := failureStreaks.Load(rule.Id); ok {
		n = v.(int) + 1
	}
	failureStreaks.Store(rule.Id, n)
	if n < autoDisableAfter {
		return
	}
	failureStreaks.Delete(rule.Id)
	rule.Set("enabled", false)
	markEngineWrite(rule.Id, rule.Id, 0)
	if err := app.Save(rule); err != nil {
		app.Logger().Error("automation: auto-disable", "rule", rule.Id, "err", err)
		return
	}
	go func() {
		if !appIsLive(app) {
			return
		}
		notifyUser(app, notify.NotifyParams{
			UserID:  rule.GetString("owner"),
			Type:    "automation_disabled",
			Package: "core",
			Title:   "Automation rule disabled",
			Body:    "\"" + rule.GetString("name") + "\" failed repeatedly and was turned off.",
			URL:     "/",
		})
	}()
}
