//go:build unix

package coreserver

import (
	"context"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
)

// stubRestoreRebuildDeps makes the restore's rebuilder run with deps in
// place of the production pipeline, which needs a toolchain.
func stubRestoreRebuildDeps(t *testing.T, deps rebuildDeps) {
	t.Helper()
	prev := restoreRebuildDeps
	restoreRebuildDeps = func(*pocketbase.PocketBase, *installjob.Job, RebuildManifest, *core.Record) rebuildDeps { return deps }
	t.Cleanup(func() { restoreRebuildDeps = prev })
}

// restoreDeps is a rebuild that succeeds, except that a migration sync
// fails: the restore must replace it, because the live database is on its
// way out and the staged one already has its packages' schema.
func restoreDeps(restarted *bool) rebuildDeps {
	var restored bool
	deps := happyDeps(&restored, restarted)
	deps.syncMig = func(string) (SyncResult, error) {
		return SyncResult{}, errors.New("the restore migrated the database it is replacing")
	}
	return deps
}

func runRestoreRebuilder(t *testing.T, app *pocketbase.PocketBase) error {
	t.Helper()
	job := installjob.New("restore", "", "")
	err := restoreRebuilder(app)(context.Background(), job, format.Lockfile{"tinycld": "1.0.0"})
	select {
	case <-job.Done:
	default:
		t.Fatal("the restore's rebuilder returned without finishing its job")
	}
	return err
}

// Under a supervisor the restart is asynchronous: the rebuilder returns
// ErrRestartUnderway, which the restore reads as success. The restart is
// cold, because the next process's boot sets this pb_data aside.
func TestSupervisedRestoreRebuildRestartsColdAndIsUnderway(t *testing.T) {
	_, parent := supervisedFixture(t)
	notDevelopment(t)
	exits := recordExit(t)
	app := bootstrappedApp(t)
	registerSupervised(app)
	lines := ackRestarts(t, parent)
	var restarted bool
	stubRestoreRebuildDeps(t, restoreDeps(&restarted))

	err := runRestoreRebuilder(t, app)
	if !errors.Is(err, backup.ErrRestartUnderway) {
		t.Fatalf("rebuilder err = %v, want ErrRestartUnderway", err)
	}
	if got := nextLine(t, lines); got != `{"type":"restart","cold":true,"version":1}` {
		t.Fatalf("control message = %q", got)
	}
	if restarted {
		t.Fatal("the rebuild's own restart ran in place of the restore's cold one")
	}
	if len(*exits) != 0 {
		t.Fatalf("exited %v under a supervisor", *exits)
	}
}

// A restart that is not under way (dev mode: nothing relaunches the
// process) must not be reported as one, or the restore would wait behind
// its maintenance 503 for a restart that never comes.
func TestRestoreRebuildWithoutARestartIsNotUnderway(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	t.Cleanup(resetReplacementForTest)
	exits := recordExit(t)
	app := bootstrappedApp(t)
	var restarted bool
	stubRestoreRebuildDeps(t, restoreDeps(&restarted))

	if err := runRestoreRebuilder(t, app); err != nil {
		t.Fatalf("rebuilder err = %v, want nil", err)
	}
	if len(*exits) != 0 {
		t.Fatalf("exited %v in dev mode", *exits)
	}
}

// The restore's rebuild tags its install-log row with its build, so a
// rollback of the restore's build marks the row the restore finalized
// "success" before its restart.
func TestRestoreRebuildTagsItsInstallLogRow(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", t.TempDir())
	t.Cleanup(resetReplacementForTest)
	recordExit(t)
	app := bootstrappedApp(t)
	addInstallLogCollection(t, app)
	prev := restoreRebuildDeps
	var buildID string
	restoreRebuildDeps = func(a *pocketbase.PocketBase, j *installjob.Job, m RebuildManifest, r *core.Record) rebuildDeps {
		buildID = m.BuildID
		return withStubbedSteps(prev(a, j, m, r))
	}
	t.Cleanup(func() { restoreRebuildDeps = prev })

	if err := runRestoreRebuilder(t, app); err != nil {
		t.Fatalf("rebuilder err = %v, want nil", err)
	}

	rows, err := app.FindAllRecords("pkg_install_log")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("%d install-log rows, want 1", len(rows))
	}
	if got := rows[0].GetString("build_id"); buildID == "" || got != buildID {
		t.Fatalf("build_id = %q, want the restore's build %q", got, buildID)
	}
	if got := rows[0].GetString("status"); got != "success" {
		t.Fatalf("status = %q, want success", got)
	}
}
