package coreserver

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newBadBundleTestApp builds a test app with just the pkg_bad_bundle collection
// (mirrors the 1910000009 migration) so recordBadBundle / loadBadBundles can be
// exercised without running migrations.
func newBadBundleTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(func() { app.Cleanup() })

	c := core.NewBaseCollection("pkg_bad_bundle")
	c.Fields.Add(&core.TextField{Name: "bundle_id", Required: true})
	c.Fields.Add(&core.TextField{Name: "bundle_hash"})
	c.Fields.Add(&core.SelectField{
		Name: "platform", Required: true, MaxSelect: 1,
		Values: []string{"ios", "android"},
	})
	c.Fields.Add(&core.NumberField{Name: "reports", Required: true})
	c.Fields.Add(&core.TextField{Name: "last_error"})
	c.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	c.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(c); err != nil {
		t.Fatalf("save pkg_bad_bundle collection: %v", err)
	}
	return app
}

func TestRecordBadBundle_InsertThenIncrement(t *testing.T) {
	app := newBadBundleTestApp(t)
	body := reportBadBody{ID: "build-200-ios", Hash: "HASH", Platform: "ios", Error: "boom"}

	count, err := recordBadBundle(app, body)
	if err != nil {
		t.Fatalf("first report: %v", err)
	}
	if count != 1 {
		t.Fatalf("first report count = %d, want 1", count)
	}

	// A second report for the same bundle increments rather than duplicating.
	count, err = recordBadBundle(app, body)
	if err != nil {
		t.Fatalf("second report: %v", err)
	}
	if count != 2 {
		t.Fatalf("second report count = %d, want 2", count)
	}

	recs, err := app.FindRecordsByFilter("pkg_bad_bundle", "bundle_id = 'build-200-ios'", "", 0, 0)
	if err != nil || len(recs) != 1 {
		t.Fatalf("expected exactly 1 row, got %d (err %v)", len(recs), err)
	}
}

// loadBadBundles must surface both the id and hash of every reported bundle so
// resolveManifest can match on either.
func TestLoadBadBundles(t *testing.T) {
	app := newBadBundleTestApp(t)
	if _, err := recordBadBundle(app, reportBadBody{ID: "build-200-ios", Hash: "HASH", Platform: "ios"}); err != nil {
		t.Fatal(err)
	}

	ids, hashes := loadBadBundles(app)
	if !ids["build-200-ios"] {
		t.Errorf("ids missing build-200-ios: %v", ids)
	}
	if !hashes["HASH"] {
		t.Errorf("hashes missing HASH: %v", hashes)
	}
}

// loadBadBundles must fail OPEN (empty sets, no error) when the collection is
// absent — a reporting-table hiccup must never block all updates.
func TestLoadBadBundles_MissingCollectionFailsOpen(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	ids, hashes := loadBadBundles(app)
	if len(ids) != 0 || len(hashes) != 0 {
		t.Fatalf("expected empty sets on missing collection, got ids=%v hashes=%v", ids, hashes)
	}
}
