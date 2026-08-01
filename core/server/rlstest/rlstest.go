// Package rlstest runs a package's SHIPPED JavaScript migrations against a
// PocketBase test app, so a rule-level test asserts the rules the product
// actually ships rather than a copy of them.
//
// It exists because of a specific failure. Seven RLS suites across this tree
// re-declared the rule strings they were testing as constants in the test
// file, with a comment promising the constant was "copied verbatim from the
// migration". Every one of them matched byte-for-byte, so they looked like
// tripwires — and then drive's `createRule` lost its guest-exclusion clause in
// a later migration and the suite that existed to catch exactly that stayed
// green, because it was asserting against its own copy of the old rule.
//
// A test that restates the thing it is testing cannot fail for the reason you
// wrote it. Load the rule from the migration instead.
package rlstest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/tests"
)

// Apply runs every .js migration in the given directories against app, each
// directory in filename order, exactly as a real boot would.
//
// Pass more than one directory when the package under test has migrations that
// build on another package's schema — text's version migrations alter drive's
// `drive_item_versions`, for instance. Order matters: the dependency's
// directory comes first.
//
// Collections that no listed migration creates must exist on the app already.
// A bare collection carrying the fields the rules reference is enough.
func Apply(t testing.TB, app core.App, migrationsDirs ...string) {
	t.Helper()

	if len(migrationsDirs) == 0 {
		t.Fatal("rlstest: Apply needs at least one migrations directory")
	}

	// AppMigrations is a package-level list the jsvm loader appends to, so it
	// carries over between tests in one binary. Snapshot and restore it, or the
	// second suite to run re-applies the first's migrations.
	saved := core.AppMigrations
	t.Cleanup(func() { core.AppMigrations = saved })

	for _, dir := range migrationsDirs {
		abs, err := filepath.Abs(dir)
		if err != nil {
			t.Fatalf("rlstest: resolve %s: %v", dir, err)
		}
		entries, err := os.ReadDir(abs)
		if err != nil {
			t.Fatalf("rlstest: read %s: %v", abs, err)
		}
		found := false
		for _, e := range entries {
			if filepath.Ext(e.Name()) == ".js" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("rlstest: no .js migrations in %s — wrong path?", abs)
		}

		// One directory at a time: each package's migrations are ordered among
		// themselves by filename, but two packages' timestamps interleaved
		// would run them in an order no real boot produces.
		core.AppMigrations = core.MigrationsList{}

		if err := jsvm.Register(app, jsvm.Config{
			MigrationsDir: abs,
			// A hooks dir is required by the plugin but irrelevant here: rules
			// come from migrations. Point it somewhere empty so no hook file
			// runs and the test measures the rules alone.
			HooksDir: t.TempDir(),
		}); err != nil {
			t.Fatalf("rlstest: register jsvm for %s: %v", abs, err)
		}

		if err := app.RunAppMigrations(); err != nil {
			t.Fatalf("rlstest: apply migrations from %s: %v", abs, err)
		}
	}
}

// ApplyWithHooks is Apply plus the package's real pb-hooks directory, so a
// test can exercise what a hosted TENANT runs: PocketBase, the shipped
// migrations, and the package's JS hooks — but none of its Go.
//
// That combination is the one no existing suite covered. Tests that bind the
// Go hooks prove the single-tenant app is safe and say nothing about a tenant;
// tests that bind nothing miss behaviour the tenant genuinely has.
func ApplyWithHooks(t testing.TB, app core.App, hooksDir string, migrationsDirs ...string) {
	t.Helper()

	abs, err := filepath.Abs(hooksDir)
	if err != nil {
		t.Fatalf("rlstest: resolve %s: %v", hooksDir, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("rlstest: hooks dir %s: %v", abs, err)
	}

	saved := core.AppMigrations
	t.Cleanup(func() { core.AppMigrations = saved })

	for _, dir := range migrationsDirs {
		mabs, err := filepath.Abs(dir)
		if err != nil {
			t.Fatalf("rlstest: resolve %s: %v", dir, err)
		}
		core.AppMigrations = core.MigrationsList{}
		if err := jsvm.Register(app, jsvm.Config{MigrationsDir: mabs, HooksDir: t.TempDir()}); err != nil {
			t.Fatalf("rlstest: register jsvm for %s: %v", mabs, err)
		}
		if err := app.RunAppMigrations(); err != nil {
			t.Fatalf("rlstest: apply migrations from %s: %v", mabs, err)
		}
	}

	// The hooks pass registers the real hook files. It comes last so the
	// collections they reference already exist.
	core.AppMigrations = core.MigrationsList{}
	if err := jsvm.Register(app, jsvm.Config{
		MigrationsDir: t.TempDir(),
		HooksDir:      abs,
		HooksPoolSize: 1,
	}); err != nil {
		t.Fatalf("rlstest: register hooks from %s: %v", abs, err)
	}
}

// Rule returns one of a collection's five access rules after the migrations
// have been applied, so a test can assert behaviour against the shipped rule
// and report the rule text when it fails.
//
// A nil rule (superusers only) is returned as ("", false) — distinct from a
// rule that is the empty string, which PocketBase reads as "public".
func Rule(t testing.TB, app core.App, collection, kind string) (string, bool) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatalf("rlstest: find %s: %v", collection, err)
	}
	var r *string
	switch kind {
	case "list":
		r = col.ListRule
	case "view":
		r = col.ViewRule
	case "create":
		r = col.CreateRule
	case "update":
		r = col.UpdateRule
	case "delete":
		r = col.DeleteRule
	default:
		t.Fatalf("rlstest: unknown rule kind %q", kind)
	}
	if r == nil {
		return "", false
	}
	return *r, true
}

// RequireRuleContains asserts a shipped rule carries a given clause.
//
// This is the guard against silent clause loss: a later migration that
// restates a rule and drops one predicate — which is how drive's guest
// exclusion disappeared — fails here naming the clause and printing the rule
// as it now ships.
//
// It is a complement to behavioural deny-tests, not a substitute. Assert the
// behaviour; use this to say which clause is meant to produce it.
func RequireRuleContains(t testing.TB, app core.App, collection, kind, clause string) {
	t.Helper()
	rule, ok := Rule(t, app, collection, kind)
	if !ok {
		t.Fatalf("%s.%sRule is nil (superusers only); expected it to contain %q",
			collection, kind, clause)
	}
	if !strings.Contains(rule, clause) {
		t.Fatalf("%s.%sRule lost the clause %q\n  shipped rule: %s",
			collection, kind, clause, rule)
	}
}

// NewApp builds a TestApp for rule testing. Callers create the collections
// their migrations expect, then call Apply.
func NewApp(t testing.TB) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("rlstest: NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })
	return app
}

// MigrationsDir resolves a sibling package's pb-migrations directory from a
// test running inside that package's server/ directory.
func MigrationsDir(t testing.TB, relative string) string {
	t.Helper()
	abs, err := filepath.Abs(relative)
	if err != nil {
		t.Fatalf("rlstest: resolve %s: %v", relative, err)
	}
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("rlstest: %s: %v", abs, err)
	}
	return abs
}
