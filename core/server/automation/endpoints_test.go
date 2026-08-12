// tinycld/core/server/automation/endpoints_test.go
package automation

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
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
