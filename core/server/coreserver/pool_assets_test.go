package coreserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupPool creates a releasesDir with a _static/<prefix>/<file> entry
// containing the given content, mirroring what the entrypoint produces.
func setupPool(t *testing.T, prefix, file, content string) string {
	t.Helper()
	releasesDir := t.TempDir()
	dir := filepath.Join(releasesDir, "_static", prefix)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return releasesDir
}

func runPoolScenario(t *testing.T, releasesDir, prefix, cacheControl string, scenario *tests.ApiScenario) {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.RouterGroup.GET("/"+prefix+"/{path...}", PoolAssets(releasesDir, prefix, cacheControl))
		return e.Next()
	})

	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

// The bundle policy the server registers for /_expo/static/. Kept in one
// place here so the tests below can't drift from the value server.go passes.
const bundleCacheControl = "public, no-cache"

func TestPoolAssets_ServesBundleWithRevalidateCache(t *testing.T) {
	releasesDir := setupPool(t, "_expo/static/js/web", "bundle-abc.js", "console.log('hi')")

	runPoolScenario(t, releasesDir, "_expo/static", bundleCacheControl, &tests.ApiScenario{
		Name:            "serves bundle from pool with a revalidating policy and a validator",
		Method:          http.MethodGet,
		URL:             "/_expo/static/js/web/bundle-abc.js",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"console.log"},
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if cc := res.Header.Get("Cache-Control"); cc != bundleCacheControl {
				t.Errorf("Cache-Control = %q, want %q", cc, bundleCacheControl)
			}
			if res.Header.Get("Last-Modified") == "" {
				t.Errorf("Last-Modified missing; no-cache without a validator refetches every load")
			}
		},
	})
}

// TestPoolAssets_ConditionalGetReturns304 is the payoff of no-cache over
// no-store: a bundle the client already holds costs one header round-trip.
func TestPoolAssets_ConditionalGetReturns304(t *testing.T) {
	releasesDir := setupPool(t, "_expo/static/js/web", "bundle-abc.js", "console.log('hi')")
	modTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(releasesDir, "_static", "_expo/static/js/web", "bundle-abc.js"), modTime, modTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	runPoolScenario(t, releasesDir, "_expo/static", bundleCacheControl, &tests.ApiScenario{
		Name:           "matching If-Modified-Since yields 304",
		Method:         http.MethodGet,
		URL:            "/_expo/static/js/web/bundle-abc.js",
		Headers:        map[string]string{"If-Modified-Since": modTime.Format(http.TimeFormat)},
		ExpectedStatus: http.StatusNotModified,
	})
}

// TestPoolAssets_NewerFileUnderSameNameIsResent covers the collision the pool
// actually sees: a later release wrote different bytes under an unchanged
// filename. A client validating against the old copy must get the new body.
func TestPoolAssets_NewerFileUnderSameNameIsResent(t *testing.T) {
	releasesDir := setupPool(t, "_expo/static/js/web", "index-abc.js", "chunks: new")
	newTime := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(filepath.Join(releasesDir, "_static", "_expo/static/js/web", "index-abc.js"), newTime, newTime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	oldTime := newTime.Add(-24 * time.Hour)

	runPoolScenario(t, releasesDir, "_expo/static", bundleCacheControl, &tests.ApiScenario{
		Name:            "older If-Modified-Since re-sends the newer same-named bundle",
		Method:          http.MethodGet,
		URL:             "/_expo/static/js/web/index-abc.js",
		Headers:         map[string]string{"If-Modified-Since": oldTime.Format(http.TimeFormat)},
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"chunks: new"},
	})
}

func TestPoolAssets_404OnMissingFile(t *testing.T) {
	releasesDir := setupPool(t, "_expo/static/js/web", "bundle-abc.js", "x")

	runPoolScenario(t, releasesDir, "_expo/static", bundleCacheControl, &tests.ApiScenario{
		Name:            "404 when chunk no longer in pool",
		Method:          http.MethodGet,
		URL:             "/_expo/static/js/web/bundle-zzz.js",
		ExpectedStatus:  http.StatusNotFound,
		ExpectedContent: []string{`"status":404`},
	})
}

func TestPoolAssets_ServesAssetsWithShortCache(t *testing.T) {
	releasesDir := setupPool(t, "assets", "app-icon.png", "PNGDATA")

	runPoolScenario(t, releasesDir, "assets", "public, max-age=300", &tests.ApiScenario{
		Name:            "serves /assets/ with short cache (mixed-hash subtree)",
		Method:          http.MethodGet,
		URL:             "/assets/app-icon.png",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"PNGDATA"},
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			cc := res.Header.Get("Cache-Control")
			if cc != "public, max-age=300" {
				t.Errorf("Cache-Control = %q", cc)
			}
		},
	})
}

// Compile-time assertion that the handler returns the expected signature.
var _ func(*core.RequestEvent) error = PoolAssets("", "", "")
