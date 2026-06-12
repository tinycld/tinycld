package coreserver

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newAppUpdateTestApp builds a test app with the pkg_build collection and a
// single `current` build carrying one ios native bundle, then registers the
// public /api/app/* routes. It exercises the real router wiring end-to-end
// (RegisterAppUpdateEndpoints → currentBuildBundles → resolveManifest /
// serveBuildFile), which the pure-function unit tests don't cover.
func newAppUpdateTestApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newPkgBuildTestApp(t) // gives us the pkg_build collection incl. `bundles`

	col, err := app.FindCollectionByNameOrId("pkg_build")
	if err != nil {
		t.Fatalf("find pkg_build: %v", err)
	}
	rec := core.NewRecord(col)
	rec.Set("build_id", "build-200")
	rec.Set("action", "install")
	rec.Set("status", "current")
	rec.Set("bundles", []any{
		map[string]any{
			"platform":        "ios",
			"bundle_id":       "build-200-ios",
			"bundle_hash":     "deadbeef",
			"bundle_file":     "_expo/static/js/ios/index.hbc",
			"runtime_version": "1.13.7",
			"assets":          []any{},
		},
	})
	if err := app.Save(rec); err != nil {
		t.Fatalf("save current build: %v", err)
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/app")
		g.GET("/update", func(re *core.RequestEvent) error {
			platform := re.Request.URL.Query().Get("platform")
			runtime := re.Request.URL.Query().Get("runtimeVersion")
			currentID := re.Request.URL.Query().Get("currentId")
			currentHash := re.Request.URL.Query().Get("currentHash")
			if platform == "" || runtime == "" {
				return re.BadRequestError("platform and runtimeVersion are required", nil)
			}
			buildID, bundles := currentBuildBundles(app)
			if buildID == "" {
				return re.NoContent(http.StatusNoContent)
			}
			m, status := resolveManifest(bundles, platform, runtime, currentID, currentHash)
			if status != manifestNew {
				return re.NoContent(http.StatusNoContent)
			}
			fillManifestURLs(&m, buildID, platform)
			return re.JSON(http.StatusOK, m)
		})
		g.GET("/bundle/{buildId}/{platform}/{path...}", func(re *core.RequestEvent) error {
			return serveBuildFile(re)
		})
		return e.Next()
	})
	return app
}

func runAppUpdateScenario(t *testing.T, app *tests.TestApp, scenario *tests.ApiScenario) {
	t.Helper()
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

func TestAppUpdate_ServesManifestForNewerBundle(t *testing.T) {
	app := newAppUpdateTestApp(t)
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "200 manifest when the client runs an older bundle",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=1.13.7&currentId=build-100-ios",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"id":"build-200-ios"`,
			`"bundleHash":"deadbeef"`,
			`/api/app/bundle/build-200/ios/_expo/static/js/ios/index.hbc`,
		},
	})
}

func TestAppUpdate_204WhenUpToDate(t *testing.T) {
	app := newAppUpdateTestApp(t)
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "204 when the client already runs the current bundle",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=1.13.7&currentId=build-200-ios",
		ExpectedStatus: http.StatusNoContent,
	})
}

func TestAppUpdate_204OnRuntimeMismatch(t *testing.T) {
	app := newAppUpdateTestApp(t)
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "204 when no bundle matches the client runtime (App Store gate)",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=2.0.0&currentId=build-100-ios",
		ExpectedStatus: http.StatusNoContent,
	})
}

func TestAppUpdate_400WithoutRequiredParams(t *testing.T) {
	app := newAppUpdateTestApp(t)
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:            "400 when platform/runtimeVersion are missing",
		Method:          http.MethodGet,
		URL:             "/api/app/update?platform=ios",
		ExpectedStatus:  http.StatusBadRequest,
		ExpectedContent: []string{`"status":400`},
	})
}

// TestAppUpdate_BundleEndpointRejectsTraversal drives the path-traversal fix
// through the REAL router, including Go's percent-decoding of path segments — a
// `..%2f..` build id must be rejected (404) before serveBuildFile ever joins it
// into the archive path. This is the end-to-end proof of the security fix that
// the buildIDPattern unit test only covers at the regex level. Each case builds
// a FRESH app: PocketBase's BuildMux re-registers its base routes per run, so a
// shared app would panic on duplicate route registration on the second request.
func TestAppUpdate_BundleEndpointRejectsTraversal(t *testing.T) {
	cases := []struct {
		name string
		url  string
	}{
		{"encoded ../ in build id", "/api/app/bundle/..%2f..%2f..%2fetc/ios/passwd"},
		{"encoded .. as whole build id", "/api/app/bundle/%2e%2e/ios/x"},
		{"unknown platform", "/api/app/bundle/build-200/linux/x"},
		{"non-build-shaped build id", "/api/app/bundle/etc/ios/passwd"},
		// Traversal in the {path...} wildcard itself (valid build id + platform).
		// buildIDPattern/platform validation passes, so these prove os.DirFS +
		// fs.ValidPath confine the wildcard — a `..` segment must not escape
		// release/native/<platform>/ to reach build.json or the binary above it.
		{"encoded ../ in wildcard path", "/api/app/bundle/build-200/ios/..%2f..%2fbuild.json"},
		{"encoded ../ escaping to binary", "/api/app/bundle/build-200/ios/..%2f..%2f..%2ftinycld"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newAppUpdateTestApp(t)
			runAppUpdateScenario(t, app, &tests.ApiScenario{
				Name:            c.name,
				Method:          http.MethodGet,
				URL:             c.url,
				ExpectedStatus:  http.StatusNotFound,
				ExpectedContent: []string{`"status":404`},
			})
		})
	}
}
