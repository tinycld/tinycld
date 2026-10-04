package coreserver

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/readonly"
	"tinycld.org/core/readonly/readonlytest"
)

// inviteNotifyApp is the invite fixture plus the notifications collection the
// invite's tail writes to, and one invited user.
func inviteNotifyApp(t *testing.T) (core.App, *core.Record) {
	t.Helper()
	app := setupInviteTestApp(t)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	notifs := core.NewBaseCollection("notifications")
	notifs.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "type"}, &core.TextField{Name: "package"}, &core.TextField{Name: "title"},
		&core.TextField{Name: "body"}, &core.TextField{Name: "url"}, &core.JSONField{Name: "metadata", MaxSize: 20000},
		&core.BoolField{Name: "read"}, &core.BoolField{Name: "dismissed"},
	)
	if err := app.Save(notifs); err != nil {
		t.Fatal(err)
	}
	return app, mustCreateUser(t, app, "invited@test.local", false)
}

func inviteNotifications(t *testing.T, app core.App, userID string) int {
	t.Helper()
	rows, err := app.FindRecordsByFilter("notifications", "user = {:id} && type = 'org_invite'", "", 0, 0, map[string]any{"id": userID})
	if err != nil {
		t.Fatal(err)
	}
	return len(rows)
}

func TestInviteNotificationWaitsForReadOnlyToEnd(t *testing.T) {
	app, user := inviteNotifyApp(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	probe, waiting := readonlytest.WaitProbe(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		notifyInvitedWhenWritable(probe, app, user.Id, "member")
	}()
	select {
	case <-waiting:
	case <-done:
		t.Fatal("the invite notification was written without waiting for read-only mode to end")
	case <-time.After(5 * time.Second):
		t.Fatal("the invite notification never reached the read-only wait")
	}
	if n := inviteNotifications(t, app, user.Id); n != 0 {
		t.Fatalf("an invite notification was written while read-only: %d rows", n)
	}

	readonly.Leave()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the invite notification was not written after read-only mode ended")
	}
	if n := inviteNotifications(t, app, user.Id); n != 1 {
		t.Fatalf("want 1 invite notification after read-only mode ended, got %d", n)
	}
}

func TestInviteNotificationDroppedAndLoggedWhenReadOnlyOutlastsTheWait(t *testing.T) {
	app, user := inviteNotifyApp(t)
	logs := readonlytest.CaptureLogs(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	notifyInvitedWhenWritable(ctx, app, user.Id, "member")

	if n := inviteNotifications(t, app, user.Id); n != 0 {
		t.Fatalf("an invite notification was written while read-only: %d rows", n)
	}
	r, ok := readonlytest.Find(logs(), inviteNotifyDroppedMsg)
	if !ok {
		t.Fatalf("no %q log for the dropped notification", inviteNotifyDroppedMsg)
	}
	if r.Level != slog.LevelWarn {
		t.Fatalf("want Warn for a dropped invite notification, got %v", r.Level)
	}
	if r.Attrs["userID"] != user.Id {
		t.Fatalf("the log must name the invited user, got %v", r.Attrs)
	}
}
