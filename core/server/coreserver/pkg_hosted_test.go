package coreserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	pbcore "github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// hostedRecorder is a hostedDeps fake that records step order and lets each
// step fail on demand.
type hostedRecorder struct {
	steps []string

	stateLock     map[string]string
	buildMigs     []string
	buildErr      error
	syncResult    SyncResult
	syncErr       error
	deployErr     error
	snapshotErr   error
	restored      bool
	discarded     bool
	recovered     bool
	finalStatus   string
	finalErr      string
	deployedLock  map[string]string
	deployedJobID string
}

func (r *hostedRecorder) deps() hostedDeps {
	return hostedDeps{
		state: func(context.Context) (ctlState, error) {
			r.steps = append(r.steps, "state")
			return ctlState{Lockfile: r.stateLock, RecipeHash: "sha256:old"}, nil
		},
		resolve: func(_ context.Context, spec string) (ctlResolvedSpec, error) {
			r.steps = append(r.steps, "resolve:"+spec)
			return ctlResolvedSpec{}, errors.New("not used in these tests")
		},
		build: func(_ context.Context, lock map[string]string) (ctlBuildResult, error) {
			r.steps = append(r.steps, "build")
			if r.buildErr != nil {
				return ctlBuildResult{}, r.buildErr
			}
			return ctlBuildResult{RecipeHash: "sha256:new", Migrations: r.buildMigs}, nil
		},
		deploy: func(_ context.Context, lock map[string]string, jobID string) error {
			r.steps = append(r.steps, "deploy")
			r.deployedLock = lock
			r.deployedJobID = jobID
			return r.deployErr
		},
		applied: func() ([]string, error) {
			r.steps = append(r.steps, "applied")
			return []string{"1700000000_core.js", "1751000000_todo.js"}, nil
		},
		snapshot: func() (func() error, func() error, error) {
			r.steps = append(r.steps, "snapshot")
			if r.snapshotErr != nil {
				return nil, nil, r.snapshotErr
			}
			restore := func() error { r.restored = true; return nil }
			discard := func() error { r.discarded = true; return nil }
			return restore, discard, nil
		},
		syncMig: func(applied, newSet []string) (SyncResult, error) {
			r.steps = append(r.steps, "sync")
			return r.syncResult, r.syncErr
		},
		recoverDB: func() error { r.recovered = true; return nil },
		finalizeLog: func(status, errMsg string) {
			r.finalStatus, r.finalErr = status, errMsg
		},
	}
}

func newHostedJob(action string) *installJob {
	return &installJob{
		ID: "job_h1", Action: action, Status: "running", Done: make(chan struct{}),
	}
}

func assertSteps(t *testing.T, got, want []string) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
}

func TestRunHostedDeploy_AcceptedLeavesJobRunning(t *testing.T) {
	r := &hostedRecorder{buildMigs: []string{"1700000000_core.js"}}
	job := newHostedJob("install")

	runHostedDeploy(job, r.deps(), map[string]string{"tinycld": "1.0.0"})

	assertSteps(t, r.steps, []string{"build", "applied", "snapshot", "sync", "deploy"})
	if job.Status != "running" {
		t.Fatalf("job status = %q — the readiness of the respawn decides the terminal state, not this process", job.Status)
	}
	if r.finalStatus != "" {
		t.Fatalf("log finalized to %q — the boot reconcile owns finalization", r.finalStatus)
	}
	if r.restored || r.discarded {
		t.Fatal("an accepted deploy must leave the snapshot for the router")
	}
	if r.deployedJobID != "job_h1" {
		t.Fatalf("proposal jobID = %q", r.deployedJobID)
	}
}

func TestRunHostedDeploy_BuildFailureTouchesNothing(t *testing.T) {
	r := &hostedRecorder{buildErr: errors.New("peer refusal: mail requires core >=9")}
	job := newHostedJob("install")

	runHostedDeploy(job, r.deps(), map[string]string{"tinycld": "1.0.0"})

	assertSteps(t, r.steps, []string{"build"})
	if job.Status != "failed" || r.finalStatus != "failed" {
		t.Fatalf("job=%q log=%q, want failed/failed", job.Status, r.finalStatus)
	}
}

func TestRunHostedDeploy_SyncFailureRestores(t *testing.T) {
	r := &hostedRecorder{syncErr: errors.New("down blew up")}
	job := newHostedJob("uninstall")

	runHostedDeploy(job, r.deps(), map[string]string{"tinycld": "1.0.0"})

	if !r.restored {
		t.Fatal("a failed migration sync must restore the snapshot")
	}
	if !r.recovered {
		t.Fatal("an in-process restore must re-open the DB pools")
	}
	if r.finalStatus != "failed" {
		t.Fatalf("log = %q", r.finalStatus)
	}
}

func TestRunHostedDeploy_RefusedProposalAfterDownsRestores(t *testing.T) {
	r := &hostedRecorder{
		syncResult: SyncResult{Reverted: []string{"1751000000_todo.js"}},
		deployErr:  errors.New("a deploy is already in progress for this org"),
	}
	job := newHostedJob("uninstall")

	runHostedDeploy(job, r.deps(), map[string]string{"tinycld": "1.0.0"})

	if !r.restored {
		t.Fatal("a refused proposal after downs must restore the snapshot")
	}
	if r.discarded {
		t.Fatal("restore already consumed the snapshot; discard must not also run")
	}
	if r.finalStatus != "failed" {
		t.Fatalf("log = %q", r.finalStatus)
	}
}

