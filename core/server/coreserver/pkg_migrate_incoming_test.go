package coreserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The defect these tests pin down:
//
// A revert always runs in the OUTGOING process — activateBuild only renames a
// symlink, and a hosted tenant runs its downs then waits to be killed. So a
// Down resolved from the process-global core.AppMigrations is the OUTGOING
// build's Down. When a release FIXES a broken down migration, the fixed copy
// can never run: the only copy in memory is the broken one. It fails SILENTLY,
// because the _migrations row is deleted either way, so the operation reports
// success while the schema change was never reverted.
//
// That is what left `comment_mentions.createRule` naming @collection.boards_cards
// after boards was uninstalled, through two separate merged fixes.

// TestRevertResolverRunsIncomingDownNotRunningOne is the regression test. The
// running binary's registry holds a broken (no-op) down; the incoming build
// ships a corrected one. The revert must run the corrected one.
func TestRevertResolverRunsIncomingDownNotRunningOne(t *testing.T) {
	app := newMigrateTestApp(t)

	const shared = "rs_shared"
	const file = "9700000000_rs_branch.js"

	sc := core.NewBaseCollection(shared)
	sc.Fields.Add(&core.TextField{Name: "target_collection"})
	if err := app.Save(sc); err != nil {
		t.Fatal(err)
	}
	rule := `target_collection = "rs_cards"`
	sc.CreateRule = &rule
	if err := app.Save(sc); err != nil {
		t.Fatal(err)
	}

	outgoingRan, incomingRan := false, false

	// What the OUTGOING build shipped: a down that misses the branch and leaves
	// it in place (boards' original prefix-anchored search).
	withTestMigrations(t, []*core.Migration{{
		File: file,
		Up:   func(core.App) error { return nil },
		Down: func(core.App) error { outgoingRan = true; return nil },
	}})
	if err := insertMigrationRow(app, file); err != nil {
		t.Fatal(err)
	}

	// What the INCOMING build ships: the corrected down.
	incoming := func(f string) (*core.Migration, bool) {
		if f != file {
			return nil, false
		}
		return &core.Migration{File: f, Down: func(txApp core.App) error {
			incomingRan = true
			c, err := txApp.FindCollectionByNameOrId(shared)
			if err != nil {
				return nil
			}
			c.CreateRule = nil
			return txApp.Save(c)
		}}, true
	}

	if _, err := revertNamedMigrations(app, []string{file}, incoming); err != nil {
		t.Fatalf("revert: %v", err)
	}

	if outgoingRan {
		t.Error("the OUTGOING build's down ran — the resolver was ignored")
	}
	if !incomingRan {
		t.Error("the INCOMING build's down did not run")
	}

	// Read the stored truth, not the cache — the whole failure mode was a rule
	// that looked handled and was not.
	var stored *string
	if err := app.DB().NewQuery("SELECT createRule FROM _collections WHERE name={:n}").
		Bind(map[string]any{"n": shared}).Row(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("createRule = %q, want NULL (the incoming down should have cleared it)", *stored)
	}
}

// A nil resolver must keep the old behavior exactly, so every caller that has
// no incoming build to read is unaffected.
func TestRevertNilResolverUsesRunningBuild(t *testing.T) {
	app := newMigrateTestApp(t)

	const colName = "rs_nilres"
	const file = "9700000001_rs_nilres.js"
	ran := false
	withTestMigrations(t, []*core.Migration{{
		File: file,
		Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(colName)) },
		Down: func(txApp core.App) error {
			ran = true
			c, err := txApp.FindCollectionByNameOrId(colName)
			if err != nil {
				return nil
			}
			return txApp.Delete(c)
		},
	}})
	if _, err := applyNamedMigrations(app, []string{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := revertNamedMigrations(app, []string{file}, nil); err != nil {
		t.Fatalf("revert: %v", err)
	}
	if !ran {
		t.Error("nil resolver must fall back to the running binary's registry")
	}
	if collectionExists(t, app, colName) {
		t.Error("collection should have been dropped")
	}
}

// The tightening: a migration with no Down cannot revert anything, so deleting
// its history row would record a revert that never happened — the schema stays
// while history says it is gone, and a later reinstall skips the Up because
// history claims it was never applied. Refuse instead of recording a lie.
//
// This is the second half of the silent-failure story: even with the right Down
// resolved, an ABSENT one used to be treated as a successful revert.
func TestRevertRefusesMigrationWithNoDown(t *testing.T) {
	app := newMigrateTestApp(t)

	const colName = "rs_nodown"
	const file = "9700000002_rs_nodown.js"
	withTestMigrations(t, []*core.Migration{{
		File: file,
		Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(colName)) },
		Down: nil,
	}})
	if _, err := applyNamedMigrations(app, []string{file}); err != nil {
		t.Fatal(err)
	}

	_, err := revertNamedMigrations(app, []string{file}, nil)
	if err == nil {
		t.Fatal("expected a revert of a migration with no down to fail, not silently succeed")
	}
	if !strings.Contains(err.Error(), "no down function") {
		t.Errorf("error should name the cause, got: %v", err)
	}

	// The refusal must leave history and schema CONSISTENT: the transaction
	// rolls back, so the row survives and matches the collection that survives.
	applied, aErr := migrationApplied(app, file)
	if aErr != nil {
		t.Fatal(aErr)
	}
	if !applied {
		t.Error("history row must survive a refused revert, or a reinstall would skip the Up")
	}
	if !collectionExists(t, app, colName) {
		t.Error("schema must survive a refused revert")
	}
}

