package coreserver

import (
	"net/http"
	"testing"

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
