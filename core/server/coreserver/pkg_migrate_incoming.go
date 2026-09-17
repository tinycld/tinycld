package coreserver

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
)

// Reading the INCOMING build's migrations.
//
// Every deploy path reverts the outgoing schema BEFORE the new binary is
// executing (see MigrationResolver for why). So the Down closures that matter
// belong to the build being deployed TO, and they have to be loaded from that
// build's own pb_migrations rather than taken from this process's
// core.AppMigrations.
//
// The load is deliberately into a PRIVATE core.MigrationsList:
// core.AppMigrations is process-global and exposes no remove or clear, so
// registering a second build's migrations into it would permanently shadow the
// ones this process booted with — for the rest of the process, including any
// later apply. A private list is discarded when the deploy goroutine returns.

// IncomingMigrations holds the migration set read out of a build that is not
// the one running. Resolve is what the revert consults for each filename.
type IncomingMigrations struct {
	list *core.MigrationsList
	dir  string
}

// Resolve returns a MigrationResolver over this set, suitable for passing to
// SyncMigrations / DryRevertNamedMigrations.
func (im *IncomingMigrations) Resolve(file string) (*core.Migration, bool) {
	for _, m := range im.list.Items() {
		if m.File == file {
			return m, true
		}
	}
	return nil, false
}

// Dir reports where the set was read from, for logging.
func (im *IncomingMigrations) Dir() string { return im.dir }

// Len reports how many migrations were loaded.
func (im *IncomingMigrations) Len() int { return len(im.list.Items()) }

// LoadIncomingMigrations evaluates the JS migrations in migrationsDir into a
// private list, so their Up/Down closures can be run without touching
// core.AppMigrations.
//
// It deliberately takes no app to REGISTER against. The loaded closures act on
// whatever txApp the migration runner passes them at call time, so the load
// itself needs no live app — and handing the live one to jsvm.Register would be
// actively wrong: Register binds hooks onto the app it is given, and attaching a
// second set to the serving process mid-deploy is what this function exists to
// avoid. (The live app still reaches the migrations, via onInit below.)
//
// sandboxed MUST match how the deployment loads its own migrations, or the
// incoming downs run under a different capability set than the ups did. A
// hosted tenant loads sandboxed (no $os/$http/$filesystem, bounded exec); a
// self-hosted deployment loads unsandboxed. Getting this wrong is a silent
// divergence: a migration guarded on `typeof $os === 'undefined'` would take
// the wrong branch, and one that uses $os unguarded would throw.
//
// onInit installs the host's own $-bindings and must be the SAME installer the
// deployment's boot registration uses — BuildJsvmOnInit(liveApp). It closes
// over the LIVE app deliberately: a binding exists to reach the real database,
// so handing it the throwaway loader app would give a Down a handle to nothing.
// Passing nil yields only the stock jsvm bindings, which is correct only where
// the boot registration also passes no OnInit.
//
// A missing or empty directory is an error rather than an empty set. An empty
// incoming set would make the revert diff look like "the new build drops every
// migration", and syncMigrations' own guard against that (refusing to revert
// everything) would then fire on what is really a bad path — so fail here,
// where the cause is still legible.
func LoadIncomingMigrations(migrationsDir string, sandboxed bool, onInit func(vm *sobek.Runtime)) (*IncomingMigrations, error) {
	if migrationsDir == "" {
		return nil, fmt.Errorf("incoming migrations: no directory given")
	}
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return nil, fmt.Errorf("incoming migrations: read %s: %w", migrationsDir, err)
	}
	var count int
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".js" {
			count++
		}
	}
	if count == 0 {
		return nil, fmt.Errorf("incoming migrations: %s contains no .js migrations", migrationsDir)
	}

	list := &core.MigrationsList{}
	// A separate app instance, never the live one: jsvm.Register binds hooks
	// (OnServe, the hooks watcher) onto whatever app it is given, and doing
	// that to the serving app mid-deploy would attach a second set of handlers
	// to a process that is about to hand over. This loader app is used only to
	// evaluate the migration files and is then dropped — it is never
	// bootstrapped and never opens a database. The Up/Down closures the load
	// produces act on the txApp the migration runner passes them at call time,
	// not on this app.
	loader := pocketbase.New()
	if err := jsvm.Register(loader, jsvm.Config{
		MigrationsDir:  migrationsDir,
		MigrationsList: list,
		Sandboxed:      sandboxed,
		OnInit:         onInit,
		// No hooks are wanted from this app at all; point it at a path that
		// carries none so the scanner finds nothing to bind.
		HooksDir: filepath.Join(migrationsDir, ".no-hooks"),
	}); err != nil {
		return nil, fmt.Errorf("incoming migrations: load %s: %w", migrationsDir, err)
	}
	if len(list.Items()) == 0 {
		return nil, fmt.Errorf("incoming migrations: %s registered no migrations", migrationsDir)
	}
	return &IncomingMigrations{list: list, dir: migrationsDir}, nil
}
