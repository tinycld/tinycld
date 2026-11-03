package automation

import (
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

var errBoom = errors.New("boom")

func TestRunObserversSeeEachLoggedRun(t *testing.T) {
	app, eng, u, save := dynamicApp(t)
	RegisterAction("posts:capture", func(core.App, ActionRequest) error { return errBoom })
	var seen []RunOutcome
	RegisterRunObserver("posts:post-created", func(_ core.App, _ *core.Record, _ *core.Record, o RunOutcome) {
		seen = append(seen, o)
	})
	trigger, _, _ := eng.defs.Trigger("posts:post-created")
	dynamicRule(t, app, u.Id, "a", `{"match":"all","groups":[]}`, "x")
	eng.DispatchForTest(event{TriggerRef: "posts:post-created", Trigger: trigger, Record: save("a", "y")})
	if len(seen) != 1 || !seen[0].Failed() {
		t.Fatalf("one failed run must be observed, got %#v", seen)
	}
}
