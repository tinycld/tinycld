package coreserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// setupCurrentManifest creates a temp directory with a current/manifest.json
// file holding the given raw JSON body.
func setupCurrentManifest(t *testing.T, body string) string {
	t.Helper()
	releasesDir := t.TempDir()
	currentDir := filepath.Join(releasesDir, "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll current: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(currentDir, "manifest.json"),
		[]byte(body),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile manifest.json: %v", err)
	}
	return releasesDir
}

// runReleaseHandlerScenario boots a TestApp with the ReleaseHandler mounted at
// GET /api/release and drives the given ApiScenario.
func runReleaseHandlerScenario(t *testing.T, releasesDir string, scenario *tests.ApiScenario) {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	defer app.Cleanup()

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.RouterGroup.GET("/api/release", ReleaseHandler(releasesDir))
		return e.Next()
	})

	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

func TestReleaseHandler_ReturnsManifest(t *testing.T) {
	body := `{"appTag":"v0.0.3","members":[{"name":"mail","tag":"v0.1.0","sha":"1111111aaaa"}]}`
	releasesDir := setupCurrentManifest(t, body)

	runReleaseHandlerScenario(t, releasesDir, &tests.ApiScenario{
		Name:           "echoes the manifest with no-store cache",
		Method:         http.MethodGet,
		URL:            "/api/release",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"appTag":"v0.0.3"`,
			`"name":"mail"`,
			`"tag":"v0.1.0"`,
		},
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want %q", cc, "no-store")
			}
		},
	})
}

func TestReleaseHandler_EmptyWhenMissing(t *testing.T) {
	// Empty temp dir with no current/manifest.json — degrade to empty shape.
	releasesDir := t.TempDir()

	runReleaseHandlerScenario(t, releasesDir, &tests.ApiScenario{
		Name:            "returns empty members with 200 when manifest is absent",
		Method:          http.MethodGet,
		URL:             "/api/release",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"members":[]`},
	})
}

func TestReleaseHandler_EmptyWhenCorrupt(t *testing.T) {
	releasesDir := setupCurrentManifest(t, "{not valid json")

	runReleaseHandlerScenario(t, releasesDir, &tests.ApiScenario{
		Name:            "returns empty members with 200 when manifest is corrupt",
		Method:          http.MethodGet,
		URL:             "/api/release",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{`"members":[]`},
	})
}

// Compile-time assertion: ReleaseHandler must have the correct signature.
var _ func(*core.RequestEvent) error = ReleaseHandler("")
