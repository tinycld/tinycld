package coreserver

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
	"tinycld.org/core/rlstest"
)

// The admin console's collections are granted by ORG ROLE. The `super_admins`
// junction that used to carry that authority is gone, along with the three
// migrations that created it and OR-ed its clause into every console
// collection (1910000005/07/11).
//
// These apply the REAL pb_migrations rather than restating rules, so a later
// migration that reintroduces a super_admins clause — or drops the role
// grants — turns this red instead of leaving a stale copy passing.
// (fresh_provision_guard_test.go has its own applyCoreMigrations with a
// different contract: it RETURNS the error so the refusal path is assertable.)
func adminConsoleTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })

	// Same fixture reconciliation as guest_rls_test / org_pkg_access_rls_test:
	// PB's bundled users fixture ships its own username index, which collides
	// with the one 1820000000 creates. Drop it so the migration applies the way
	// it does against a real (fixture-free) DB.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	var kept types.JSONArray[string]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))
	return app
}

// The collection must be gone. If a migration recreates it, the guards in
// pkg_install.go no longer consult it and the grant would be silently inert —
// worse than absent, because the UI would suggest an authority that does
// nothing.
func TestSuperAdminsCollectionIsGone(t *testing.T) {
	app := adminConsoleTestApp(t)

	if _, err := app.FindCollectionByNameOrId("super_admins"); err == nil {
		t.Fatal("super_admins collection still exists after migrations — " +
			"role is the only privilege axis now")
	}
}

// No surviving rule may reference the dropped table. A `@collection.super_admins`
// clause against a non-existent collection is not a no-op — it makes the whole
// rule fail to evaluate, which would lock out the very admins it was meant to
// admit.
func TestNoRuleReferencesSuperAdmins(t *testing.T) {
	app := adminConsoleTestApp(t)

	collections, err := app.FindAllCollections()
	if err != nil {
		t.Fatal(err)
	}

	for _, c := range collections {
		rules := map[string]*string{
			"list":   c.ListRule,
			"view":   c.ViewRule,
			"create": c.CreateRule,
			"update": c.UpdateRule,
			"delete": c.DeleteRule,
		}
		if c.IsAuth() {
			rules["manage"] = c.ManageRule
		}
		for label, r := range rules {
			if r != nil && strings.Contains(*r, "super_admins") {
				t.Errorf("%s.%sRule still references super_admins: %s", c.Name, label, *r)
			}
		}
	}
}

// Package writes are OWNER-only while console reads are admin-wide. This is
// the split the whole change turns on, so it's asserted on the shipped rules
// rather than only on the Go guards.
func TestAdminConsoleRulesGrantByRole(t *testing.T) {
	app := adminConsoleTestApp(t)

	cases := []struct {
		collection string
		rule       func(*core.Collection) *string
		label      string
		wantOwner  bool // owner-only (admins excluded)
	}{
		{"pkg_registry", func(c *core.Collection) *string { return c.CreateRule }, "create", true},
		{"pkg_registry", func(c *core.Collection) *string { return c.UpdateRule }, "update", true},
		{"pkg_registry", func(c *core.Collection) *string { return c.DeleteRule }, "delete", true},
		{"pkg_build", func(c *core.Collection) *string { return c.DeleteRule }, "delete", true},
		{"pkg_build", func(c *core.Collection) *string { return c.ListRule }, "list", false},
		{"pkg_build", func(c *core.Collection) *string { return c.ViewRule }, "view", false},
		{"system_settings", func(c *core.Collection) *string { return c.ListRule }, "list", false},
		{"system_settings", func(c *core.Collection) *string { return c.UpdateRule }, "update", false},
	}

	for _, tc := range cases {
		c, err := app.FindCollectionByNameOrId(tc.collection)
		if err != nil {
			t.Errorf("collection %s missing: %v", tc.collection, err)
			continue
		}
		r := tc.rule(c)
		if r == nil {
			t.Errorf("%s.%sRule is nil (superuser-only) — the console can't reach it",
				tc.collection, tc.label)
			continue
		}
		mentionsAdmin := strings.Contains(*r, `role = "admin"`)
		if tc.wantOwner && mentionsAdmin {
			t.Errorf("%s.%sRule admits admins but must be owner-only: %s",
				tc.collection, tc.label, *r)
		}
		if !tc.wantOwner && !mentionsAdmin {
			t.Errorf("%s.%sRule must admit admins (console read surface): %s",
				tc.collection, tc.label, *r)
		}
		if !strings.Contains(*r, `role = "owner"`) {
			t.Errorf("%s.%sRule must admit the owner: %s", tc.collection, tc.label, *r)
		}
	}
}
