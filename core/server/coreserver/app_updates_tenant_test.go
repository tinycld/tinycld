package coreserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/tenantcfg"
)

// tenantArtifact lays out a minimal artifact + materialized org dir: a
// recipe.json carrying one ios bundle, and the bundle bytes under
// pb_public/native/ios/ where a real build stages them.
func tenantArtifact(t *testing.T, recipe tenantcfg.ArtifactRecipe) (orgDir, artifactDir string) {
	t.Helper()
	artifactDir = t.TempDir()
	orgDir = t.TempDir()

	raw, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tenantcfg.ArtifactRecipePath(artifactDir), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	hbc := filepath.Join(orgDir, "pb_public", "native", "ios", "_expo", "static", "js", "ios", "index.hbc")
	if err := os.MkdirAll(filepath.Dir(hbc), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hbc, []byte("IOSBYTES"), 0o644); err != nil {
		t.Fatal(err)
	}
	return orgDir, artifactDir
}

func iosRecipe() tenantcfg.ArtifactRecipe {
	return tenantcfg.ArtifactRecipe{
		RecipeHash:     "sha256:" + "ab12cd34ef56" + "00000000000000000000000000000000000000000000000000",
		BuildID:        "recipe-ab12cd34ef56",
		RuntimeVersion: "0.4.0",
		Bundles: []pkgbuild.BundleMeta{{
			Platform:       "ios",
			BundleID:       "recipe-ab12cd34ef56-ios",
			BundleHash:     "deadbeef",
			BundleFile:     "_expo/static/js/ios/index.hbc",
			RuntimeVersion: "0.4.0",
		}},
	}
}

// newTenantUpdateTestApp registers the real tenant OTA endpoints against an
// artifact on disk. pkg_bad_bundle exists because loadBadBundles queries it.
func newTenantUpdateTestApp(t *testing.T, recipe tenantcfg.ArtifactRecipe) *tests.TestApp {
	t.Helper()
	app := newBadBundleTestApp(t)
	orgDir, artifactDir := tenantArtifact(t, recipe)
	RegisterTenantAppUpdateEndpoints(app, orgDir, artifactDir)
	return app
}

// TestTenantAppUpdate_ServesManifestFromArtifact is the core of DESIGN §6: a
// hosted org's mobile client gets a manifest from the org's OWN artifact, with
// no pkg_build row anywhere (an org dir has no build archive).
func TestTenantAppUpdate_ServesManifestFromArtifact(t *testing.T) {
	app := newTenantUpdateTestApp(t, iosRecipe())
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "200 manifest from the tenant's artifact recipe",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=0.4.0&currentId=embedded-0.4.0",
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"id":"recipe-ab12cd34ef56-ios"`,
			`"bundleHash":"deadbeef"`,
			`/api/app/bundle/recipe-ab12cd34ef56/ios/_expo/static/js/ios/index.hbc`,
		},
	})
}

// The content-addressed bundle id is what lets a device that already runs this
// package set be told "up to date" — including after moving between two orgs
// that resolve to the SAME recipe, which is the whole point of sharing
// artifacts by hash.
func TestTenantAppUpdate_204WhenClientRunsThisRecipe(t *testing.T) {
	app := newTenantUpdateTestApp(t, iosRecipe())
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "204 when the client already runs this recipe's bundle",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=0.4.0&currentId=recipe-ab12cd34ef56-ios",
		ExpectedStatus: http.StatusNoContent,
	})
}

func TestTenantAppUpdate_204OnRuntimeMismatch(t *testing.T) {
	app := newTenantUpdateTestApp(t, iosRecipe())
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "204 when the org's bundle targets a different app version",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=9.9.9&currentId=embedded-9.9.9",
		ExpectedStatus: http.StatusNoContent,
	})
}

// An artifact built before bundles were recorded (or by a web-only toolchain)
// must answer 204, leaving mobile on its embedded bundle — never 500, and never
// advertising something that isn't there.
func TestTenantAppUpdate_204WhenArtifactHasNoBundles(t *testing.T) {
	recipe := iosRecipe()
	recipe.Bundles = nil
	app := newTenantUpdateTestApp(t, recipe)
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:           "204 when the artifact records no native bundles",
		Method:         http.MethodGet,
		URL:            "/api/app/update?platform=ios&runtimeVersion=0.4.0&currentId=embedded-0.4.0",
		ExpectedStatus: http.StatusNoContent,
	})
}

// TestTenantAppUpdate_ServesBundleBytes proves the manifest's bundleUrl is
// actually fetchable from the org's materialized pb_public — the pairing whose
// absence would present to a device as a spurious rollback (manifest advertises,
// download 404s).
func TestTenantAppUpdate_ServesBundleBytes(t *testing.T) {
	app := newTenantUpdateTestApp(t, iosRecipe())
	runAppUpdateScenario(t, app, &tests.ApiScenario{
		Name:            "200 bundle bytes from pb_public/native",
		Method:          http.MethodGet,
		URL:             "/api/app/bundle/recipe-ab12cd34ef56/ios/_expo/static/js/ios/index.hbc",
		ExpectedStatus:  http.StatusOK,
		ExpectedContent: []string{"IOSBYTES"},
	})
}

// The widened buildIDPattern must not have opened a traversal hole: the id is
// interpolated into a filesystem path, and Go's mux percent-decodes segments.
func TestTenantAppUpdate_BundleEndpointRejectsTraversal(t *testing.T) {
	cases := []struct{ name, url string }{
		{"encoded ../ in build id", "/api/app/bundle/..%2f..%2f..%2fetc/ios/passwd"},
		{"encoded .. as whole build id", "/api/app/bundle/%2e%2e/ios/x"},
		{"recipe-shaped but non-hex", "/api/app/bundle/recipe-zzzzzzzzzzzz/ios/x"},
		{"recipe-shaped but wrong length", "/api/app/bundle/recipe-abc/ios/x"},
		{"unknown platform", "/api/app/bundle/recipe-ab12cd34ef56/linux/x"},
		{"traversal in the wildcard path", "/api/app/bundle/recipe-ab12cd34ef56/ios/..%2f..%2frecipe.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newTenantUpdateTestApp(t, iosRecipe())
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
