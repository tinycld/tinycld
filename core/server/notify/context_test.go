package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// stuckExpo seeds owner with one Expo subscription and points the Expo API at
// a server that takes the request and never answers. It lets go when the client
// gives up, or when the test ends.
func stuckExpo(t *testing.T) (*tests.TestApp, *core.Record, <-chan struct{}) {
	t.Helper()
	app := setupAdminApp(t)
	owner := seedUser(t, app, "owner@example.com", "owner", false)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	subs := core.NewBaseCollection("push_subscriptions")
	subs.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "platform"},
		&core.TextField{Name: "expo_token"},
		&core.TextField{Name: "endpoint"},
		&core.JSONField{Name: "keys"},
	)
	if err := app.Save(subs); err != nil {
		t.Fatal(err)
	}
	sub := core.NewRecord(subs)
	sub.Set("user", owner.Id)
	sub.Set("platform", "expo")
	sub.Set("expo_token", "ExponentPushToken[test]")
	if err := app.Save(sub); err != nil {
		t.Fatal(err)
	}

	hit := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case hit <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	prev := expoPushURL
	expoPushURL = srv.URL
	t.Cleanup(func() { expoPushURL = prev })
	return app, owner, hit
}

// A caller on a deadline — a backup run the server waits for on its way down —
// must be able to end a push it started. The push services are peers this
// process does not control, and the default client has no timeout at all.
func TestDeliverToUserContextEndsAPushThatNeverAnswers(t *testing.T) {
	app, owner, hit := stuckExpo(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- DeliverToUserContext(ctx, app, NotifyParams{UserID: owner.Id, Type: "system.notice", Title: "T", Body: "B"})
	}()
	select {
	case <-hit:
	case <-time.After(10 * time.Second):
		t.Fatal("the push was never sent")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the notification row is what counts, and it was written: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("cancelling the context did not end the push")
	}
	if n := notificationCount(t, app); n != 1 {
		t.Fatalf("notifications = %d, want 1", n)
	}
}

// With no deadline from the caller, a hung push service must still give up: a
// backup announces itself before it releases the job interlock, so an unbounded
// push would block every later backup, restore and package install.
func TestDeliverToUserGivesUpOnAPushThatNeverAnswers(t *testing.T) {
	app, owner, _ := stuckExpo(t)
	prev := expoSendTimeout
	expoSendTimeout = 50 * time.Millisecond
	t.Cleanup(func() { expoSendTimeout = prev })

	done := make(chan error, 1)
	go func() {
		done <- DeliverToUser(app, NotifyParams{UserID: owner.Id, Type: "system.notice", Title: "T", Body: "B"})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a push to a service that never answers was never given up on")
	}
}
