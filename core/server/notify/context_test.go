package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// A caller on a deadline — a backup run the server waits for on its way down —
// must be able to end a push it started. The push services are peers this
// process does not control, and the default client has no timeout at all.
func TestDeliverToUserContextEndsAPushThatNeverAnswers(t *testing.T) {
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
