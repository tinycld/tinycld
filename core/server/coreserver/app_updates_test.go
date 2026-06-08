package coreserver

import "testing"

func bundlesFixture() []any {
	return []any{
		map[string]any{
			"platform": "ios", "bundle_id": "build-200-ios", "bundle_hash": "HASH",
			"bundle_file": "_expo/static/js/ios/i.hbc", "runtime_version": "1.13.7",
			"assets": []any{map[string]any{"key": "assets/a", "hash": "AH", "content_type": "image/png", "file": "assets/a"}},
		},
	}
}

func TestResolveManifestNewBundle(t *testing.T) {
	m, status := resolveManifest(bundlesFixture(), "ios", "1.13.7", "build-100-ios")
	if status != manifestNew {
		t.Fatalf("status = %v, want manifestNew", status)
	}
	if m.ID != "build-200-ios" || m.BundleHash != "HASH" || len(m.Assets) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestResolveManifestUpToDate(t *testing.T) {
	_, status := resolveManifest(bundlesFixture(), "ios", "1.13.7", "build-200-ios")
	if status != manifestUpToDate {
		t.Fatalf("status = %v, want manifestUpToDate", status)
	}
}

func TestResolveManifestRuntimeMismatch(t *testing.T) {
	_, status := resolveManifest(bundlesFixture(), "ios", "1.14.0", "build-100-ios")
	if status != manifestNoMatch {
		t.Fatalf("status = %v, want manifestNoMatch", status)
	}
}

func TestResolveManifestPlatformMissing(t *testing.T) {
	_, status := resolveManifest(bundlesFixture(), "android", "1.13.7", "x")
	if status != manifestNoMatch {
		t.Fatalf("status = %v, want manifestNoMatch", status)
	}
}

func TestFillManifestURLs(t *testing.T) {
	m := clientManifest{
		ID: "build-200-ios", BundleFile: "_expo/static/js/ios/i.hbc",
		Assets: []manifestAsset{{Key: "assets/a", File: "assets/a"}},
	}
	fillManifestURLs(&m, "build-200", "ios")
	if m.BundleURL != "/api/app/bundle/build-200/ios/_expo/static/js/ios/i.hbc" {
		t.Fatalf("bundle url = %q", m.BundleURL)
	}
	if m.Assets[0].URL != "/api/app/asset/build-200/ios/assets/a" {
		t.Fatalf("asset url = %q", m.Assets[0].URL)
	}
}
