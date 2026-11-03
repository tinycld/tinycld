package automation

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestValidateRecordRun(t *testing.T) {
	app, _, u := engineApp(t)
	recordTrigger := TriggerDef{ID: "ticket-created", Collection: "tickets", On: "create"}
	org := makeRule(t, app, u.Id, "org", nil, nil, 0, false)
	u.Set("role", "member")
	if err := validateRecordRun(org, recordTrigger, u); !errors.Is(err, errRunNotAllowed) {
		t.Fatalf("a member may not retry an org rule, as a permission failure: %v", err)
	}
	u.Set("role", "admin")
	if err := validateRecordRun(org, recordTrigger, u); err != nil {
		t.Fatalf("an admin may retry an org rule: %v", err)
	}
	if err := validateRecordRun(org, TriggerDef{ID: "manual", Synthetic: "manual"}, u); err == nil {
		t.Fatal("a synthetic trigger has no record to retry")
	}
	if err := validateRecordRun(org, recordTrigger, nil); err == nil {
		t.Fatal("anonymous callers are refused")
	}
	org.Set("enabled", false)
	if err := validateRecordRun(org, recordTrigger, u); err == nil || errors.Is(err, errRunNotAllowed) {
		t.Fatal("a disabled rule is refused, and not as a permission failure")
	}
}

func TestRetryRunsTheRuleForTheRecord(t *testing.T) {
	app, eng, u, save := dynamicApp(t)
	calls := 0
	RegisterAction("posts:capture", func(core.App, ActionRequest) error { calls++; return nil })
	rule := dynamicRule(t, app, u.Id, "a", `{"match":"all","groups":[]}`, "x")
	rec := save("a", "y")
	trigger, _, _ := eng.defs.Trigger("posts:post-created")
	eng.DispatchForTest(retryEvent("posts:post-created", trigger, rule, rec))
	if calls != 1 {
		t.Fatalf("retry must run the rule once, got %d", calls)
	}
	other := save("b", "z")
	eng.DispatchForTest(retryEvent("posts:post-created", trigger, rule, other))
	if calls != 1 {
		t.Fatal("a retry for a record the rule filter refuses must not run")
	}
}
