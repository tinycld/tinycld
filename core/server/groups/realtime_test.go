package groups

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// realtimeZooApp is newZooApp with the realtime broadcast hooks bound and a
// DB-resolved rule on every collection a test subscribes to, as every real
// package rule is. A rule that needs the database is what drops an event
// when the access check runs against the wrong app.
func realtimeZooApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newZooApp(t)
	if _, err := apis.NewRouter(app); err != nil {
		t.Fatalf("bind realtime: %v", err)
	}
	rules := map[string]string{
		"zoo_keepers":   `@request.auth.id != "" && (user = @request.auth.id || group.id != "")`,
		"group_members": `@request.auth.id != "" && user = @request.auth.id`,
		"users":         `id = @request.auth.id`,
	}
	for name, rule := range rules {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatal(err)
		}
		col.ListRule = &rule
		col.ViewRule = &rule
		if err := app.Save(col); err != nil {
			t.Fatal(err)
		}
	}
	return app
}

type rtSubscriber struct {
	t      *testing.T
	app    core.App
	client *subscriptions.DefaultClient
}

func subscribe(t *testing.T, app core.App, auth *core.Record, topic string) *rtSubscriber {
	t.Helper()
	client := subscriptions.NewDefaultClient()
	client.Set(apis.RealtimeClientAuthKey, auth)
	client.Subscribe(topic)
	app.SubscriptionsBroker().Register(client)
	t.Cleanup(func() { app.SubscriptionsBroker().Unregister(client.Id()) })
	return &rtSubscriber{t: t, app: app, client: client}
}

// expect waits for the realtime event for one row, then reads that row back
// through the app, as a client does when it refetches on the event.
func (s *rtSubscriber) expect(action, collection, id string) {
	s.t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case msg := <-s.client.Channel():
			var body struct {
				Action string `json:"action"`
				Record struct {
					Id string `json:"id"`
				} `json:"record"`
			}
			if err := json.Unmarshal(msg.Data, &body); err != nil {
				s.t.Fatalf("decode realtime message: %v", err)
			}
			if body.Action != action || body.Record.Id != id {
				continue
			}
			_, err := s.app.FindRecordById(collection, id)
			if action == "delete" && err == nil {
				s.t.Fatalf("%s %s still readable after its delete event", collection, id)
			}
			if action != "delete" && err != nil {
				s.t.Fatalf("%s %s not readable after its %s event: %v", collection, id, action, err)
			}
			return
		case <-deadline:
			s.t.Fatalf("no realtime %s event for %s %s", action, collection, id)
		}
	}
}

// A direct row has nothing to expand and takes no hook transaction.
func TestDirectRowWritesReachRealtimeSubscribers(t *testing.T) {
	app := realtimeZooApp(t)
	bob := zooUser(t, app, "bob@x.test")
	sub := subscribe(t, app, bob, "zoo_keepers")

	keepers, _ := app.FindCollectionByNameOrId("zoo_keepers")
	row := core.NewRecord(keepers)
	row.Set("zoo", "bronx")
	row.Set("user", bob.Id)
	row.Set("role", "viewer")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	sub.expect("create", "zoo_keepers", row.Id)

	row.Set("role", "editor")
	if err := app.Save(row); err != nil {
		t.Fatal(err)
	}
	sub.expect("update", "zoo_keepers", row.Id)
}

// The writes below expand derived rows inside a transaction the hook opens.
// Their own realtime event must still reach a subscriber once that
// transaction commits.

func TestGrantWritesReachRealtimeSubscribers(t *testing.T) {
	app := realtimeZooApp(t)
	bob := zooUser(t, app, "bob@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, bob)
	sub := subscribe(t, app, bob, "zoo_keepers")

	grant := zooGrant(t, app, "bronx", g, "viewer")
	sub.expect("create", "zoo_keepers", grant.Id)
	if got := derivedRows(t, app, "bronx")[bob.Id]; got != "viewer" {
		t.Fatalf("derived row role = %q, want viewer", got)
	}

	grant.Set("role", "editor")
	if err := app.Save(grant); err != nil {
		t.Fatal(err)
	}
	sub.expect("update", "zoo_keepers", grant.Id)

	if err := app.Delete(grant); err != nil {
		t.Fatal(err)
	}
	sub.expect("delete", "zoo_keepers", grant.Id)
}

func TestMembershipWritesReachRealtimeSubscribers(t *testing.T) {
	app := realtimeZooApp(t)
	bob := zooUser(t, app, "bob@x.test")
	g := zooGroup(t, app, "keepers")
	sub := subscribe(t, app, bob, "group_members")

	m := zooMember(t, app, g, bob)
	sub.expect("create", "group_members", m.Id)

	if err := app.Delete(m); err != nil {
		t.Fatal(err)
	}
	sub.expect("delete", "group_members", m.Id)
}

func TestGuestDemotionReachesRealtimeSubscribers(t *testing.T) {
	app := realtimeZooApp(t)
	bob := zooUser(t, app, "bob@x.test")
	g := zooGroup(t, app, "keepers")
	zooMember(t, app, g, bob)
	sub := subscribe(t, app, bob, "users")

	bob.Set("role", "guest")
	if err := app.Save(bob); err != nil {
		t.Fatal(err)
	}
	sub.expect("update", "users", bob.Id)
}
