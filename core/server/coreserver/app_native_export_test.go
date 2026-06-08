package coreserver

import (
	"encoding/json"
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

	bm, err := parseExportMetadata(dir, platformIOS, "build-100", "1.13.7")
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
	bm := bundleMeta{
		Platform:   "ios",
		BundleFile: "_expo/static/js/ios/i.hbc",
		Assets:     []assetMeta{{File: "assets/a"}},
		distDir:    src,
	}
	if err := stageNativeBundlesIntoRelease(releaseDir, []bundleMeta{bm}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	for _, rel := range []string{"_expo/static/js/ios/i.hbc", "assets/a"} {
		p := filepath.Join(releaseDir, "native", "ios", rel)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected staged file %s: %v", p, err)
		}
	}
}

// TestSerializeBundlesRoundTripsToResolveManifest is the write→read contract
// test for native OTA bundles: the install pipeline writes serializeBundles()
// output into the pkg_build `bundles` JSON field, and the /api/app/update
// endpoint reads it back via resolveManifest(). Those two sides were built and
// unit-tested in isolation, so this guards the seam between them — including the
// JSON marshal/unmarshal the PocketBase JSON field performs, which turns the
// typed []bundleMeta into the []any / map[string]any shape resolveManifest
// asserts on. A field-name drift on either side (e.g. content_type vs
// contentType) or a type-assertion mismatch after the round-trip fails here.
func TestSerializeBundlesRoundTripsToResolveManifest(t *testing.T) {
	bundles := []bundleMeta{
		{
			Platform:       "ios",
			BundleID:       "build-200-ios",
			BundleHash:     "deadbeef",
			BundleFile:     "_expo/static/js/ios/index.hbc",
			RuntimeVersion: "1.13.7",
			Assets: []assetMeta{
				{Key: "assets/img", Hash: "cafef00d", ContentType: "image/png", File: "assets/img"},
			},
		},
		{
			Platform:       "android",
			BundleID:       "build-200-android",
			BundleHash:     "0badf00d",
			BundleFile:     "_expo/static/js/android/index.hbc",
			RuntimeVersion: "1.13.7",
			Assets:         nil,
		},
	}

	// Mirror the PocketBase JSON field: serialize → marshal → unmarshal into []any.
	serialized := serializeBundles(bundles)
	raw, err := json.Marshal(serialized)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded []any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// iOS: a newer build than the client's current → full manifest, asset intact.
	m, status := resolveManifest(decoded, "ios", "1.13.7", "build-100-ios", "")
	if status != manifestNew {
		t.Fatalf("ios status = %v, want manifestNew", status)
	}
	if m.ID != "build-200-ios" || m.BundleHash != "deadbeef" || m.BundleFile != "_expo/static/js/ios/index.hbc" {
		t.Fatalf("ios manifest = %+v", m)
	}
	if len(m.Assets) != 1 || m.Assets[0].Key != "assets/img" || m.Assets[0].Hash != "cafef00d" || m.Assets[0].ContentType != "image/png" {
		t.Fatalf("ios assets = %+v", m.Assets)
	}

	// Android: the assetless bundle still resolves; assets is an empty slice.
	ma, statusA := resolveManifest(decoded, "android", "1.13.7", "build-100-android", "")
	if statusA != manifestNew || ma.ID != "build-200-android" {
		t.Fatalf("android manifest = %+v (status %v)", ma, statusA)
	}

	// Same id the client already runs → up to date (204 on the wire).
	if _, s := resolveManifest(decoded, "ios", "1.13.7", "build-200-ios", ""); s != manifestUpToDate {
		t.Fatalf("expected manifestUpToDate, got %v", s)
	}

	// A runtime the build has no bundle for → no match (204 / App Store gate).
	if _, s := resolveManifest(decoded, "ios", "2.0.0", "build-100-ios", ""); s != manifestNoMatch {
		t.Fatalf("expected manifestNoMatch for mismatched runtime, got %v", s)
	}
}