// The dry run must refuse identically, so an unrevertable migration surfaces at
// report time rather than after the operator confirms the downgrade.
func TestDryRevertRefusesMigrationWithNoDown(t *testing.T) {
	app := newMigrateTestApp(t)

	const colName = "rs_drynodown"
	const file = "9700000003_rs_drynodown.js"
	withTestMigrations(t, []*core.Migration{{
		File: file,
		Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(colName)) },
		Down: nil,
	}})
	if _, err := applyNamedMigrations(app, []string{file}); err != nil {
		t.Fatal(err)
	}
	if _, err := dryRevertNamedMigrations(app, []string{file}, nil); err == nil {
		t.Fatal("dry revert must refuse a migration with no down")
	}
}

// syncMigrations must judge resolvability against the SAME set that will run the
// downs. A migration the INCOMING build can revert must not be skipped merely
// because the outgoing binary lacks it.
func TestSyncMigrationsJudgesResolvabilityAgainstIncomingSet(t *testing.T) {
	app := newMigrateTestApp(t)

	// Deliberately NOT registered in core.AppMigrations: only the incoming
	// build knows this file.
	const file = "9700000004_rs_incoming_only.js"
	if err := insertMigrationRow(app, file); err != nil {
		t.Fatal(err)
	}

	ran := false
	incoming := func(f string) (*core.Migration, bool) {
		if f != file {
			return nil, false
		}
		return &core.Migration{File: f, Down: func(core.App) error { ran = true; return nil }}, true
	}

	// newSet omits the file, so it is a revert candidate.
	res, err := syncMigrations(app, []string{file}, []string{"9700000005_other.js"}, incoming)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !ran {
		t.Error("the incoming build's down should have run")
	}
	if len(res.Reverted) != 1 || res.Reverted[0] != file {
		t.Errorf("Reverted = %v, want [%s]", res.Reverted, file)
	}
	if len(res.SkippedUnregistered) != 0 {
		t.Errorf("SkippedUnregistered = %v, want empty (the incoming build resolves it)", res.SkippedUnregistered)
	}
}

// LoadIncomingMigrations reads real .js files into a PRIVATE list. Loading a
// second build's migrations must NOT disturb core.AppMigrations — it has no
// remove, so a leaked registration would shadow this process's own migrations
// for the rest of its life, including any later apply.
func TestLoadIncomingMigrationsDoesNotTouchGlobalRegistry(t *testing.T) {
	dir := t.TempDir()
	const name = "9700000006_rs_loaded.js"
	src := `migrate((app) => {}, (app) => { throw new Error("incoming-down-ran") })`
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	globalBefore := len(core.AppMigrations.Items())

	incoming, err := LoadIncomingMigrations(dir, false, nil)
	if err != nil {
		t.Fatalf("LoadIncomingMigrations: %v", err)
	}
	if incoming.Len() != 1 {
		t.Errorf("loaded %d migrations, want 1", incoming.Len())
	}
	if got := len(core.AppMigrations.Items()); got != globalBefore {
		t.Errorf("core.AppMigrations grew from %d to %d — the load leaked into the global registry",
			globalBefore, got)
	}
	if _, ok := pkgMigrationByFile(name); ok {
		t.Error("the incoming migration is resolvable via the GLOBAL registry; it must be private")
	}

	// It must be resolvable through the returned resolver, and the closure it
	// hands back must be the one from the file on disk.
	m, ok := incoming.Resolve(name)
	if !ok {
		t.Fatal("resolver did not find the loaded migration")
	}
	if m.Down == nil {
		t.Fatal("loaded migration has no Down")
	}
	err = m.Down(nil)
	if err == nil || !strings.Contains(err.Error(), "incoming-down-ran") {
		t.Errorf("Down did not come from the file on disk, got err = %v", err)
	}
}

// An unreadable or migration-less directory must fail loudly. An empty incoming
// set would otherwise make the revert diff look like "the new build drops every
// migration" and trip syncMigrations' revert-everything guard far from the cause.
func TestLoadIncomingMigrationsRefusesEmptyOrMissing(t *testing.T) {
	if _, err := LoadIncomingMigrations("", false, nil); err == nil {
		t.Error("empty path must fail")
	}
	if _, err := LoadIncomingMigrations(filepath.Join(t.TempDir(), "nope"), false, nil); err == nil {
		t.Error("missing dir must fail")
	}
	if _, err := LoadIncomingMigrations(t.TempDir(), false, nil); err == nil {
		t.Error("dir with no .js migrations must fail")
	}
}

