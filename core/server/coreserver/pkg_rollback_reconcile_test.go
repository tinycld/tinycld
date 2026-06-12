package coreserver

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newRollbackReconcileTestApp builds a test app with a minimal pkg_install_log
// collection (the subset ReconcileRolledBackInstall reads/writes) and points
// statePbDataDir() at a writable temp dir so the marker file can be created.
func newRollbackReconcileTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	// Isolate the state dir so rollbackPendingMarkerPath() resolves under a temp
	// pb_data we control. statePbDataDir() = <TINYCLD_STATE_DIR>/pb_data.
	stateDir := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", stateDir)
	if err := os.MkdirAll(filepath.Join(stateDir, "pb_data"), 0o755); err != nil {
		t.Fatalf("mkdir pb_data: %v", err)
	}

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	c := core.NewBaseCollection("pkg_install_log")
	c.Fields.Add(&core.SelectField{
		Name: "action", Required: true, MaxSelect: 1,
		Values: []string{"install", "uninstall", "enable", "disable", "revert", "version_change"},
	})
	c.Fields.Add(&core.TextField{Name: "pkg_slug", Required: true})
	c.Fields.Add(&core.TextField{Name: "npm_package"})
	c.Fields.Add(&core.SelectField{
		Name: "status", Required: true, MaxSelect: 1,
		Values: []string{"pending", "running", "success", "failed", "rolled_back"},
	})
	c.Fields.Add(&core.TextField{Name: "error", Max: 5000})
	c.Fields.Add(&core.TextField{Name: "job_id"})
	c.Fields.Add(&core.DateField{Name: "started_at"})
	c.Fields.Add(&core.DateField{Name: "completed_at"})
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(c); err != nil {
		t.Fatalf("save pkg_install_log collection: %v", err)
	}
	return app
}

// addInstallLog inserts a pkg_install_log row with the given slug/status and
// returns its id.
func addInstallLog(t *testing.T, app *tests.TestApp, slug, status string) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		t.Fatalf("find pkg_install_log: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("action", "install")
	rec.Set("pkg_slug", slug)
	rec.Set("status", status)
	rec.Set("started_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	if err := app.Save(rec); err != nil {
		t.Fatalf("save install-log row: %v", err)
	}
	return rec.Id
}

func writeRollbackMarker(t *testing.T, buildID string) {
	t.Helper()
	if err := os.WriteFile(rollbackPendingMarkerPath(), []byte(buildID), 0o644); err != nil {
		t.Fatalf("write rollback marker: %v", err)
	}
}

func markerExists() bool {
	_, err := os.Stat(rollbackPendingMarkerPath())
	return err == nil
}

func TestReconcileMarksStrandedRunningRowRolledBack(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	id := addInstallLog(t, app, "todo", "running")
	writeRollbackMarker(t, "build-123")

	ReconcileRolledBackInstall(app)

	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got := rec.GetString("status"); got != "rolled_back" {
		t.Fatalf("status = %q, want rolled_back", got)
	}
	if rec.GetString("completed_at") == "" {
		t.Fatalf("completed_at should be set after reconcile")
	}
	if rec.GetString("error") == "" {
		t.Fatalf("error should describe the rollback")
	}
	if markerExists() {
		t.Fatalf("marker should be consumed (deleted) after a successful reconcile")
	}
}

func TestReconcileNoMarkerIsNoOp(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	id := addInstallLog(t, app, "todo", "running")

	ReconcileRolledBackInstall(app)

	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got := rec.GetString("status"); got != "running" {
		t.Fatalf("status = %q, want running (untouched with no marker)", got)
	}
}

func TestReconcileMarkerButOnlySuccessRowDeletesMarker(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	id := addInstallLog(t, app, "todo", "success")
	writeRollbackMarker(t, "build-123")

	ReconcileRolledBackInstall(app)

	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got := rec.GetString("status"); got != "success" {
		t.Fatalf("status = %q, want success (a non-running row must not be touched)", got)
	}
	if markerExists() {
		t.Fatalf("marker should be deleted even when there is nothing to reconcile")
	}
}

func TestReconcileDefersWhenJobInFlight(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	id := addInstallLog(t, app, "todo", "running")
	writeRollbackMarker(t, "build-123")

	// Simulate a genuinely in-flight install (cannot happen on a fresh boot, but
	// the guard must hold). Restore the global afterwards so other tests are clean.
	installMu.Lock()
	currentJob = &installJob{ID: "job_test", Action: "install", Status: "running"}
	installMu.Unlock()
	t.Cleanup(func() {
		installMu.Lock()
		currentJob = nil
		installMu.Unlock()
	})

	ReconcileRolledBackInstall(app)

	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got := rec.GetString("status"); got != "running" {
		t.Fatalf("status = %q, want running (must defer while a job is in-flight)", got)
	}
	if !markerExists() {
		t.Fatalf("marker should be kept when reconcile is deferred")
	}
}
