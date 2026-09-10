package rlstest

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// NewBadBundleTestApp returns a test app with the pkg_bad_bundle collection
// created. That collection is normally made by a migration, which a bare
// tests.TestApp does not run.
//
// It lives here rather than in a _test.go file because the OTA endpoints are
// served by more than one composition, each of which needs this schema to test
// the bad-bundle skip. A second copy of the field list is a place for the two
// to silently disagree.
func NewBadBundleTestApp(t *testing.T) *tests.TestApp {
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
