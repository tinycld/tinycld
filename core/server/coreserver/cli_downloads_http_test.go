package coreserver

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgbuild"
)

// newCliDownloadTestApp registers the REAL endpoints against a seeded temp
// dist dir. Only darwin-arm64 and windows-amd64 binaries exist, so the listing
// tests also cover a partially-failed (best-effort) cross-compile.
func newCliDownloadTestApp(t *testing.T, seed bool) (*tests.TestApp, string) {
	t.Helper()
	app, err := tests.NewTestApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	dist := t.TempDir()
	if seed {
		if err := os.WriteFile(filepath.Join(dist, "tinycld-darwin-arm64"), []byte("mach-o bytes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dist, "tinycld-windows-amd64.exe"), []byte("pe bytes"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	registerCliDownloadEndpoints(app, func() string { return dist })
	return app, dist
}

func runCliDownloadScenario(t *testing.T, app *tests.TestApp, scenario *tests.ApiScenario) {
	t.Helper()
	scenario.TestAppFactory = func(_ testing.TB) *tests.TestApp { return app }
	scenario.DisableTestAppCleanup = true
	scenario.Test(t)
}

func TestCliDownloadsListsBuiltPlatforms(t *testing.T) {
	app, _ := newCliDownloadTestApp(t, true)
	runCliDownloadScenario(t, app, &tests.ApiScenario{
		Name:           "list includes only the built platforms, unauthenticated",
		Method:         http.MethodGet,
		URL:            "/api/cli/downloads",
		ExpectedStatus: 200,
		ExpectedContent: []string{
			`"platform":"darwin-arm64"`,
			`"filename":"tinycld"`,
			`"url":"/api/cli/download/darwin-arm64"`,
			`"platform":"windows-amd64"`,
			`"filename":"tinycld.exe"`,
			`"size":12`,
		},
		NotExpectedContent: []string{
			`"platform":"linux-amd64"`,
			`"platform":"linux-arm64"`,
			`"platform":"darwin-amd64"`,
		},
	})
}

func TestCliDownloadsEmptyDistIsEmptyList(t *testing.T) {
	app, _ := newCliDownloadTestApp(t, false)
	runCliDownloadScenario(t, app, &tests.ApiScenario{
		Name:            "missing dist dir serves an empty array, not null or an error",
		Method:          http.MethodGet,
		URL:             "/api/cli/downloads",
		ExpectedStatus:  200,
		ExpectedContent: []string{`"downloads":[]`},
	})
}

func TestCliDownloadStreamsBinary(t *testing.T) {
	app, _ := newCliDownloadTestApp(t, true)
	runCliDownloadScenario(t, app, &tests.ApiScenario{
		Name:            "download streams the exact bytes",
		Method:          http.MethodGet,
		URL:             "/api/cli/download/darwin-arm64",
		ExpectedStatus:  200,
		ExpectedContent: []string{"mach-o bytes"},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			if got := res.Header.Get("Content-Disposition"); got != `attachment; filename="tinycld"` {
				t.Fatalf("Content-Disposition = %q", got)
			}
		},
	})
}

func TestCliDownloadWindowsFilename(t *testing.T) {
	app, _ := newCliDownloadTestApp(t, true)
	runCliDownloadScenario(t, app, &tests.ApiScenario{
		Name:            "windows download suggests tinycld.exe",
		Method:          http.MethodGet,
		URL:             "/api/cli/download/windows-amd64",
		ExpectedStatus:  200,
		ExpectedContent: []string{"pe bytes"},
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, res *http.Response) {
			if got := res.Header.Get("Content-Disposition"); got != `attachment; filename="tinycld.exe"` {
				t.Fatalf("Content-Disposition = %q", got)
			}
		},
	})
}

func TestCliDownloadRejectsUnknownPlatforms(t *testing.T) {
	for _, platform := range []string{
		"darwin-mips",
		"..%2F..%2Fetc%2Fpasswd",
		"tinycld-darwin-arm64",
		"build-1",
	} {
		t.Run(platform, func(t *testing.T) {
			app, _ := newCliDownloadTestApp(t, true)
			runCliDownloadScenario(t, app, &tests.ApiScenario{
				Name:           "unknown platform " + platform,
				Method:         http.MethodGet,
				URL:            "/api/cli/download/" + platform,
				ExpectedStatus: 404,
				ExpectedContent: []string{
					`"status":404`,
				},
			})
		})
	}
}

func TestCliDownloadNotBuiltPlatformIs404(t *testing.T) {
	// linux-amd64 is a valid target but was not built (best-effort compile).
	app, _ := newCliDownloadTestApp(t, true)
	runCliDownloadScenario(t, app, &tests.ApiScenario{
		Name:            "valid but unbuilt platform 404s",
		Method:          http.MethodGet,
		URL:             "/api/cli/download/linux-amd64",
		ExpectedStatus:  404,
		ExpectedContent: []string{`"status":404`},
	})
}

// Pins that the URL keys the listing advertises are exactly the platform
// strings the download route accepts.
func TestCliDownloadURLsMatchTargets(t *testing.T) {
	for _, target := range pkgbuild.CLITargets {
		if target.Platform() == "" || target.FileName() == "" {
			t.Fatalf("target %+v has empty naming", target)
		}
	}
}
