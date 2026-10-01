package coreserver

import (
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func TestNotifyDue(t *testing.T) {
	now := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	if !notifyDue(now, false, now) {
		t.Error("a changed fingerprint must notify")
	}
	if notifyDue(now.Add(-6*24*time.Hour), true, now) {
		t.Error("same fingerprint within 7 days must not notify")
	}
	if !notifyDue(now.Add(-7*24*time.Hour), true, now) {
		t.Error("7-day reminder must notify")
	}
}

func TestRecordPauseGate(t *testing.T) {
	app := adminConsoleTestApp(t)
	var sent []notice
	notify := func(n notice) { sent = append(sent, n) }
	t0 := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	wanted := map[string]string{"mail": "0.6.0"}

	mustNil(t, recordPause(app, "fp1", wanted, "mail needs x", t0, notify))
	mustNil(t, recordPause(app, "fp1", wanted, "mail needs x", t0.Add(time.Hour), notify))
	if len(sent) != 1 {
		t.Fatalf("same pause sent %d emails, want 1", len(sent))
	}
	mustNil(t, recordPause(app, "fp2", wanted, "mail needs y", t0.Add(2*time.Hour), notify))
	if len(sent) != 2 {
		t.Fatalf("changed fingerprint: %d emails, want 2", len(sent))
	}
	mustNil(t, recordPause(app, "fp2", wanted, "mail needs y", t0.Add(2*time.Hour+7*24*time.Hour), notify))
	if len(sent) != 3 {
		t.Fatalf("reminder: %d emails, want 3", len(sent))
	}
	rows, _ := app.FindRecordsByFilter("autoupgrade_state", "kind = 'pause'", "", 0, 0)
	if len(rows) != 1 {
		t.Fatalf("%d pause rows, want 1", len(rows))
	}

	mustNil(t, clearPause(app))
	mustNil(t, recordPause(app, "fp2", wanted, "mail needs y", t0.Add(8*24*time.Hour), notify))
	if len(sent) != 4 {
		t.Fatalf("pause after clear: %d emails, want 4", len(sent))
	}
}

func TestRecordBlockedOncePerInstallLog(t *testing.T) {
	app := adminConsoleTestApp(t)
	logID := newInstallLogRow(t, app)
	var sent []notice
	notify := func(n notice) { sent = append(sent, n) }
	t0 := time.Date(2026, 10, 1, 3, 0, 0, 0, time.UTC)
	target := map[string]string{"mail": "0.6.0"}

	mustNil(t, recordBlocked(app, "fpA", target, "rolled back", logID, t0, notify))
	mustNil(t, recordBlocked(app, "fpA", target, "rolled back", logID, t0, notify))
	if len(sent) != 1 {
		t.Fatalf("%d emails, want 1", len(sent))
	}
	fps, err := blockedFingerprints(app)
	mustNil(t, err)
	if !fps["fpA"] {
		t.Fatal("fpA not blocked")
	}

	mustNil(t, remindBlocked(app, t0.Add(24*time.Hour), notify))
	if len(sent) != 1 {
		t.Fatal("reminder sent before 7 days")
	}
	mustNil(t, remindBlocked(app, t0.Add(7*24*time.Hour), notify))
	if len(sent) != 2 {
		t.Fatal("7-day reminder not sent")
	}

	row, _ := app.FindFirstRecordByFilter("autoupgrade_state", "kind = 'blocked'")
	row.Set("cleared", true)
	mustNil(t, app.Save(row))
	fps, _ = blockedFingerprints(app)
	if fps["fpA"] {
		t.Fatal("a cleared set must not be blocked")
	}
}

func TestNotifyAdminsRecipients(t *testing.T) {
	app := adminConsoleTestApp(t)
	newUser(t, app, "owner@x.test", "owner", false)
	newUser(t, app, "admin@x.test", "admin", false)
	newUser(t, app, "member@x.test", "member", false)
	newUser(t, app, "gone@x.test", "admin", true)
	var to []string
	notifyAdmins(app, func(_, email, _, _, _ string) { to = append(to, email) }, pauseNotice(map[string]string{"mail": "0.6.0"}, "r", false))
	if len(to) != 2 {
		t.Fatalf("sent to %v, want owner + admin", to)
	}
}

func mustNil(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func newInstallLogRow(t *testing.T, app core.App) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_install_log")
	mustNil(t, err)
	r := core.NewRecord(col)
	r.Set("action", "version_change")
	r.Set("pkg_slug", "mail")
	r.Set("status", "rolled_back")
	r.Set("trigger", "auto")
	r.Set("changes", []map[string]string{{"slug": "mail", "targetVersion": "0.6.0"}})
	mustNil(t, app.Save(r))
	return r.Id
}

func newUser(t *testing.T, app core.App, email, role string, disabled bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	mustNil(t, err)
	u := core.NewRecord(col)
	u.SetEmail(email)
	u.SetPassword("Password1234!")
	u.SetVerified(true)
	u.Set("name", role)
	u.Set("role", role)
	u.Set("disabled", disabled)
	mustNil(t, app.Save(u))
	return u
}