// The sandbox flag must reach the loaded runtime. A migration guarded on
// `typeof $os === 'undefined'` takes a DIFFERENT branch depending on it, so a
// loader that hardcoded one mode would make the down run under capabilities the
// up never had. Hosting loads sandboxed; self-hosted does not.
func TestLoadIncomingMigrationsHonorsSandboxFlag(t *testing.T) {
	dir := t.TempDir()
	// Reports which world it was loaded into by throwing a distinguishable error.
	src := `migrate((app) => {}, (app) => {
		throw new Error(typeof $os === 'undefined' ? 'sandboxed' : 'unsandboxed')
	})`
	const name = "9700000007_rs_sandbox.js"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		sandboxed bool
		want      string
	}{
		{true, "sandboxed"},
		{false, "unsandboxed"},
	} {
		incoming, err := LoadIncomingMigrations(dir, tc.sandboxed, nil)
		if err != nil {
			t.Fatalf("sandboxed=%v: %v", tc.sandboxed, err)
		}
		m, ok := incoming.Resolve(name)
		if !ok {
			t.Fatalf("sandboxed=%v: migration not resolved", tc.sandboxed)
		}
		err = m.Down(nil)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("sandboxed=%v: Down reported %v, want %q", tc.sandboxed, err, tc.want)
		}
	}
}

// UNINSTALL is the case the incoming-first resolver must NOT break. The
// incoming build is by definition the one WITHOUT the removed package, so its
// pb_migrations carries none of that package's files — only the outgoing build
// still has them. Resolving against the incoming set ALONE silently
// reclassifies every one of the package's migrations as unrevertable, so the
// downs never run and its collections survive an uninstall that reports
// success. That feeds purgeUnregisteredPackageRows, whose doc comment records
// this same shape as what once took tinycld.org down.
//
// Hence the fallback in resolveWith: incoming first (to pick up a corrected
// Down), outgoing second (so a departing package is revertable at all).
func TestUninstallRevertsWhenIncomingBuildDroppedThePackage(t *testing.T) {
	app := newMigrateTestApp(t)

	const colName = "un_pkg_cards"
	const pkgFile = "9700000010_un_create_pkg.js"
	const coreFile = "9700000009_un_core.js"

	ran := false
	withTestMigrations(t, []*core.Migration{
		{
			File: pkgFile,
			Up:   func(txApp core.App) error { return txApp.Save(core.NewBaseCollection(colName)) },
			Down: func(txApp core.App) error {
				ran = true
				c, err := txApp.FindCollectionByNameOrId(colName)
				if err != nil {
					return nil
				}
				return txApp.Delete(c)
			},
		},
		{File: coreFile, Up: func(core.App) error { return nil }, Down: func(core.App) error { return nil }},
	})
	if _, err := applyNamedMigrations(app, []string{coreFile, pkgFile}); err != nil {
		t.Fatal(err)
	}

	// The incoming build ships core's migration only — the uninstalled
	// package's file is not in the new artifact.
	incoming := func(f string) (*core.Migration, bool) {
		if f == coreFile {
			return &core.Migration{File: f, Down: func(core.App) error { return nil }}, true
		}
		return nil, false
	}

	res, err := syncMigrations(app, []string{coreFile, pkgFile}, []string{coreFile}, incoming)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if !ran {
		t.Error("the uninstalled package's down never ran")
	}
	if collectionExists(t, app, colName) {
		t.Error("the uninstalled package's collection survived")
	}
	if len(res.Reverted) != 1 || res.Reverted[0] != pkgFile {
		t.Errorf("Reverted = %v, want [%s]", res.Reverted, pkgFile)
	}
	if len(res.SkippedUnregistered) != 0 {
		t.Errorf("SkippedUnregistered = %v, want empty — the outgoing build can still revert it",
			res.SkippedUnregistered)
	}
}

// The fallback must not let the OUTGOING build win where the incoming one has
// an opinion: that would restore the original defect. Incoming wins whenever it
// ships the file; outgoing is consulted only when it does not.
func TestResolverPrefersIncomingOverOutgoingForSharedFiles(t *testing.T) {
	const file = "9700000011_un_shared.js"
	outgoing := false
	withTestMigrations(t, []*core.Migration{{
		File: file,
		Up:   func(core.App) error { return nil },
		Down: func(core.App) error { outgoing = true; return nil },
	}})

	incomingRan := false
	resolve := resolveWith(func(f string) (*core.Migration, bool) {
		if f != file {
			return nil, false
		}
		return &core.Migration{File: f, Down: func(core.App) error { incomingRan = true; return nil }}, true
	})

	m, ok := resolve(file)
	if !ok {
		t.Fatal("file not resolved")
	}
	if err := m.Down(nil); err != nil {
		t.Fatal(err)
	}
	if !incomingRan {
		t.Error("the INCOMING build's down must win when it ships the file")
	}
	if outgoing {
		t.Error("the outgoing build's down ran — the fallback took precedence, restoring the original bug")
	}
}
