package coreserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
	"github.com/pocketbase/pocketbase/tools/router"
)

// keepUnrestored lays out what the supervisor leaves when a rollback could
// not restore a backup: <state>/unrestored/<build>/{data.db,unrestored.json}.
func keepUnrestored(t *testing.T, build string, withNote bool) string {
	t.Helper()
	dir := filepath.Join(stateUnrestoredDir(), build)
	mustNil(t, os.MkdirAll(dir, 0o755))
	mustNil(t, os.WriteFile(filepath.Join(dir, "data.db"), []byte("pre-migration"), 0o644))
	if withNote {
		data, err := json.Marshal(map[string]any{
			"build": build, "rolled_to": "build-prev", "at": time.Now().UTC(),
			"restore_error": "no space left on device", "size": len("pre-migration"),
		})
		mustNil(t, err)
		mustNil(t, os.WriteFile(filepath.Join(dir, "unrestored.json"), data, 0o644))
	}
	return dir
}

func unrestoredTestApp(t *testing.T) core.App {
	t.Helper()
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	app := adminConsoleTestApp(t)
	newUser(t, app, "owner@x.test", "owner", false)
	return app
}

func unrestoredNotifications(t *testing.T, app core.App) []*core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter("notifications", "type = {:t}", "", 0, 0, map[string]any{"t": unrestoredNotifyType})
	mustNil(t, err)
	return rows
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// A kept backup is told to the administrators once, in the app and by email,
// and the "notified" file stops the next boot from telling them again.
func TestReportUnrestoredNotifiesOnce(t *testing.T) {
	app := unrestoredTestApp(t)
	dir := keepUnrestored(t, "build-9", true)

	var sent []notice
	mail := func(_ context.Context, _ core.App, n notice) error {
		sent = append(sent, n)
		return nil
	}
	reportUnrestored(context.Background(), app, mail)

	if len(sent) != 1 {
		t.Fatalf("sent %d emails, want 1", len(sent))
	}
	if !strings.Contains(sent[0].BodyText, "build build-9") || !strings.Contains(sent[0].BodyText, dir) {
		t.Fatalf("email body %q does not name the build and the dir", sent[0].BodyText)
	}
	rows := unrestoredNotifications(t, app)
	if len(rows) != 1 {
		t.Fatalf("%d in-app notifications, want 1", len(rows))
	}
	if !strings.Contains(rows[0].GetString("body"), dir) {
		t.Fatalf("in-app body %q does not name the dir", rows[0].GetString("body"))
	}
	if !exists(filepath.Join(dir, "notified-app")) || !exists(filepath.Join(dir, "notified-email")) {
		t.Fatal("no notified markers after a delivered notice")
	}

	reportUnrestored(context.Background(), app, mail)
	if len(sent) != 1 || len(unrestoredNotifications(t, app)) != 1 {
		t.Fatalf("second boot sent again: %d emails, %d notifications", len(sent), len(unrestoredNotifications(t, app)))
	}
}

// A failed email leaves no email marker, so the next boot sends the email
// again; the in-app notice that worked is not repeated.
func TestReportUnrestoredRetriesOnlyTheChannelThatFailed(t *testing.T) {
	app := unrestoredTestApp(t)
	dir := keepUnrestored(t, "build-9", true)

	reportUnrestored(context.Background(), app, func(context.Context, core.App, notice) error { return errors.New("smtp down") })
	if !exists(filepath.Join(dir, "notified-app")) || exists(filepath.Join(dir, "notified-email")) {
		t.Fatalf("after a failed email: app marker %v, email marker %v; want true, false",
			exists(filepath.Join(dir, "notified-app")), exists(filepath.Join(dir, "notified-email")))
	}

	var sent int
	reportUnrestored(context.Background(), app, func(context.Context, core.App, notice) error { sent++; return nil })
	if sent != 1 || !exists(filepath.Join(dir, "notified-email")) {
		t.Fatalf("retry sent %d emails, email marker present: %v", sent, exists(filepath.Join(dir, "notified-email")))
	}
	if got := len(unrestoredNotifications(t, app)); got != 1 {
		t.Fatalf("%d in-app notifications after the retry, want 1", got)
	}
}

// With no administrator to tell, no in-app marker is written, so the first
// owner created later is still told.
func TestReportUnrestoredWithoutAdministratorsRetries(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	app := adminConsoleTestApp(t)
	dir := keepUnrestored(t, "build-9", true)

	reportUnrestored(context.Background(), app, func(context.Context, core.App, notice) error { return nil })
	if exists(filepath.Join(dir, "notified-app")) {
		t.Fatal("in-app marker written although no administrator exists")
	}
}

