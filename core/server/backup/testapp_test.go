package backup

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newTestApp boots the PB fixture and adds the tinycld collections the engine
// touches. Collections mirror the migrations in core/server/pb_migrations.
//
// tests.NewTestApp clones its data dir into os.MkdirTemp("", "pb_test_*"), so
// LedgerPath would be the shared system temp root for every test app. The
// ledger override points it at this test's own directory instead, so
// backup-tmp is per-test and never outlives the run.
func newTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := newEmptyApp(t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	setLedgerPathForTesting(t, filepath.Join(t.TempDir(), "state"))

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.Fields.GetByName("role") == nil {
		users.Fields.Add(&core.SelectField{Name: "role", Values: []string{"owner", "admin", "member", "guest"}, MaxSelect: 1})
		users.Fields.Add(&core.BoolField{Name: "disabled"})
		if err := app.Save(users); err != nil {
			t.Fatal(err)
		}
	}
	mustCreate := func(c *core.Collection) {
		if err := app.Save(c); err != nil {
			t.Fatalf("create %s: %v", c.Name, err)
		}
	}
	backups := core.NewBaseCollection("backups")
	backups.Fields.Add(
		&core.SelectField{Name: "kind", Required: true, MaxSelect: 1, Values: []string{"manual", "scheduled", "pre_restore", "restore"}},
		&core.SelectField{Name: "status", Required: true, MaxSelect: 1, Values: []string{"running", "waiting_for_source", "succeeded", "failed", "interrupted"}},
		&core.RelationField{Name: "initiated_by", CollectionId: users.Id, MaxSelect: 1},
		&core.DateField{Name: "started", Required: true},
		&core.DateField{Name: "finished"},
		&core.NumberField{Name: "bytes"},
		&core.TextField{Name: "sha256", Max: 64},
		&core.JSONField{Name: "manifest", MaxSize: 200000},
		&core.TextField{Name: "target_host", Max: 253},
		&core.TextField{Name: "error", Max: 2000},
		&core.JSONField{Name: "metadata", MaxSize: 20000},
		&core.AutodateField{Name: "created", OnCreate: true},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)
	mustCreate(backups)

	reg := core.NewBaseCollection("pkg_registry")
	reg.Fields.Add(
		&core.TextField{Name: "name"}, &core.TextField{Name: "slug", Required: true},
		&core.TextField{Name: "npm_package"}, &core.TextField{Name: "version"},
		&core.SelectField{Name: "status", MaxSelect: 1, Values: []string{"bundled", "available", "installed", "disabled"}},
	)
	mustCreate(reg)
	addRegistry := func(slug, version, spec, status string) {
		r := core.NewRecord(reg)
		r.Set("name", slug)
		r.Set("slug", slug)
		r.Set("version", version)
		r.Set("npm_package", spec)
		r.Set("status", status)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
	}
	addRegistry("core", "1.2.3", "tinycld@1.2.3", "bundled")
	addRegistry("widgets", "1.0.0", "@example/widgets@1.0.0", "installed")

	notifs := core.NewBaseCollection("notifications")
	notifs.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "type"}, &core.TextField{Name: "package"}, &core.TextField{Name: "title"},
		&core.TextField{Name: "body"}, &core.TextField{Name: "url"}, &core.JSONField{Name: "metadata", MaxSize: 20000},
		&core.BoolField{Name: "read"}, &core.BoolField{Name: "dismissed"},
	)
	mustCreate(notifs)

	audit := core.NewBaseCollection("audit_logs")
	audit.Fields.Add(
		&core.TextField{Name: "action"}, &core.TextField{Name: "resource_type"}, &core.TextField{Name: "resource_id"},
		&core.TextField{Name: "resource_label"}, &core.RelationField{Name: "actor", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "ip_address"}, &core.TextField{Name: "user_agent"}, &core.JSONField{Name: "metadata", MaxSize: 20000},
	)
	mustCreate(audit)

	// A file in local storage so the walk has something to copy.
	storageDir := filepath.Join(app.DataDir(), "storage", "col1", "rec1")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storageDir, "hello.txt"), []byte("hello file"), 0o644); err != nil {
		t.Fatal(err)
	}
	return app
}

// setLedgerPathForTesting pins LedgerPath for the duration of one test.
func setLedgerPathForTesting(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	prev := ledgerPathOverride
	ledgerPathOverride = dir
	t.Cleanup(func() { ledgerPathOverride = prev })
}

func makeUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword("password12345")
	u.Set("role", role)
	u.Set("username", email[:len(email)-len("@example.com")])
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return u
}

// newEmptyApp boots a PocketBase app on an EMPTY data dir rather than the
// tests fixture. The fixture ships demo collections, demo users and 28 stored
// files, and the engine's manifest counts every non-system collection and
// every stored file — so on the fixture no test could assert an exact count.
// Bootstrapping from empty runs the same migrations a real deployment runs and
// leaves only what a test puts there.
func newEmptyApp(t *testing.T) (*tests.TestApp, error) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "empty")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	return tests.NewTestAppWithConfig(core.BaseAppConfig{
		DataDir:       dir,
		EncryptionEnv: "pb_test_env",
	})
}

// newBareApp boots the PB fixture with no tinycld collections and no ledger
// override, for the tests that assert on the real path derivation.
func newBareApp() (*tests.TestApp, error) { return tests.NewTestApp() }
