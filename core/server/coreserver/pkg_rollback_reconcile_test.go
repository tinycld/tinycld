package coreserver

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/dbx"
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
	addInstallLogCollection(t, app)
	return app
}

// addInstallLogCollection creates the subset of pkg_install_log that the
// rebuild and the reconciler read and write, for an app that does not run
// the core migrations.
func addInstallLogCollection(t *testing.T, app core.App) {
	t.Helper()
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
	c.Fields.Add(&core.TextField{Name: "log"})
	c.Fields.Add(&core.TextField{Name: "error", Max: 5000})
	c.Fields.Add(&core.TextField{Name: "job_id"})
	c.Fields.Add(&core.TextField{Name: "build_id"})
	c.Fields.Add(&core.SelectField{Name: "trigger", MaxSelect: 1, Values: []string{"manual", "auto"}})
	c.Fields.Add(&core.JSONField{Name: "changes"})
	c.Fields.Add(&core.JSONField{Name: "steps", MaxSize: 2000000})
	c.Fields.Add(&core.TextField{Name: "current_step", Max: 200})
	c.Fields.Add(&core.TextField{Name: "current_message", Max: 500})
	c.Fields.Add(&core.DateField{Name: "started_at"})
	c.Fields.Add(&core.DateField{Name: "completed_at"})
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(c); err != nil {
		t.Fatalf("save pkg_install_log collection: %v", err)
	}
}

// addInstallLog inserts a pkg_install_log row with the given slug/status and
// no build id (a row written before the build_id field), and returns its id.
func addInstallLog(t *testing.T, app core.App, slug, status string) string {
	t.Helper()
	return addBuildInstallLog(t, app, slug, status, "")
}