// A crash between the supervisor's two renames can leave the copy without
// its note. It is still a kept backup: it is reported and listed.
func TestUnrestoredCopyWithoutANoteIsReported(t *testing.T) {
	app := unrestoredTestApp(t)
	dir := keepUnrestored(t, "build-7", false)

	notes, err := listUnrestored()
	mustNil(t, err)
	if len(notes) != 1 || notes[0].Build != "build-7" {
		t.Fatalf("listUnrestored() = %+v, want build-7", notes)
	}
	if !hasUnrestored() {
		t.Fatal("hasUnrestored() = false for a copy without a note")
	}

	var sent []notice
	reportUnrestored(context.Background(), app, func(_ context.Context, _ core.App, n notice) error { sent = append(sent, n); return nil })
	if len(sent) != 1 || !strings.Contains(sent[0].BodyText, "build build-7") {
		t.Fatalf("sent %+v, want one notice naming build-7", sent)
	}
	if !exists(filepath.Join(dir, "notified-email")) {
		t.Fatal("no email marker")
	}
}

// A dir the operator emptied (only a marker is left) holds nothing.
func TestUnrestoredIgnoresADirWithoutACopy(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	if hasUnrestored() {
		t.Fatal("hasUnrestored() = true with no unrestored dir")
	}
	dir := filepath.Join(stateUnrestoredDir(), "build-5")
	mustNil(t, os.MkdirAll(dir, 0o755))
	mustNil(t, os.WriteFile(filepath.Join(dir, "notified-app"), nil, 0o644))
	if hasUnrestored() {
		t.Fatal("hasUnrestored() = true for a dir with no copy and no note")
	}
}

// While a backup is kept, the database schema is ahead of the build that
// serves it, so the tick starts nothing; it runs again once the dir is gone.
func TestTickHoldsWhileABackupIsUnrestored(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	s, applied, sent := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	dir := keepUnrestored(t, "build-9", true)

	// The status says so from boot on, before the first tick.
	st, err := s.Status(context.Background())
	mustNil(t, err)
	if st.LastResult != resultUnrestored || !st.NextCheck.IsZero() {
		t.Fatalf("status before a tick: %+v, want %q and no next check", st, resultUnrestored)
	}

	if got := s.tick(context.Background()); got != resultUnrestored {
		t.Fatalf("result %q, want %q", got, resultUnrestored)
	}
	if len(*applied) != 0 || len(*sent) != 0 {
		t.Fatalf("held tick applied %+v and sent %+v", *applied, *sent)
	}
	st, err = s.Status(context.Background())
	mustNil(t, err)
	if st.LastResult != resultUnrestored {
		t.Fatalf("status last result %q, want %q", st.LastResult, resultUnrestored)
	}

	mustNil(t, os.RemoveAll(dir))
	if got := s.tick(context.Background()); got != "upgrading" {
		t.Fatalf("after the dir is removed: result %q", got)
	}
}

// A mail server that never answers must not hold the serve chain: the
// supervisor waits only 60 s for ready, which follows the chain. The notices
// run beside it, and terminating the app ends them.
func TestServeDoesNotWaitForBootNotices(t *testing.T) {
	app := unrestoredTestApp(t)
	keepUnrestored(t, "build-9", true)

	mailing := make(chan struct{})
	mailEnded := make(chan struct{})
	registerStaticServeWith(app, Options{TypesDir: t.TempDir()}, func(ctx context.Context, _ core.App, _ notice) error {
		close(mailing)
		<-ctx.Done()
		close(mailEnded)
		return ctx.Err()
	})
	reached := make(chan struct{})
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Priority: 1000, // after the static serve handler
		Func: func(e *core.ServeEvent) error {
			close(reached)
			return e.Next()
		},
	})

	serveErr := make(chan error, 1)
	go func() {
		e := new(core.ServeEvent)
		e.App = app
		e.Router = router.NewRouter[*core.RequestEvent](nil)
		serveErr <- app.OnServe().Trigger(e)
	}()
	awaitClosed(t, reached, "the serve chain to pass the static serve handler")
	awaitClosed(t, mailing, "the boot notice to start its email")
	mustNil(t, <-serveErr)

	mustNil(t, app.OnTerminate().Trigger(&core.TerminateEvent{App: app}))
	select {
	case <-mailEnded:
	default:
		t.Fatal("terminating the app returned before the boot notices ended")
	}
}

// awaitClosed fails the test when ch is not closed by the deadline.
func awaitClosed(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
