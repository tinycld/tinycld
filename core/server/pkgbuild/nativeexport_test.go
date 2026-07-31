package pkgbuild

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSha256OfFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "bundle.hbc")
	if err := os.WriteFile(p, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	// hex sha256("hello world")
	want := "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"
	got, err := sha256OfFile(p)
	if err != nil {
		t.Fatalf("sha256OfFile: %v", err)
	}
	if got != want {
		t.Fatalf("hash = %q, want %q", got, want)
	}
}

func TestParseExportMetadata(t *testing.T) {
	dir := t.TempDir()
	meta := `{
	  "version": 0,
	  "bundler": "metro",
	  "fileMetadata": {
	    "ios": {
	      "bundle": "_expo/static/js/ios/index-abc.hbc",
	      "assets": [ { "path": "assets/img-1", "ext": "png" } ]
	    }
	  }
	}`
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_expo/static/js/ios"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_expo/static/js/ios/index-abc.hbc"), []byte("BUNDLE"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets/img-1"), []byte("IMG"), 0o644); err != nil {
		t.Fatal(err)
	}

	bm, err := parseExportMetadata(dir, PlatformIOS, "build-100", "1.13.7")
	if err != nil {
		t.Fatalf("parseExportMetadata: %v", err)
	}
	if bm.Platform != "ios" || bm.BundleID != "build-100-ios" {
		t.Fatalf("bad meta: %+v", bm)
	}
	if bm.BundleFile != "_expo/static/js/ios/index-abc.hbc" {
		t.Fatalf("bundle file = %q", bm.BundleFile)
	}
	if bm.RuntimeVersion != "1.13.7" {
		t.Fatalf("runtime = %q", bm.RuntimeVersion)
	}
	if len(bm.Assets) != 1 || bm.Assets[0].File != "assets/img-1" || bm.Assets[0].ContentType != "image/png" {
		t.Fatalf("assets = %+v", bm.Assets)
	}
	if bm.BundleHash == "" || bm.Assets[0].Hash == "" {
		t.Fatalf("expected non-empty hashes: %+v", bm)
	}
}

func TestNativeToolchainPresent(t *testing.T) {
	dir := t.TempDir()
	if nativeToolchainPresent(dir) {
		t.Fatal("expected absent toolchain with empty appDir")
	}
	if err := os.MkdirAll(filepath.Join(dir, "node_modules", "expo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !nativeToolchainPresent(dir) {
		t.Fatal("expected present toolchain once node_modules/expo exists")
	}
}

func TestStageNativeBundlesIntoRelease(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "_expo/static/js/ios"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "_expo/static/js/ios/i.hbc"), []byte("B"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "assets/a"), []byte("A"), 0o644); err != nil {
		t.Fatal(err)
	}

	releaseDir := t.TempDir()
	bm := BundleMeta{
		Platform:   "ios",
		BundleFile: "_expo/static/js/ios/i.hbc",
		Assets:     []AssetMeta{{File: "assets/a"}},
		DistDir:    src,
	}
	if err := stageNativeBundlesIntoRelease(releaseDir, []BundleMeta{bm}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	for _, rel := range []string{"_expo/static/js/ios/i.hbc", "assets/a"} {
		p := filepath.Join(releaseDir, "native", "ios", rel)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected staged file %s: %v", p, err)
		}
	}
}

func TestAppVersionFromManifest(t *testing.T) {
	writeJSON := func(t *testing.T, dir, name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// The production shape: app.json carries no expo.version (app.config.ts
	// injects it from package.json), so the resolver must fall back to
	// package.json's version.
	t.Run("falls back to package.json when app.json has no expo.version", func(t *testing.T) {
		dir := t.TempDir()
		writeJSON(t, dir, "app.json", `{"expo":{"name":"TinyCld"}}`)
		writeJSON(t, dir, "package.json", `{"name":"@tinycld/app","version":"2.0.0"}`)
		if got := appVersionFromManifest(dir); got != "2.0.0" {
			t.Fatalf("got %q, want 2.0.0", got)
		}
	})

	// A statically-pinned expo.version still wins (don't break a build that sets it).
	t.Run("prefers expo.version when present", func(t *testing.T) {
		dir := t.TempDir()
		writeJSON(t, dir, "app.json", `{"expo":{"version":"9.9.9"}}`)
		writeJSON(t, dir, "package.json", `{"version":"2.0.0"}`)
		if got := appVersionFromManifest(dir); got != "9.9.9" {
			t.Fatalf("got %q, want 9.9.9", got)
		}
	})

	// Dev layout: manifests live one dir above appDir.
	t.Run("reads package.json from the parent dir", func(t *testing.T) {
		parent := t.TempDir()
		writeJSON(t, parent, "package.json", `{"version":"3.1.4"}`)
		appDir := filepath.Join(parent, "tinycld")
		if err := os.MkdirAll(appDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if got := appVersionFromManifest(appDir); got != "3.1.4" {
			t.Fatalf("got %q, want 3.1.4", got)
		}
	})

	// Neither source has a version → empty (the caller turns this into a loud error).
	t.Run("returns empty when no version anywhere", func(t *testing.T) {
		dir := t.TempDir()
		writeJSON(t, dir, "app.json", `{"expo":{"name":"x"}}`)
		writeJSON(t, dir, "package.json", `{"name":"x"}`)
		if got := appVersionFromManifest(dir); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

// TestSentryReleaseFor locks the Sentry release string format. This MUST stay in
// lockstep with the client (core/lib/sentry.ts → sentryReleaseAndDist), which
// reports `tinycld@<version>` for a promoted OTA bundle. If these two ever
// diverge, OTA-bundle crashes upload under one release and report under another,
// and symbolication silently fails — so this test is the canary for that drift.
func TestSentryReleaseFor(t *testing.T) {
	if got := sentryReleaseFor("2.0.0"); got != "tinycld@2.0.0" {
		t.Fatalf("sentryReleaseFor(2.0.0) = %q, want tinycld@2.0.0", got)
	}
	if got := sentryReleaseFor("1.13.7"); got != "tinycld@1.13.7" {
		t.Fatalf("sentryReleaseFor(1.13.7) = %q, want tinycld@1.13.7", got)
	}
}

func TestOrDefault(t *testing.T) {
	if got := orDefault("", "fallback"); got != "fallback" {
		t.Fatalf("empty value should yield fallback, got %q", got)
	}
	if got := orDefault("set-value", "fallback"); got != "set-value" {
		t.Fatalf("non-empty value should win, got %q", got)
	}
}
