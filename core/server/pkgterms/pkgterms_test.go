package pkgterms_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgterms"
	"tinycld.org/core/rlstest"
	"tinycld.org/core/usernames"
)

// newApp is a database with every core migration: the real pkg_registry,
// pkg_terms and pkg_terms_decisions.
func newApp(t testing.TB) *tests.TestApp {
	t.Helper()
	return rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
}

// addPackage stores the registry row of a fictional package. A blank terms
// version is a package with no terms.
func addPackage(t testing.TB, app core.App, slug, status, termsVersion, acceptedVersion string) *core.Record {
	t.Helper()
	registry, err := app.FindCollectionByNameOrId(pkgterms.RegistryCollection)
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", slug)
	pkg.Set("slug", slug)
	pkg.Set("version", "1.0.0")
	pkg.Set("status", status)
	pkg.Set(pkgterms.FieldTermsVersion, termsVersion)
	pkg.Set(pkgterms.FieldAcceptedVersion, acceptedVersion)
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	return pkg
}

// addCollection stores a collection any signed-in user may read and write,
// so a refusal in a test is the terms guard and never a rule.
func addCollection(t testing.TB, app core.App, name string, fields ...core.Field) *core.Collection {
	t.Helper()
	authed := "@request.auth.id != ''"
	col := core.NewBaseCollection(name)
	col.Fields.Add(&core.TextField{Name: "body"})
	for _, f := range fields {
		col.Fields.Add(f)
	}
	col.ListRule, col.ViewRule, col.CreateRule, col.UpdateRule, col.DeleteRule = &authed, &authed, &authed, &authed, &authed
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	return col
}

func addRecord(t testing.TB, app core.App, collection, body string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("body", body)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func addUser(t testing.TB, app core.App, email, role string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.Set("username", usernames.Derive(email))
	u.Set("name", "Test")
	u.Set("role", role)
	u.SetVerified(true)
	u.SetPassword("Password123!")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	return u
}

func tokenOf(t testing.TB, rec *core.Record) string {
	t.Helper()
	token, err := rec.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func superuserToken(t testing.TB, app core.App) string {
	t.Helper()
	col, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(col)
	su.SetEmail("root@terms.test")
	rlstest.SetPassword(su, "Password123!")
	if err := app.Save(su); err != nil {
		t.Fatal(err)
	}
	return tokenOf(t, su)
}

// call sends one request to the app's API and checks the status and the
// parts of the body. A blank token is a signed-out request.
func call(t *testing.T, app *tests.TestApp, method, url, token, body string, status int, parts ...string) {
	t.Helper()
	headers := map[string]string{"Content-Type": "application/json"}
	if token != "" {
		headers["Authorization"] = token
	}
	sc := tests.ApiScenario{
		Name: method + " " + url, Method: method, URL: url, Headers: headers,
		ExpectedStatus: status, ExpectedContent: parts,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	if body != "" {
		sc.Body = strings.NewReader(body)
	}
	if status == http.StatusNoContent {
		sc.ExpectedContent = nil
	}
	sc.Test(t)
}

func reload(t testing.TB, app core.App, rec *core.Record) *core.Record {
	t.Helper()
	fresh, err := app.FindRecordById(rec.Collection().Name, rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	return fresh
}

func TestPending(t *testing.T) {
	app := newApp(t)
	cases := []struct {
		name, slug, version, accepted string
		want                          bool
	}{
		{"a package with no terms", "plain", "", "", false},
		{"terms nobody accepted", "fresh", "2", "", true},
		{"the current version accepted", "agreed", "2", "2", false},
		{"an older version accepted", "moved", "2", "1", true},
		{"terms removed after an acceptance", "dropped", "", "1", false},
	}
	for _, c := range cases {
		pkg := addPackage(t, app, c.slug, "bundled", c.version, c.accepted)
		if got := pkgterms.Pending(pkg); got != c.want {
			t.Errorf("%s: Pending = %v, want %v", c.name, got, c.want)
		}
	}
}

func termsRow(t testing.TB, app core.App, slug string) *core.Record {
	t.Helper()
	rows, err := app.FindRecordsByFilter(pkgterms.TermsCollection, "pkg = {:pkg}", "", 0, 0, map[string]any{"pkg": slug})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) > 1 {
		t.Fatalf("%d terms rows for %s, want at most one", len(rows), slug)
	}
	if len(rows) == 0 {
		return nil
	}
	return rows[0]
}

func TestSyncStoresUpdatesAndRemovesTheText(t *testing.T) {
	app := newApp(t)

	if err := pkgterms.Sync(app, "acme", "1", "# Acme terms"); err != nil {
		t.Fatal(err)
	}
	first := termsRow(t, app, "acme")
	if first == nil || first.GetString("version") != "1" || first.GetString("body") != "# Acme terms" {
		t.Fatalf("after the first sync: %v", first)
	}

	// The same terms again write nothing: `updated` does not move at a boot.
	if err := pkgterms.Sync(app, "acme", "1", "# Acme terms"); err != nil {
		t.Fatal(err)
	}
	if same := termsRow(t, app, "acme"); same.GetString("updated") != first.GetString("updated") {
		t.Fatal("a sync of unchanged terms wrote the row")
	}

	if err := pkgterms.Sync(app, "acme", "2", "# Acme terms, second"); err != nil {
		t.Fatal(err)
	}
	second := termsRow(t, app, "acme")
	if second.Id != first.Id || second.GetString("version") != "2" || second.GetString("body") != "# Acme terms, second" {
		t.Fatalf("after a new version: %v", second)
	}

	if err := pkgterms.Sync(app, "acme", "", ""); err != nil {
		t.Fatal(err)
	}
	if gone := termsRow(t, app, "acme"); gone != nil {
		t.Fatalf("terms removed from the manifest are still stored: %v", gone)
	}
	// A package that never had terms has nothing to remove.
	if err := pkgterms.Sync(app, "plain", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestSyncRefusesAVersionWithNoText(t *testing.T) {
	app := newApp(t)
	if err := pkgterms.Sync(app, "acme", "1", ""); err == nil {
		t.Fatal("a version with no text was stored")
	}
	if row := termsRow(t, app, "acme"); row != nil {
		t.Fatalf("stored: %v", row)
	}
}

// A database with no pkg_terms collection (a bare fixture) takes a package
// with no terms and refuses one that has them.
func TestSyncWithoutTheCollection(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	if err := pkgterms.Sync(app, "plain", "", ""); err != nil {
		t.Fatalf("a package with no terms: %v", err)
	}
	if err := pkgterms.Sync(app, "acme", "1", "text"); err == nil {
		t.Fatal("terms with nowhere to store them gave no error")
	}
}
