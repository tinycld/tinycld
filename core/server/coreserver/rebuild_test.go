package coreserver

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestRebuildManifest_RoundTrip(t *testing.T) {
	m := RebuildManifest{
		BuildID: "build-1234",
		Members: []MemberSpec{
			{Slug: "tinycld", Version: "1.2.0", Spec: "git+https://github.com/tinycld/tinycld#v1.2.0"},
			{Slug: "mail", Version: "0.3.1", Spec: "@tinycld/mail@0.3.1"},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got RebuildManifest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.BuildID != "build-1234" || len(got.Members) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Members[1].Slug != "mail" || got.Members[1].Spec != "@tinycld/mail@0.3.1" {
		t.Fatalf("member mismatch: %+v", got.Members[1])
	}
}

func TestRebuildManifest_MemberBySlug(t *testing.T) {
	m := RebuildManifest{Members: []MemberSpec{{Slug: "mail"}, {Slug: "calc"}}}
	if ms, ok := m.MemberBySlug("calc"); !ok || ms.Slug != "calc" {
		t.Fatalf("MemberBySlug(calc) failed: %+v ok=%v", ms, ok)
	}
	if _, ok := m.MemberBySlug("absent"); ok {
		t.Fatal("MemberBySlug(absent) should be !ok")
	}
}

func setRegistryRow(t *testing.T, app core.App, slug, status, version, npmPkg string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("slug", slug)
	r.Set("status", status)
	r.Set("version", version)
	r.Set("npm_package", npmPkg)
	if err := app.Save(r); err != nil {
		t.Fatalf("save registry row %s: %v", slug, err)
	}
}

func TestBuildCurrentMemberSet_MapsCoreToTinycld(t *testing.T) {
	app := newMigrateTestApp(t)
	addPkgRegistryCollection(t, app)
	setRegistryRow(t, app, "core", "bundled", "1.0.0", "git+https://x/tinycld")
	setRegistryRow(t, app, "mail", "installed", "0.3.1", "@tinycld/mail@0.3.1")
	setRegistryRow(t, app, "calc", "available", "0.1.0", "@tinycld/calc@0.1.0") // excluded

	set, err := buildCurrentMemberSet(app)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := map[string]MemberSpec{}
	for _, ms := range set {
		bySlug[ms.Slug] = ms
	}
	if _, ok := bySlug["tinycld"]; !ok {
		t.Fatal("core row should map to tinycld member")
	}
	if _, ok := bySlug["mail"]; !ok {
		t.Fatal("installed mail missing")
	}
	if _, ok := bySlug["calc"]; ok {
		t.Fatal("available calc should be excluded")
	}
}

func TestCommitRegistry_MirrorsManifest(t *testing.T) {
	app := newMigrateTestApp(t)
	addPkgRegistryCollection(t, app)
	setRegistryRow(t, app, "core", "bundled", "1.0.0", "git+https://x/tinycld")
	setRegistryRow(t, app, "mail", "installed", "0.3.1", "@tinycld/mail@0.3.1")

	// Desired set upgrades mail and drops nothing; core stays.
	m := RebuildManifest{
		BuildID: "build-x",
		Members: []MemberSpec{
			{Slug: "tinycld", Version: "1.0.0", Spec: "git+https://x/tinycld"},
			{Slug: "mail", Version: "0.4.0", Spec: "@tinycld/mail@0.4.0"},
		},
	}
	if err := commitRegistry(app, m, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	mail, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatal(err)
	}
	if mail.GetString("version") != "0.4.0" {
		t.Fatalf("mail version = %s, want 0.4.0", mail.GetString("version"))
	}
}

func TestCommitRegistry_DisablesDroppedMember(t *testing.T) {
	app := newMigrateTestApp(t)
	addPkgRegistryCollection(t, app)
	setRegistryRow(t, app, "core", "bundled", "1.0.0", "git+https://x/tinycld")
	setRegistryRow(t, app, "mail", "installed", "0.3.1", "@tinycld/mail@0.3.1")

	// Desired set drops mail (uninstall).
	m := RebuildManifest{
		BuildID: "build-y",
		Members: []MemberSpec{{Slug: "tinycld", Version: "1.0.0", Spec: "git+https://x/tinycld"}},
	}
	if err := commitRegistry(app, m, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	mail, err := app.FindFirstRecordByFilter("pkg_registry", "slug = 'mail'", nil)
	if err != nil {
		t.Fatal(err)
	}
	if mail.GetString("status") != "disabled" {
		t.Fatalf("mail status = %s, want disabled", mail.GetString("status"))
	}
}

func TestDesiredSet_Install(t *testing.T) {
	current := []MemberSpec{
		{Slug: "tinycld", Version: "1.0.0", Spec: "git+https://x/tinycld#v1.0.0"},
	}
	delta := setDelta{op: "install", slug: "mail", version: "0.3.1", spec: "@tinycld/mail@0.3.1"}
	m := desiredSet("build-1", current, delta)
	if _, ok := m.MemberBySlug("mail"); !ok {
		t.Fatal("mail not added")
	}
	if len(m.Members) != 2 {
		t.Fatalf("want 2 members, got %d", len(m.Members))
	}
}

func TestDesiredSet_Uninstall(t *testing.T) {
	current := []MemberSpec{{Slug: "tinycld"}, {Slug: "mail"}}
	m := desiredSet("build-2", current, setDelta{op: "uninstall", slug: "mail"})
	if _, ok := m.MemberBySlug("mail"); ok {
		t.Fatal("mail should be removed")
	}
	if _, ok := m.MemberBySlug("tinycld"); !ok {
		t.Fatal("tinycld must remain")
	}
}

func TestDesiredSet_Upgrade_OverridesSpec(t *testing.T) {
	current := []MemberSpec{
		{Slug: "tinycld"},
		{Slug: "mail", Version: "0.3.1", Spec: "@tinycld/mail@0.3.1"},
	}
	m := desiredSet("build-3", current, setDelta{op: "version", slug: "mail", version: "0.4.0", spec: "@tinycld/mail@0.4.0"})
	ms, _ := m.MemberBySlug("mail")
	if ms.Version != "0.4.0" || ms.Spec != "@tinycld/mail@0.4.0" {
		t.Fatalf("mail not upgraded: %+v", ms)
	}
}

func TestRebuild_HappyPath_Sequence(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var seq []string
	deps := rebuildDeps{
		assemble: func(m RebuildManifest, dir string) error {
			seq = append(seq, "assemble")
			return os.MkdirAll(filepath.Join(dir, "tinycld", "server", "pb_migrations"), 0o755)
		},
		pipeline: func(j *installJob, dir string) (buildOutput, error) {
			seq = append(seq, "pipeline")
			return buildOutput{}, nil
		},
		backupDB:       func() error { seq = append(seq, "backup"); return nil },
		syncMig:        func(buildDir string) (SyncResult, error) { seq = append(seq, "sync"); return SyncResult{}, nil },
		activate:       func(id string) error { seq = append(seq, "activate"); return nil },
		recordBuild:    func(out buildOutput) error { seq = append(seq, "record"); return nil },
		commitRegistry: func() error { seq = append(seq, "commit"); return nil },
		prune:          func(keep int) error { seq = append(seq, "prune"); return nil },
		finalizeLog:    func(status, errMsg string) { seq = append(seq, "finalize") },
		restart:        func() { seq = append(seq, "restart") },
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	m := RebuildManifest{BuildID: "build-1", Members: []MemberSpec{{Slug: "tinycld", Spec: "x"}}}
	if err := rebuildWith(job, m, deps); err != nil {
		t.Fatal(err)
	}
	want := "assemble,pipeline,backup,sync,activate,record,commit,prune,finalize,restart"
	if got := strings.Join(seq, ","); got != want {
		t.Fatalf("sequence = %s, want %s", got, want)
	}
}

func TestRebuild_FinalizesLogOnFailure(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var finalized string
	deps := rebuildDeps{
		assemble:    func(m RebuildManifest, dir string) error { return fmt.Errorf("assemble broke") },
		pipeline:    func(j *installJob, dir string) (buildOutput, error) { return buildOutput{}, nil },
		backupDB:    func() error { return nil },
		syncMig:     func(buildDir string) (SyncResult, error) { return SyncResult{}, nil },
		activate:    func(id string) error { return nil },
		prune:       func(keep int) error { return nil },
		finalizeLog: func(status, errMsg string) { finalized = status },
		restart:     func() {},
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := rebuildWith(job, RebuildManifest{BuildID: "build-f"}, deps); err == nil {
		t.Fatal("expected error")
	}
	if finalized != "failed" {
		t.Fatalf("finalizeLog status = %q, want failed", finalized)
	}
}

func TestRebuild_PipelineFailure_NoActivateNoRestore(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var restored, activated bool
	deps := rebuildDeps{
		assemble:  func(m RebuildManifest, dir string) error { return nil },
		pipeline:  func(j *installJob, dir string) (buildOutput, error) { return buildOutput{}, fmt.Errorf("build broke") },
		backupDB:  func() error { return nil },
		restoreDB: func() error { restored = true; return nil },
		syncMig:   func(buildDir string) (SyncResult, error) { return SyncResult{}, nil },
		activate:  func(id string) error { activated = true; return nil },
		prune:     func(keep int) error { return nil },
		restart:   func() {},
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := rebuildWith(job, RebuildManifest{BuildID: "build-2"}, deps); err == nil {
		t.Fatal("expected error")
	}
	if activated {
		t.Fatal("must NOT activate after pipeline failure")
	}
	// Failure precedes the backup, so restore should not run.
	if restored {
		t.Fatal("restore should not run when failure precedes backup")
	}
}

// writeFakeBackup drops a stand-in data.db.backup under the state's pb_data dir
// so the armed-backup helpers have a file to act on without running sqlite3.
func writeFakeBackup(t *testing.T, state string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(state, "pb_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(state, "pb_data", "data.db.backup")
	if err := os.WriteFile(p, []byte("fake-snapshot"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// armDatabaseBackup must LEAVE the backup file in place (it's the armed rollback
// snapshot) and write the marker recording the build it predates. This is the
// crux of review finding H3: the success-with-exit-75 path keeps the backup so
// the entrypoint can restore the DB if the new binary fails its health probe.
func TestArmDatabaseBackup_LeavesBackupAndWritesMarker(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	backup := writeFakeBackup(t, state)

	armDatabaseBackup("build-42")

	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("armed backup must survive (not be deleted): %v", err)
	}
	got, err := os.ReadFile(dbArmedMarkerPath())
	if err != nil {
		t.Fatalf("arm marker not written: %v", err)
	}
	if string(got) != "build-42" {
		t.Fatalf("arm marker = %q, want build-42", got)
	}
}

// With no backup on disk there's nothing to arm; arming must not create a marker
// and must clear any stale one (e.g. left by a prior aborted op).
func TestArmDatabaseBackup_NoBackup_ClearsStaleMarker(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	if err := os.MkdirAll(filepath.Join(state, "pb_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dbArmedMarkerPath(), []byte("stale-build"), 0o644); err != nil {
		t.Fatal(err)
	}

	armDatabaseBackup("build-99") // no data.db.backup present

	if _, err := os.Stat(dbArmedMarkerPath()); !os.IsNotExist(err) {
		t.Fatal("stale arm marker should be cleared when there's no backup to arm")
	}
}

// The pre-activation restore closure (used ONLY for migrate/activate failures
// before the symlink swaps) must DISARM: remove both the backup file and the arm
// marker, since the live `current` never moved and there's no post-restart
// rollback to keep the backup for. Contrast with the success path, which arms.
func TestBackupRestoreClosure_DisarmsMarkerAndBackup(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)

	// backupDatabase shells out to sqlite3 (VACUUM INTO); require it here — it's
	// present in CI + the runtime image, and exercising the REAL restore closure
	// is the point of the test.
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 not on PATH; restore-closure disarm test needs it")
	}
	if err := os.MkdirAll(filepath.Join(state, "pb_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A valid (empty) SQLite db for VACUUM INTO to snapshot. Let sqlite3 create
	// the file fresh (a non-DB placeholder would fail VACUUM with "file is not a
	// database").
	dbFile := filepath.Join(state, "pb_data", "data.db")
	if out, err := exec.Command("sqlite3", dbFile, "VACUUM;").CombinedOutput(); err != nil {
		t.Fatalf("init sqlite db: %v\n%s", err, out)
	}
	restoreFn, err := backupDatabase(filepath.Join(state, "builds", "x", "tinycld"))
	if err != nil {
		t.Fatalf("backupDatabase: %v", err)
	}
	// backupDatabase re-creates the backup; re-arm the marker to mimic an armed
	// state, then prove the restore closure disarms it.
	if err := os.WriteFile(dbArmedMarkerPath(), []byte("build-7"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Stale WAL/SHM from the forward-migrated DB: the restore must delete these or
	// SQLite would replay them over the snapshot and undo the restore.
	dbWAL := dbFile + "-wal"
	dbSHM := dbFile + "-shm"
	for _, f := range []string{dbWAL, dbSHM} {
		if err := os.WriteFile(f, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := restoreFn(); err != nil {
		t.Fatalf("restore closure: %v", err)
	}
	if _, err := os.Stat(dbBackupPath()); !os.IsNotExist(err) {
		t.Fatal("restore closure must remove the backup file (disarm)")
	}
	if _, err := os.Stat(dbArmedMarkerPath()); !os.IsNotExist(err) {
		t.Fatal("restore closure must remove the arm marker (disarm)")
	}
	if _, err := os.Stat(dbWAL); !os.IsNotExist(err) {
		t.Fatal("restore closure must remove the stale -wal (else SQLite replays it)")
	}
	if _, err := os.Stat(dbSHM); !os.IsNotExist(err) {
		t.Fatal("restore closure must remove the stale -shm")
	}
}

// The happy-path rebuild must NOT invoke the restore closure (so the backup is
// left armed for the entrypoint), and its restart step must arm the marker.
func TestRebuild_HappyPath_ArmsBackup_NoRestore(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	backup := writeFakeBackup(t, state)
	var restored bool
	deps := rebuildDeps{
		assemble: func(m RebuildManifest, dir string) error {
			return os.MkdirAll(filepath.Join(dir, "tinycld", "server", "pb_migrations"), 0o755)
		},
		pipeline:  func(j *installJob, dir string) (buildOutput, error) { return buildOutput{}, nil },
		backupDB:  func() error { return nil }, // the fake backup already exists on disk
		restoreDB: func() error { restored = true; return nil },
		syncMig:   func(buildDir string) (SyncResult, error) { return SyncResult{}, nil },
		activate:  func(id string) error { return nil },
		// Mirror the production restart closure's arming so the test covers the
		// real success-path behavior (arm marker written, backup left in place).
		restart: func() { armDatabaseBackup("build-armed") },
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	m := RebuildManifest{BuildID: "build-armed", Members: []MemberSpec{{Slug: "tinycld", Spec: "x"}}}
	if err := rebuildWith(job, m, deps); err != nil {
		t.Fatal(err)
	}
	if restored {
		t.Fatal("happy path must NOT restore the DB — the backup stays armed for the entrypoint")
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("backup must survive the success path (armed): %v", err)
	}
	got, err := os.ReadFile(dbArmedMarkerPath())
	if err != nil || string(got) != "build-armed" {
		t.Fatalf("arm marker = %q (err %v), want build-armed", got, err)
	}
}

func TestRebuild_MigrateFailure_RestoresAndNoActivate(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var restored, activated bool
	deps := rebuildDeps{
		assemble:  func(m RebuildManifest, dir string) error { return nil },
		pipeline:  func(j *installJob, dir string) (buildOutput, error) { return buildOutput{}, nil },
		backupDB:  func() error { return nil },
		restoreDB: func() error { restored = true; return nil },
		syncMig:   func(buildDir string) (SyncResult, error) { return SyncResult{}, fmt.Errorf("down broke") },
		activate:  func(id string) error { activated = true; return nil },
		prune:     func(keep int) error { return nil },
		restart:   func() {},
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := rebuildWith(job, RebuildManifest{BuildID: "build-3"}, deps); err == nil {
		t.Fatal("expected error")
	}
	if !restored {
		t.Fatal("DB should be restored after a migration failure")
	}
	if activated {
		t.Fatal("must NOT activate after migration failure")
	}
}