func TestRunHostedDeploy_RefusedProposalWithoutDownsDiscardsSnapshot(t *testing.T) {
	r := &hostedRecorder{deployErr: errors.New("rate-limited")}
	job := newHostedJob("install")

	runHostedDeploy(job, r.deps(), map[string]string{"tinycld": "1.0.0"})

	if r.restored {
		t.Fatal("no downs ran — restoring would clobber post-snapshot user writes")
	}
	if !r.discarded {
		t.Fatal("a stale snapshot must not linger: a later failed operator deploy would restore it over newer data")
	}
}

func TestRunHostedUninstall_RemovesLockfileEntry(t *testing.T) {
	app := newTenantPkgStateApp(t)
	reg := addRegistryRow(t, app, "todo", "1.0.0", "installed")
	reg.Set("npm_package", "@tinycld/todo")
	if err := app.Save(reg); err != nil {
		t.Fatal(err)
	}

	r := &hostedRecorder{stateLock: map[string]string{
		"tinycld": "1.0.0", "@tinycld/todo": "1.0.0",
	}}
	job := newHostedJob("uninstall")
	job.Slug = "todo"

	runHostedUninstall(app, job, r.deps())

	if r.deployedLock == nil {
		t.Fatal("no proposal was made")
	}
	if _, still := r.deployedLock["@tinycld/todo"]; still {
		t.Fatalf("uninstall must drop the member from the proposed lockfile: %v", r.deployedLock)
	}
	if r.deployedLock["tinycld"] != "1.0.0" {
		t.Fatalf("other members must carry through: %v", r.deployedLock)
	}
}

func TestRunHostedVersionChange_EditsLockfile(t *testing.T) {
	app := newTenantPkgStateApp(t)
	reg := addRegistryRow(t, app, "todo", "1.0.0", "installed")
	reg.Set("npm_package", "@tinycld/todo")
	if err := app.Save(reg); err != nil {
		t.Fatal(err)
	}
	coreRow := addRegistryRow(t, app, "core", "0.0.4", "bundled")
	coreRow.Set("npm_package", "tinycld")
	if err := app.Save(coreRow); err != nil {
		t.Fatal(err)
	}

	r := &hostedRecorder{stateLock: map[string]string{
		"tinycld": "1.0.0", "@tinycld/todo": "1.0.0",
	}}
	job := newHostedJob("version_change")
	job.Changes = []versionChange{
		{Slug: "todo", TargetVersion: "2.0.0"},
		{Slug: "core", TargetVersion: "1.1.0"},
	}

	runHostedVersionChange(app, job, r.deps())

	if r.deployedLock["@tinycld/todo"] != "2.0.0" {
		t.Fatalf("todo not moved: %v", r.deployedLock)
	}
	if r.deployedLock["tinycld"] != "1.1.0" {
		t.Fatalf("a core change must move the base lockfile entry: %v", r.deployedLock)
	}
}

func TestHostedNpmSource(t *testing.T) {
	app := newTenantPkgStateApp(t)
	for _, row := range []struct{ slug, spec string }{
		{"todo", "@tinycld/todo"},
		{"pinned", "@tinycld/pinned@1.2.3"},
		{"gitpkg", "github:acme/gitpkg"},
		{"none", ""},
	} {
		rec := addRegistryRow(t, app, row.slug, "1.0.0", "installed")
		if row.spec != "" {
			rec.Set("npm_package", row.spec)
			if err := app.Save(rec); err != nil {
				t.Fatal(err)
			}
		}
	}

	name, current, err := hostedNpmSource(app, "todo")
	if err != nil || name != "@tinycld/todo" || current != "1.0.0" {
		t.Fatalf("bare: %q %q %v", name, current, err)
	}
	name, _, err = hostedNpmSource(app, "pinned")
	if err != nil || name != "@tinycld/pinned" {
		t.Fatalf("versioned spec must normalize to the bare name: %q %v", name, err)
	}
	if _, _, err := hostedNpmSource(app, "gitpkg"); err == nil {
		t.Fatal("git source must refuse")
	}
	if _, _, err := hostedNpmSource(app, "none"); err == nil {
		t.Fatal("missing source must refuse")
	}
	if _, _, err := hostedNpmSource(app, "absent"); err == nil {
		t.Fatal("unknown slug must refuse")
	}
}

func TestHostedSnapshot_RestoreAndDiscard(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	orgDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(orgDir, "pb_data"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A real SQLite file at the org layout's path — the snapshot VACUUMs it
	// over its own dedicated connection.
	dbPath := filepath.Join(orgDir, "pb_data", "data.db")
	db, err := pbcore.DefaultDBConnect(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.NewQuery("CREATE TABLE t (x TEXT)").Execute(); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if err := os.WriteFile(p, []byte("live"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	restore, discard, err := hostedSnapshot(app, orgDir)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(orgDir, ".deploy", "backup.db")
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("snapshot not created: %v", err)
	}

	if err := restore(); err != nil {
		t.Fatal(err)
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("restore must drop %s — leftover WAL frames would replay over the snapshot", sidecar)
		}
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("restore must consume the snapshot")
	}

	// Discard path: fresh snapshot, then discard without touching the DB's
	// CONTENT (the snapshot connection may legitimately rewrite journal-mode
	// framing, so assert the schema survives rather than byte equality).
	_, discard, err = hostedSnapshot(app, orgDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := discard(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backup); !os.IsNotExist(err) {
		t.Fatal("discard must remove the snapshot")
	}
	check, err := pbcore.DefaultDBConnect(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var n int
	if err := check.NewQuery("SELECT count(*) FROM sqlite_master WHERE name = 't'").Row(&n); err != nil || n != 1 {
		t.Fatalf("discard must leave the live DB's schema intact (n=%d, err=%v)", n, err)
	}
}
