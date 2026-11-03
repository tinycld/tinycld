package automation

import (
	"context"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestRelationMatchRefusesAnotherKindOfRecord(t *testing.T) {
	app, _, u := engineApp(t)
	tickets, _ := app.FindCollectionByNameOrId("tickets")
	tickets.ViewRule = types.Pointer("user = @request.auth.id")
	if err := app.Save(tickets); err != nil {
		t.Fatal(err)
	}
	open := core.NewRecord(tickets)
	open.Set("title", "a")
	open.Set("status", "open")
	open.Set("user", u.Id)
	closed := core.NewRecord(tickets)
	closed.Set("title", "b")
	closed.Set("status", "closed")
	closed.Set("user", u.Id)
	for _, r := range []*core.Record{open, closed} {
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	defs := &Defs{Packages: []PackageDefs{{Slug: "gizmos", Actions: []ActionDef{{ID: "use", Kind: "native", Params: []ParamDef{
		{Key: "ticket", Type: "relation", RelationTarget: "tickets", RelationMatch: map[string]any{"status": "open"}},
	}}}}}}
	ran := 0
	RegisterAction("gizmos:use", func(core.App, ActionRequest) error { ran++; return nil })
	RegisterRelationAuthorizer("gizmos:use", "ticket", func(core.App, ActionRequest, string) error { return nil })
	rule := makeRule(t, app, u.Id, "personal", nil, nil, 0, false)
	trigger := TriggerDef{ID: "x", Collection: "tickets", On: "create"}

	if err := ExecuteActionContext(context.Background(), app, defs, "gizmos:use", map[string]any{"ticket": open.Id}, rule, trigger, open, 0, nil); err != nil {
		t.Fatalf("a matching record runs: %v", err)
	}
	err := ExecuteActionContext(context.Background(), app, defs, "gizmos:use", map[string]any{"ticket": closed.Id}, rule, trigger, open, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "not one this action can use") {
		t.Fatalf("a record that does not match is refused, got %v", err)
	}
	if ran != 1 {
		t.Fatalf("the handler ran %d times, want 1", ran)
	}
}
