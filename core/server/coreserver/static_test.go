package coreserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// staticDirs builds a publicDir (app's own web static), a websiteDir (marketing
// site), and a releasesDir with current/app.html (the SPA shell), each populated
// from the given name→content maps. Any map may be nil/empty. Returns the three
// dir paths; an empty websiteDir map yields "" (website disabled).
func staticDirs(t *testing.T, public, website map[string]string, appHTML string) (publicDir, websiteDir, releasesDir string) {
	t.Helper()
	write := func(dir string, files map[string]string) {
		for name, content := range files {
			p := filepath.Join(dir, name)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	publicDir = t.TempDir()
	write(publicDir, public)

	if len(website) > 0 {
		websiteDir = t.TempDir()
		write(websiteDir, website)
	}

	releasesDir = t.TempDir()
	if appHTML != "" {
		cur := filepath.Join(releasesDir, "current")
		if err := os.MkdirAll(cur, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(cur, "app.html"), []byte(appHTML), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return publicDir, websiteDir, releasesDir
}

func runStaticScenario(t *testing.T, publicDir, websiteDir, releasesDir string, scenario *tests.ApiScenario) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.RouterGroup.Any("/{path...}", StaticWithDynamicFallback(publicDir, websiteDir, releasesDir))
		return e.Next()
	})

	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

// TestStatic_AppRouteServesShellNotWebsite is the regression guard for the bug
// this fix targets: with a marketing site present, an app route (not in website/
// or public/) must fall through to the SPA shell — NOT render the website
// homepage. Before the website lived in its own dir, an in-app rebuild's expo
// export absorbed it into the bundle and every app route served the marketing
// page.
func TestStatic_AppRouteServesShellNotWebsite(t *testing.T) {
	publicDir, websiteDir, releasesDir := staticDirs(t,
		map[string]string{"sw.js": "// service worker"},
		map[string]string{"index.html": "<html>MARKETING HOME</html>"},
		"<html>APP SHELL</html>",
	)
	runStaticScenario(t, publicDir, websiteDir, releasesDir, &tests.ApiScenario{
		Name:               "app route falls back to the SPA shell, not the website",
		Method:             http.MethodGet,
		URL:                "/a/acme/mail",
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"APP SHELL"},
		NotExpectedContent: []string{"MARKETING HOME"},
	})
}

func TestStatic_RootServesWebsiteHome(t *testing.T) {
	publicDir, websiteDir, releasesDir := staticDirs(t,
		map[string]string{"sw.js": "// sw"},
		map[string]string{"index.html": "<html>MARKETING HOME</html>"},
		"<html>APP SHELL</html>",
	)
	runStaticScenario(t, publicDir, websiteDir, releasesDir, &tests.ApiScenario{
		Name:            "/ serves the marketing homepage",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"MARKETING HOME"},
	})
}

func TestStatic_WebsiteSubpageServesFromWebsiteDir(t *testing.T) {
	publicDir, websiteDir, releasesDir := staticDirs(t,
		nil,
		map[string]string{"docs/index.html": "<html>DOCS PAGE</html>"},
		"<html>APP SHELL</html>",
	)
	runStaticScenario(t, publicDir, websiteDir, releasesDir, &tests.ApiScenario{
		Name:            "/docs resolves to website/docs/index.html",
		Method:          http.MethodGet,
		URL:             "/docs",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"DOCS PAGE"},
	})
}

func TestStatic_AppOwnPublicFileStillServed(t *testing.T) {
	publicDir, websiteDir, releasesDir := staticDirs(t,
		map[string]string{"sw.js": "SERVICE_WORKER_BODY"},
		map[string]string{"index.html": "<html>HOME</html>"},
		"<html>APP SHELL</html>",
	)
	runStaticScenario(t, publicDir, websiteDir, releasesDir, &tests.ApiScenario{
		Name:            "the app's own public/ file (sw.js) is served from publicDir",
		Method:          http.MethodGet,
		URL:             "/sw.js",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"SERVICE_WORKER_BODY"},
	})
}

// TestStatic_NoWebsiteDirStillServesApp confirms dev / com mode (websiteDir
// empty): / has no website index, so it falls through to the SPA shell.
func TestStatic_NoWebsiteDirStillServesApp(t *testing.T) {
	publicDir, websiteDir, releasesDir := staticDirs(t,
		map[string]string{"sw.js": "// sw"},
		nil, // no website
		"<html>APP SHELL</html>",
	)
	if websiteDir != "" {
		t.Fatalf("expected empty websiteDir, got %q", websiteDir)
	}
	runStaticScenario(t, publicDir, websiteDir, releasesDir, &tests.ApiScenario{
		Name:            "no website: / serves the SPA shell",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"APP SHELL"},
	})
}

func TestDefaultWebsiteDir_StateDirSet(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", "/workspace")
	if got, want := DefaultWebsiteDir(), filepath.Join("/workspace", "website"); got != want {
		t.Fatalf("DefaultWebsiteDir() = %q, want %q", got, want)
	}
}

func TestDefaultWebsiteDir_StateDirUnsetFallsBackToRelative(t *testing.T) {
	t.Setenv("TINYCLD_STATE_DIR", "")
	if got, want := DefaultWebsiteDir(), "./website"; got != want {
		t.Fatalf("DefaultWebsiteDir() = %q, want %q", got, want)
	}
}

func TestStatic_ApiPathNeverServesShell(t *testing.T) {
	publicDir, websiteDir, releasesDir := staticDirs(t,
		nil,
		map[string]string{"index.html": "<html>HOME</html>"},
		"<html>APP SHELL</html>",
	)
	runStaticScenario(t, publicDir, websiteDir, releasesDir, &tests.ApiScenario{
		Name:               "/api/* miss returns JSON 404, never the HTML shell",
		Method:             http.MethodGet,
		URL:                "/api/not-yet-registered",
		ExpectedStatus:     http.StatusNotFound,
		ExpectedContent:    []string{`"status":404`},
		NotExpectedContent: []string{"APP SHELL", "HOME"},
	})
}
