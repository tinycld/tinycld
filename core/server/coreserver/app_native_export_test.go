package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The export/staging behavior itself is covered in pkgbuild/nativeexport_test.go.
// What stays here are the host-side seams: archiving staged bundles into the
// durable build archive, and the serialize→resolve round trip through the
// pkg_build JSON field.

// TestArchiveNativeBundlesToRelease guards the install-pipeline fix that promotes
// staged native bundles into the durable build archive's release dir — the exact
// path /api/app/bundle (serveBuildFile) reads. Before this, the manifest
// advertised a bundle URL whose file 404'd (the natives sat only in the transient
// release-staging dir), so a mobile client found an update but could never
// download it. The relative layout under native/<platform>/ must be preserved so
// the recorded bundle_file/asset paths resolve.
func TestArchiveNativeBundlesToRelease(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	buildID := "build-123"

	// Lay out a staged release dir as the pipeline does: <stageDir>/native/<plat>/...
	stageDir := t.TempDir()
	staged := filepath.Join(stageDir, "native", "ios", "_expo/static/js/ios")
	if err := os.MkdirAll(staged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "entry.hbc"), []byte("BUNDLE"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := archiveNativeBundlesToRelease(buildID, stageDir); err != nil {
		t.Fatalf("archive: %v", err)
	}

	// The file must now live where serveBuildFile reads it:
	// buildArchiveFor(buildID).release/native/<platform>/<bundle_file>.
	want := filepath.Join(buildArchiveFor("", buildID).release, "native", "ios", "_expo/static/js/ios/entry.hbc")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected archived bundle at %s: %v", want, err)
	}
	if string(got) != "BUNDLE" {
		t.Fatalf("archived bundle content = %q, want %q", got, "BUNDLE")
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
	m, status := resolveNoBad(decoded, "ios", "1.13.7", "build-100-ios", "")
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
	ma, statusA := resolveNoBad(decoded, "android", "1.13.7", "build-100-android", "")
	if statusA != manifestNew || ma.ID != "build-200-android" {
		t.Fatalf("android manifest = %+v (status %v)", ma, statusA)
	}

	// Same id the client already runs → up to date (204 on the wire).
	if _, s := resolveNoBad(decoded, "ios", "1.13.7", "build-200-ios", ""); s != manifestUpToDate {
		t.Fatalf("expected manifestUpToDate, got %v", s)
	}

	// A runtime the build has no bundle for → no match (204 / App Store gate).
	if _, s := resolveNoBad(decoded, "ios", "2.0.0", "build-100-ios", ""); s != manifestNoMatch {
		t.Fatalf("expected manifestNoMatch for mismatched runtime, got %v", s)
	}
}