// addBuildInstallLog inserts a pkg_install_log row that the given build
// produced, and returns its id.
func addBuildInstallLog(t *testing.T, app core.App, slug, status, buildID string) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		t.Fatalf("find pkg_install_log: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("action", "install")
	rec.Set("pkg_slug", slug)
	rec.Set("status", status)
	rec.Set("build_id", buildID)
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

// The rolled-back build's rows are marked whatever state the restored
// database holds them in: "running" when the snapshot predates the finalize,
// "success" when a restore swap or an unrestored backup left the finalized
// database live. Another build's rows are history and stay as they are.
func TestReconcileMarksEveryRowOfTheRolledBackBuild(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	other := addBuildInstallLog(t, app, "todo", "success", "build-a")
	finalized := addBuildInstallLog(t, app, "todo", "success", "build-b")
	stranded := addBuildInstallLog(t, app, "mail", "running", "build-b")
	writeRollbackMarker(t, "build-b")

	ReconcileRolledBackInstall(app)

	for _, id := range []string{finalized, stranded} {
		assertRolledBack(t, app, id)
	}
	if got := statusOf(t, app, other); got != "success" {
		t.Fatalf("another build's row status = %q, want success", got)
	}
	if markerExists() {
		t.Fatal("the record should be consumed after a successful reconcile")
	}
}

// A revert re-activates a retained build, so its row carries a build id an
// older row already has. Only the run since the last other build is the
// one rolled back; the earlier install of the same build stays history.
func TestReconcileLeavesAnEarlierRunOfTheSameBuild(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	earlier := addBuildInstallLog(t, app, "todo", "success", "build-b")
	between := addBuildInstallLog(t, app, "todo", "success", "build-c")
	revert := addBuildInstallLog(t, app, "todo", "success", "build-b")
	setCreated(t, app, earlier, "2026-10-01 10:00:00.000Z")
	setCreated(t, app, between, "2026-10-02 10:00:00.000Z")
	setCreated(t, app, revert, "2026-10-03 10:00:00.000Z")
	writeRollbackMarker(t, "build-b")

	ReconcileRolledBackInstall(app)

	assertRolledBack(t, app, revert)
	for _, id := range []string{earlier, between} {
		if got := statusOf(t, app, id); got != "success" {
			t.Fatalf("row %s status = %q, want success", id, got)
		}
	}
}

// setCreated moves a row in time; autodate sets created on insert, and rows
// inserted in one test can share a millisecond.
func setCreated(t *testing.T, app core.App, id, created string) {
	t.Helper()
	if _, err := app.DB().NewQuery("UPDATE pkg_install_log SET created = {:c} WHERE id = {:id}").
		Bind(dbx.Params{"c": created, "id": id}).Execute(); err != nil {
		t.Fatalf("set created: %v", err)
	}
}

func assertRolledBack(t *testing.T, app core.App, id string) {
	t.Helper()
	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	if got := rec.GetString("status"); got != "rolled_back" {
		t.Fatalf("row %s status = %q, want rolled_back", id, got)
	}
	if rec.GetString("completed_at") == "" {
		t.Fatalf("row %s completed_at should be set after reconcile", id)
	}
	if got := rec.GetString("error"); got != "the build failed its health check and was rolled back" {
		t.Fatalf("row %s error = %q", id, got)
	}
}

func statusOf(t *testing.T, app core.App, id string) string {
	t.Helper()
	rec, err := app.FindRecordById("pkg_install_log", id)
	if err != nil {
		t.Fatalf("reload row: %v", err)
	}
	return rec.GetString("status")
}

// A row written before the build_id field cannot be matched by build; the
// newest running row without a build id is still the one the rollback
// stranded, as before the field existed.
func TestReconcileFallsBackToTheNewestRunningRowWithoutABuild(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	done := addInstallLog(t, app, "mail", "success")
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
	if got := rec.GetString("error"); got != "the build failed its health check and was rolled back" {
		t.Fatalf("error = %q", got)
	}
	if got := statusOf(t, app, done); got != "success" {
		t.Fatalf("a finished row without a build id status = %q, want success", got)
	}
	if markerExists() {
		t.Fatalf("marker should be consumed (deleted) after a successful reconcile")
	}
}

// A supervisor from before the state-dir record left the failed build id as
// plain text in pb_data. A boot after the upgrade still reads it once, as
// the build to mark.
func TestReconcileReadsTheLegacyPbDataMarker(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	id := addBuildInstallLog(t, app, "todo", "success", "build-77")
	if err := os.WriteFile(legacyRollbackMarkerPath(), []byte("build-77\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ReconcileRolledBackInstall(app)

	assertRolledBack(t, app, id)
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

// A record whose build matches no row (a rollback before the installer
// wrote one, or rows already marked) is still removed, but with a warning
// that names the build, so a row that should have been marked can be traced.
func TestReconcileWarnsWhenTheRecordedBuildHasNoRow(t *testing.T) {
	app := newRollbackReconcileTestApp(t)
	addBuildInstallLog(t, app, "todo", "success", "build-a")
	writeRollbackMarker(t, "build-gone")
	logs := recordLogs(t)

	ReconcileRolledBackInstall(app)

	if markerExists() {
		t.Fatal("the record should be removed when nothing matches")
	}
	warned := logs.find(slog.LevelWarn, rolledBackNoRowsMessage)
	if len(warned) != 1 || warned[0]["build"] != "build-gone" {
		t.Fatalf("warnings = %+v, want one naming build-gone", warned)
	}
}

// loggedRecords keeps every record logged through the default logger, with
// its attrs as text.
type loggedRecords struct {
	slog.Handler
	mu    *sync.Mutex
	recs  *[]slog.Record
	attrs []slog.Attr
}

// recordLogs swaps the global default logger, so it relies on this
// package's tests not running in parallel; the previous one is restored at
// cleanup.
func recordLogs(t *testing.T) *loggedRecords {
	t.Helper()
	prev := slog.Default()
	// Not prev's handler: wrapping slog's built-in default handler in a new
	// default deadlocks, because that handler writes through the log package,
	// which SetDefault points back at the new default.
	h := &loggedRecords{Handler: slog.NewTextHandler(os.Stderr, nil), mu: &sync.Mutex{}, recs: &[]slog.Record{}}
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func (h *loggedRecords) Handle(ctx context.Context, r slog.Record) error {
	r = r.Clone()
	r.AddAttrs(h.attrs...)
	h.mu.Lock()
	*h.recs = append(*h.recs, r)
	h.mu.Unlock()
	return h.Handler.Handle(ctx, r)
}

func (h *loggedRecords) WithAttrs(as []slog.Attr) slog.Handler {
	return &loggedRecords{Handler: h.Handler, mu: h.mu, recs: h.recs, attrs: append(append([]slog.Attr(nil), h.attrs...), as...)}
}

func (h *loggedRecords) WithGroup(name string) slog.Handler {
	return &loggedRecords{Handler: h.Handler.WithGroup(name), mu: h.mu, recs: h.recs, attrs: h.attrs}
}

// find returns the attrs of each record logged at level with message msg.
func (h *loggedRecords) find(level slog.Level, msg string) []map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []map[string]string
	for _, r := range *h.recs {
		if r.Level != level || r.Message != msg {
			continue
		}
		attrs := map[string]string{}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.String()
			return true
		})
		out = append(out, attrs)
	}
	return out
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

// The reconciler's mark is what blocks an automatic upgrade from being
// tried again: a rolled-back auto row finalized "success" before the restart
// must end up blocking its set. Runs on the real migrations, so the build_id
// field is the one the migration adds.
func TestReconcileRolledBackAutoRowBlocksItsSet(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	app := adminConsoleTestApp(t)
	id := addBuildInstallLog(t, app, "mail", "success", "build-b")
	row, err := app.FindRecordById("pkg_install_log", id)
	mustNil(t, err)
	row.Set("action", "version_change")
	row.Set("trigger", "auto")
	row.Set("changes", []map[string]string{{"slug": "mail", "targetVersion": "0.6.0"}})
	mustNil(t, app.Save(row))
	writeRollbackMarker(t, "build-b")

	ReconcileRolledBackInstall(app)
	reconcileAutoUpgradeResults(app, time.Now(), func(notice) {})

	assertRolledBack(t, app, id)
	fps, err := blockedFingerprints(app)
	mustNil(t, err)
	if !fps[fingerprint(map[string]string{"mail": "0.6.0"}, nil)] {
		t.Fatal("the rolled-back automatic upgrade's set is not blocked")
	}
}
