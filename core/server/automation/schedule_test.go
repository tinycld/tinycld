// tinycld/core/server/automation/schedule_test.go
package automation

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/rlstest"
)

func scheduleApp(t *testing.T) (core.App, *Engine, *core.Record) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	t.Cleanup(ResetRunStateForTest)
	app := rlstest.NewApp(t)

	// Same fixture reconciliation as engineApp/pkgaccessApp: drop the bundled
	// username index so 1820000000 (users_username_required) applies the way
	// it does against a real DB.
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
	defs := &Defs{Packages: []PackageDefs{{
		Slug:     "core",
		Triggers: []TriggerDef{{ID: "schedule", Synthetic: "schedule"}, {ID: "manual", Synthetic: "manual"}},
	}}}
	eng := NewEngine(app, defs)
	eng.Start()
	u, _ := app.FindFirstRecordByFilter("users", "id != ''")
	return app, eng, u
}

func scheduleRule(t *testing.T, app core.App, owner, cron string, enabled bool) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("rules")
	r := core.NewRecord(col)
	r.Set("name", "sched")
	r.Set("scope", "personal")
	r.Set("owner", owner)
	r.Set("trigger", "core:schedule")
	r.Set("trigger_config", map[string]any{"cron": cron})
	r.Set("enabled", enabled)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func hasJob(app core.App, ruleID string) bool {
	for _, j := range app.Cron().Jobs() {
		if j.Id() == "automation:"+ruleID {
			return true
		}
	}
	return false
}

func TestScheduleReconcile(t *testing.T) {
	app, _, u := scheduleApp(t)
	rule := scheduleRule(t, app, u.Id, "0 8 * * *", true)
	if !hasJob(app, rule.Id) {
		t.Fatal("enabled schedule rule must register a cron job")
	}

	rule.Set("enabled", false)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	if hasJob(app, rule.Id) {
		t.Fatal("disabling must remove the job")
	}

	rule.Set("enabled", true)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	if !hasJob(app, rule.Id) {
		t.Fatal("re-enabling must re-add the job")
	}

	if err := app.Delete(rule); err != nil {
		t.Fatal(err)
	}
	if hasJob(app, rule.Id) {
		t.Fatal("deleting must remove the job")
	}
}

func TestSyncSchedulesOnBoot(t *testing.T) {
	app, _, u := scheduleApp(t)
	// Rule created while "engine offline": simulate by removing the job, then sync.
	rule := scheduleRule(t, app, u.Id, "*/5 * * * *", true)
	app.Cron().Remove("automation:" + rule.Id)
	eng2 := NewEngine(app, &Defs{})
	eng2.syncSchedules()
	if !hasJob(app, rule.Id) {
		t.Fatal("syncSchedules must pick up existing enabled rules")
	}
}

func TestInvalidCronIsSurfaced(t *testing.T) {
	app, _, u := scheduleApp(t)
	rule := scheduleRule(t, app, u.Id, "not a cron", true)
	if hasJob(app, rule.Id) {
		t.Fatal("invalid cron must not register")
	}
	runs, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": rule.Id})
	if len(runs) != 1 || runs[0].GetString("error") == "" {
		t.Fatalf("invalid cron must write an explanatory run row: %d", len(runs))
	}
}

func TestScheduledDispatchTargetsOneRule(t *testing.T) {
	app, eng, u := scheduleApp(t)
	a := scheduleRule(t, app, u.Id, "0 8 * * *", true)
	b := scheduleRule(t, app, u.Id, "0 9 * * *", true)

	trigger, _, _ := eng.defs.Trigger("core:schedule")
	eng.DispatchForTest(event{TriggerRef: "core:schedule", Trigger: trigger, RuleID: a.Id})

	runsA, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": a.Id})
	runsB, _ := app.FindRecordsByFilter("rule_runs", "rule = {:id}", "", 0, 0, map[string]any{"id": b.Id})
	if len(runsA) != 1 || len(runsB) != 0 {
		t.Fatalf("scheduled dispatch must hit exactly its rule: a=%d b=%d", len(runsA), len(runsB))
	}
	if !runsA[0].GetBool("matched") {
		t.Fatal("nil-record dispatch with empty conditions must match (personal owner filter skipped)")
	}
}
