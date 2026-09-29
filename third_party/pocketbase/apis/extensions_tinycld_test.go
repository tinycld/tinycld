package apis_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// A test that reuses one app across several ApiScenarios builds a new router
// per request. Each build must not add another OnServe handler for the UI
// routes, or the second request panics on a duplicate "GET /_/extensions.js".
func TestUIExtensions_ReusedAppServesMoreThanOnce(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	scenario := tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/_/extensions.js",
		TestAppFactory:        func(t testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		ExpectedStatus:        200,
		ExpectedContent:       []string{},
		ExpectedEvents:        map[string]int{"*": 0},
	}

	for _, name := range []string{"first request", "second request", "third request"} {
		scenario.Name = name
		scenario.Test(t)
	}
}
