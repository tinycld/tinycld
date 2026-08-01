package coreserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// A tenant serves its org's web bundle itself — the router only
// reverse-proxies — from the materialized <orgDir>/pb_public (the artifact's
// staged release). These tests pin the three behaviors the hosted browser
// suite depends on: static files serve, SPA routes fall back to app.html, and
// /api/ misses stay JSON 404s (never the HTML shell).

// tenantPublicDir builds a pb_public shaped like a staged release: app.html
// (StageRelease renames index.html), a public file, and a content-hashed
// expo asset.
func tenantPublicDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"app.html":                         "<html>ORG APP SHELL</html>",
		"sw.js":                            "// service worker",
		"_expo/static/js/web/index-abc.js": "console.log('bundle')",
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func runTenantStaticScenario(t *testing.T, publicDir string, scenario *tests.ApiScenario) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	registerTenantStaticServe(app, publicDir)

	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

func TestTenantStatic_RootServesAppShell(t *testing.T) {
	runTenantStaticScenario(t, tenantPublicDir(t), &tests.ApiScenario{
		Name:            "root falls back to the org's app.html",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"ORG APP SHELL"},
	})
}

func TestTenantStatic_SpaRouteServesAppShell(t *testing.T) {
	runTenantStaticScenario(t, tenantPublicDir(t), &tests.ApiScenario{
		Name:            "an SPA route (no such file) falls back to app.html",
		Method:          http.MethodGet,
		URL:             "/admin",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"ORG APP SHELL"},
	})
}

func TestTenantStatic_StaticFilesServe(t *testing.T) {
	runTenantStaticScenario(t, tenantPublicDir(t), &tests.ApiScenario{
		Name:            "a real file in pb_public serves as-is",
		Method:          http.MethodGet,
		URL:             "/_expo/static/js/web/index-abc.js",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"console.log('bundle')"},
	})
}

func TestTenantStatic_ApiMissNeverServesShell(t *testing.T) {
	// The same guard the host catch-all has: during boot windows an /api/ path
	// can reach the catch-all; answering with the HTML shell hands JSON
	// clients "<!DOCTYPE …" with a 200. Must be a JSON 404 instead.
	runTenantStaticScenario(t, tenantPublicDir(t), &tests.ApiScenario{
		Name:               "an unknown /api/ path 404s as JSON, not the shell",
		Method:             http.MethodGet,
		URL:                "/api/definitely-not-a-route",
		ExpectedStatus:     http.StatusNotFound,
		ExpectedContent:    []string{`"data"`},
		NotExpectedContent: []string{"ORG APP SHELL"},
	})
}
