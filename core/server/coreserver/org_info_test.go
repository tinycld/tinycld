package coreserver

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// /api/org-info is the client's one branding source (useOrgInfo → document
// title, org avatar). It must answer unauthenticated with the settings
// AppName and nothing else from settings.
func TestOrgInfo_ServesAppNameUnauthenticated(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	app.Settings().Meta.AppName = "Acme Incorporated"
	RegisterOrgInfoEndpoint(app)

	scenario := &tests.ApiScenario{
		Name:            "unauthenticated org-info",
		Method:          http.MethodGet,
		URL:             "/api/org-info",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"name":"Acme Incorporated"`},
		TestAppFactory:  func(_ testing.TB) *tests.TestApp { return app },
	}
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

// TestOrgInfoReturnsEmptyLogoWhenUnset covers the state of any DB that has
// never run the Task 8 migration: the org_branding collection is entirely
// absent. A deployment that has never uploaded a logo must still answer
// cleanly — the client renders name-initials from the same response, and a
// pre-login 500 here would break the login screen itself.
func TestOrgInfoReturnsEmptyLogoWhenUnset(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	RegisterOrgInfoEndpoint(app)

	scenario := &tests.ApiScenario{
		Name:            "no org_branding collection at all",
		Method:          http.MethodGet,
		URL:             "/api/org-info",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"logoUrl":""`, `"logoCrop":""`},
		TestAppFactory:  func(_ testing.TB) *tests.TestApp { return app },
	}
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

// TestOrgInfoServesUploadedLogo covers the collection existing (mirroring the
// create_org_branding migration's fields) with a branding record set: the URL
// must reference the stored filename, and the crop must be served alongside
// it verbatim.
func TestOrgInfoServesUploadedLogo(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	col := core.NewBaseCollection("org_branding")
	col.Fields.Add(&core.FileField{Name: "logo", MaxSelect: 1})
	col.Fields.Add(&core.TextField{Name: "logo_crop"})
	if err := app.Save(col); err != nil {
		t.Fatalf("save org_branding collection: %v", err)
	}

	// SaveNoValidate: the file field's validator requires an actual multipart
	// upload (and PB's normalizeName randomizes whatever filename it's given),
	// so a plain stored filename can't round-trip through app.Save the way the
	// real upload endpoint writes it. Seeding the DB row directly matches how
	// FindFirstRecordByFilter will see it in production, without fighting the
	// upload plumbing this test isn't exercising.
	record := core.NewRecord(col)
	record.Set("logo", "logo_abc.png")
	record.Set("logo_crop", `{"x":0.5,"y":0.5,"zoom":1}`)
	if err := app.SaveNoValidate(record); err != nil {
		t.Fatalf("save org_branding record: %v", err)
	}

	RegisterOrgInfoEndpoint(app)

	scenario := &tests.ApiScenario{
		Name:           "logo set",
		Method:         http.MethodGet,
		URL:            "/api/org-info",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			"logo_abc.png",
			`"logoCrop":"{\"x\":0.5,\"y\":0.5,\"zoom\":1}"`,
		},
		TestAppFactory: func(_ testing.TB) *tests.TestApp { return app },
	}
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}
