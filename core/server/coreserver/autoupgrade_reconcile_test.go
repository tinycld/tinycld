package coreserver

import (
	"testing"
	"time"
)

func TestReconcileBlocksRolledBackAutoJob(t *testing.T) {
	app := adminConsoleTestApp(t)
	newInstallLogRow(t, app) // auto, rolled_back, mail -> 0.6.0
	var sent []notice
	notify := func(n notice) { sent = append(sent, n) }
	now := time.Date(2026, 10, 1, 3, 10, 0, 0, time.UTC)

	reconcileAutoUpgradeResults(app, now, notify)
	reconcileAutoUpgradeResults(app, now, notify)

	fps, err := blockedFingerprints(app)
	mustNil(t, err)
	if !fps[fingerprint(map[string]string{"mail": "0.6.0"}, nil)] {
		t.Fatal("set not blocked")
	}
	if len(sent) != 1 {
		t.Fatalf("%d emails, want 1", len(sent))
	}
}

func TestReconcileIgnoresManualAndFailedJobs(t *testing.T) {
	app := adminConsoleTestApp(t)
	id := newInstallLogRow(t, app)
	row, _ := app.FindRecordById("pkg_install_log", id)
	row.Set("trigger", "manual")
	mustNil(t, app.Save(row))
	id2 := newInstallLogRow(t, app)
	row2, _ := app.FindRecordById("pkg_install_log", id2)
	row2.Set("status", "failed")
	mustNil(t, app.Save(row2))

	reconcileAutoUpgradeResults(app, time.Now(), func(notice) { t.Fatal("must not notify") })
	if fps, _ := blockedFingerprints(app); len(fps) != 0 {
		t.Fatalf("blocked %v", fps)
	}
}
