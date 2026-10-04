package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/installjob"
)

// newRollbackReconcileTestApp builds a test app with a minimal pkg_install_log
// collection (the subset ReconcileRolledBackInstall reads/writes) and points
// statePbDataDir() at a writable temp dir so the marker file can be created.
func newRollbackReconcileTestApp(t *testing.T) *tests.TestApp {
	t.Helper()

	// Isolate the state dir so the rollback record and the legacy pb_data
	// marker resolve under a temp dir we control.
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

// writeRollbackMarker writes the record the supervisor leaves in the state
// dir when it rolls buildID back.
func writeRollbackMarker(t *testing.T, buildID string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"build": buildID, "rolled_to": "build-prev", "at": time.Now().UTC()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stateRollbackRecordPath(), data, 0o644); err != nil {
		t.Fatalf("write rollback record: %v", err)
	}
}

func markerExists() bool {
	_, err := os.Stat(stateRollbackRecordPath())
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
	if got := rec.GetString("error"); !strings.Contains(got, "(build build-123)") {
		t.Fatalf("error = %q, want it to name the rolled-back build", got)
	}
	if markerExists() {
		t.Fatalf("marker should be consumed (deleted) after a successful reconcile")
	}
}

// A supervisor from before the state-dir record left the failed build id as
// plain text in pb_data. A boot after the upgrade still reads it once.
func TestReconcileReadsTheLegacyPbDataMarker(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	id := addInstallLog(t, app, "todo", "running")
	if err := os.WriteFile(legacyRollbackMarkerPath(), []byte("build-77\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ReconcileRolledBackInstall(app)

	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got := rec.GetString("status"); got != "rolled_back" {
		t.Fatalf("status = %q, want rolled_back", got)
	}
	if got := rec.GetString("error"); !strings.Contains(got, "(build build-77)") {
		t.Fatalf("error = %q, want it to name the legacy marker's build", got)
	}
	if _, err := os.Stat(legacyRollbackMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("the legacy marker should be consumed, stat err = %v", err)
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
	inflight := installjob.New("install", "", "")
	if _, ok := installjob.Claim(inflight); !ok {
		t.Fatal("claim must win against an idle interlock")
	}
	t.Cleanup(func() { installjob.Release(inflight) })

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
