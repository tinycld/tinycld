package automation

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestActionRegistry(t *testing.T) {
	t.Cleanup(ResetRegistriesForTest)
	called := false
	RegisterAction("mail:send-message", func(app core.App, req ActionRequest) error {
		called = true
		return nil
	})
	h, ok := actionHandler("mail:send-message")
	if !ok {
		t.Fatal("registered handler must resolve")
	}
	if err := h(nil, ActionRequest{}); err != nil || !called {
		t.Fatal("handler must be invocable")
	}
	if _, ok := actionHandler("mail:unregistered"); ok {
		t.Fatal("unknown ref must miss")
	}
}

func TestOwnerAutoDetect(t *testing.T) {
	t.Cleanup(ResetRegistriesForTest)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	col := core.NewBaseCollection("owned_things")
	col.Fields.Add(&core.TextField{Name: "title"})
	col.Fields.Add(&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}

	u, err := app.FindFirstRecordByFilter("users", "id != ''")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("title", "x")
	r.Set("owner", u.Id)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}

	owners := ResolveOwners(app, "pkg:thing-created", TriggerDef{Collection: "owned_things", On: "create"}, r)
	if len(owners) != 1 || owners[0] != u.Id {
		t.Fatalf("auto-detect via 'owner' relation: %v", owners)
	}

	// No user-relation field → no personal scope.
	bare := core.NewBaseCollection("bare_things")
	bare.Fields.Add(&core.TextField{Name: "title"})
	if err := app.Save(bare); err != nil {
		t.Fatal(err)
	}
	br := core.NewRecord(bare)
	br.Set("title", "y")
	if err := app.Save(br); err != nil {
		t.Fatal(err)
	}
	if owners := ResolveOwners(app, "pkg:bare", TriggerDef{Collection: "bare_things"}, br); len(owners) != 0 {
		t.Fatalf("unresolvable owner must be empty, got %v", owners)
	}

	// A registered resolver wins over auto-detection.
	RegisterOwnerResolver("pkg:bare", func(app core.App, record *core.Record) []string {
		return []string{"custom-user-id"}
	})
	if owners := ResolveOwners(app, "pkg:bare", TriggerDef{Collection: "bare_things"}, br); len(owners) != 1 || owners[0] != "custom-user-id" {
		t.Fatalf("resolver must win: %v", owners)
	}
}
