package coreserver

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

// fresh_provision_guard_test.go proves the fresh-provisioning decision is
// ENFORCED, not just documented.
//
// The multi-org transition dropped the org-scoped schema (`orgs`, `user_org`)
// by editing already-shipped migrations in place, with no data-conversion
// migration. That is coherent only because every database is provisioned
// fresh: PocketBase never re-runs an applied migration, so a legacy database
// would keep `user_org` ids in its FK columns while the rewritten rules
// (`user ?= @request.auth.id`) silently never match — every user locked out
// of their own data with no error anywhere. The guard migration turns that
// silent lockout into a loud, named boot failure.

// applyCoreMigrations mirrors rlstest.Apply but returns the migration error
// instead of failing the test, so the refusal path can be asserted.
func applyCoreMigrations(t *testing.T, app core.App) error {
	t.Helper()

	saved := core.AppMigrations
	t.Cleanup(func() { core.AppMigrations = saved })
	core.AppMigrations = core.MigrationsList{}

	if err := jsvm.Register(app, jsvm.Config{
		MigrationsDir: rlstest.MigrationsDir(t, "../pb_migrations"),
		HooksDir:      t.TempDir(),
	}); err != nil {
		t.Fatalf("register jsvm: %v", err)
	}
	return app.RunAppMigrations()
}

func TestFreshProvisionGuard_RefusesLegacyDatabase(t *testing.T) {
	for _, legacy := range []string{"user_org", "orgs"} {
		t.Run(legacy, func(t *testing.T) {
			app, err := tests.NewTestApp()
			if err != nil {
				t.Fatalf("NewTestApp: %v", err)
			}
			t.Cleanup(func() { app.Cleanup() })

			// Plant a bare legacy collection, as any database created by a
			// pre-multi-org build would carry.
			if err := app.Save(core.NewBaseCollection(legacy)); err != nil {
				t.Fatalf("plant legacy collection %s: %v", legacy, err)
			}

			err = applyCoreMigrations(t, app)
			if err == nil {
				t.Fatalf("core migrations applied cleanly against a database containing legacy collection %q; expected the fresh-provisioning guard to refuse", legacy)
			}
			if !strings.Contains(err.Error(), legacy) || !strings.Contains(err.Error(), "pre-multi-org") {
				t.Fatalf("guard refusal should name the legacy collection %q and say the database is pre-multi-org; got: %v", legacy, err)
			}
		})
	}
}

// The guard is only loud on a legacy database if it runs before anything else
// gets a chance to half-apply against the old schema, which depends on its
// filename sorting first. Assert that ordering so a renamed guard or an
// earlier-timestamped newcomer turns this red instead of silently demoting
// the guard to "runs eventually".
func TestFreshProvisionGuard_SortsFirst(t *testing.T) {
	dir := rlstest.MigrationsDir(t, "../pb_migrations")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".js" {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		t.Fatal("no migrations found")
	}
	if !strings.Contains(names[0], "refuse_legacy_org_database") {
		t.Fatalf("the fresh-provisioning guard must be the first migration to run on a legacy database; first by filename is %q", names[0])
	}
}
