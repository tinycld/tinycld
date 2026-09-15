package coreserver

import (
	"io/fs"
	"net/http"
	"testing"
	"testing/fstest"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// runStaticFSScenario mirrors runStaticScenario but registers the fs.FS form of
// the handler, which is what a single-binary build uses.
func runStaticFSScenario(t *testing.T, publicFs, websiteFs, releasesFs fs.FS, scenario *tests.ApiScenario) {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.RouterGroup.Any("/{path...}", StaticWithDynamicFallbackFS(publicFs, websiteFs, releasesFs, ""))
		return e.Next()
	})

	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

// TestStaticFS_ServesAssetFromEmbeddedBundle proves the handler reads real bytes
// out of the embedded FS rather than the filesystem. With no dirs on disk at
// all, a hit here can only have come from the FS.
func TestStaticFS_ServesAssetFromEmbeddedBundle(t *testing.T) {
	publicFs := fstest.MapFS{
		"sw.js":    {Data: []byte("// EMBEDDED SERVICE WORKER")},
		"app.html": {Data: []byte("<html>EMBEDDED SHELL</html>")},
	}
	runStaticFSScenario(t, publicFs, nil, nil, &tests.ApiScenario{
		Name:            "a bundled asset is served from the embedded FS",
		Method:          http.MethodGet,
		URL:             "/sw.js",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"EMBEDDED SERVICE WORKER"},
	})
}

// TestStaticFS_AppRouteFallsBackToEmbeddedShell covers the path every deep link
// into the SPA takes in a standalone binary: no such file exists, so the shell
// must come back.
func TestStaticFS_AppRouteFallsBackToEmbeddedShell(t *testing.T) {
	publicFs := fstest.MapFS{
		"app.html": {Data: []byte("<html>EMBEDDED SHELL</html>")},
	}
	runStaticFSScenario(t, publicFs, nil, nil, &tests.ApiScenario{
		Name:            "an app route falls back to the embedded SPA shell",
		Method:          http.MethodGet,
		URL:             "/mail",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"EMBEDDED SHELL"},
	})
}

// TestStaticFS_ReleasesFSWinsOverPublicShell pins the precedence when a
// releases FS is supplied: its shell is the active one.
func TestStaticFS_ReleasesFSWinsOverPublicShell(t *testing.T) {
	publicFs := fstest.MapFS{"app.html": {Data: []byte("<html>PUBLIC SHELL</html>")}}
	releasesFs := fstest.MapFS{"app.html": {Data: []byte("<html>RELEASE SHELL</html>")}}
	runStaticFSScenario(t, publicFs, nil, releasesFs, &tests.ApiScenario{
		Name:               "the releases FS shell takes precedence",
		Method:             http.MethodGet,
		URL:                "/mail",
		ExpectedStatus:     http.StatusOK,
		ExpectedContent:    []string{"RELEASE SHELL"},
		NotExpectedContent: []string{"PUBLIC SHELL"},
	})
}

// TestStaticFS_APIPathIsNotGivenTheShell guards the JSON-vs-HTML contract in
// the embedded path too: an unrouted /api/ request must 404, never receive
// "<!DOCTYPE" with a 200.
func TestStaticFS_APIPathIsNotGivenTheShell(t *testing.T) {
	publicFs := fstest.MapFS{"app.html": {Data: []byte("<html>EMBEDDED SHELL</html>")}}
	runStaticFSScenario(t, publicFs, nil, nil, &tests.ApiScenario{
		Name:               "an unrouted /api/ path 404s instead of getting the shell",
		Method:             http.MethodGet,
		URL:                "/api/nope",
		ExpectedStatus:     http.StatusNotFound,
		NotExpectedContent: []string{"EMBEDDED SHELL"},
	})
}

// TestStaticFS_UnrenamedShellBreaksEveryDeepLink pins the failure shape of the
// single-binary 404, so the handler's contract with the staging step is
// explicit: the embedded bundle must carry the shell as app.html, NOT under
// expo's exported name. Staging once copied dist verbatim, leaving index.html.
//
// The insidious part is asymmetry, reproduced here: "/" keeps working, because
// an empty path defaults to index.html and hits it as an ordinary static file.
// So the binary boots, the root URL renders, and only deep links 404 — starting
// with the setup URL the server prints on first run. See scripts/app-shell.ts.
func TestStaticFS_UnrenamedShellBreaksEveryDeepLink(t *testing.T) {
	unrenamed := fstest.MapFS{
		"index.html": {Data: []byte("<html>EMBEDDED SHELL</html>")},
	}

	runStaticFSScenario(t, unrenamed, nil, nil, &tests.ApiScenario{
		Name:            "the root URL still serves, hiding the breakage",
		Method:          http.MethodGet,
		URL:             "/",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"EMBEDDED SHELL"},
	})

	runStaticFSScenario(t, unrenamed, nil, nil, &tests.ApiScenario{
		Name:            "the first-run setup deep link 404s",
		Method:          http.MethodGet,
		URL:             "/a/setup?token=abc",
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{"wasn't found"},
	})
}
