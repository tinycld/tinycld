package groups

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// A hook that wraps e.Next() in its own transaction makes PocketBase fire the
// row's after-success hooks, realtime among them, before that transaction
// commits. The realtime access check then cannot see the row, and a
// subscriber never hears of it. A direct row has nothing to expand, so its
// write must stay outside any hook-opened transaction.
func TestDirectRowWritesReachRealtimeSubscribers(t *testing.T) {
	app := newZooApp(t)
	if _, err := apis.NewRouter(app); err != nil {
		t.Fatalf("bind realtime: %v", err)
	}
	keepers, err := app.FindCollectionByNameOrId("zoo_keepers")
	if err != nil {
		t.Fatal(err)
	}
	// A rule the access check must resolve against the database, as every
	// real package rule does.
	rule := `@request.auth.id != "" && user = @request.auth.id`
	keepers.ListRule = &rule
	keepers.ViewRule = &rule
	if err := app.Save(keepers); err != nil {
		t.Fatal(err)
	}
	bob := zooUser(t, app, "bob@x.test")

	client := subscriptions.NewDefaultClient()
	client.Set(apis.RealtimeClientAuthKey, bob)
	client.Subscribe("zoo_keepers")
	app.SubscriptionsBroker().Register(client)
	t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })

	expectEvent := func(action string) {
		t.Helper()
		select {
		case <-client.Channel():
		case <-time.After(2 * time.Second):
			t.Fatalf("no realtime %s event for bob's direct row", action)
		}
	}

	row := core.NewRecord(keepers)
	row.Set("zoo", "bronx")
	row.Set("user", bob.Id)
	row.Set("role", "viewer")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	expectEvent("create")

	row.Set("role", "editor")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	expectEvent("update")
}
