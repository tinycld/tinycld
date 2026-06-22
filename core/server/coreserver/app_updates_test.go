package coreserver

import "testing"

// resolveNoBad calls resolveManifest with no reported-bad bundles — the common
// case for tests that don't exercise the bad-bundle skip.
func resolveNoBad(bundles []any, platform, runtime, currentID, currentHash string) (clientManifest, manifestStatus) {
	return resolveManifest(bundles, platform, runtime, currentID, currentHash, nil, nil)
}

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
	m, status := resolveNoBad(bundlesFixture(), "ios", "1.13.7", "build-100-ios", "")
	if status != manifestNew {
		t.Fatalf("status = %v, want manifestNew", status)
	}
	if m.ID != "build-200-ios" || m.BundleHash != "HASH" || len(m.Assets) != 1 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestResolveManifestUpToDate(t *testing.T) {
	_, status := resolveNoBad(bundlesFixture(), "ios", "1.13.7", "build-200-ios", "")
	if status != manifestUpToDate {
		t.Fatalf("status = %v, want manifestUpToDate", status)
	}
}

// A fresh App Store install reports the embedded id (never a server build id) but
// its bytecode can be identical to the server's current bundle. The hash match
// must then report up-to-date so the app doesn't download + reload on every first
// foreground after a store update.
func TestResolveManifestUpToDateByHash(t *testing.T) {
	_, status := resolveNoBad(bundlesFixture(), "ios", "1.13.7", "embedded-1.13.7", "HASH")
	if status != manifestUpToDate {
		t.Fatalf("status = %v, want manifestUpToDate (hash match across embedded→server boundary)", status)
	}
}

// A non-matching hash with a non-matching id must still offer the update — the
// hash short-circuit only suppresses, never blocks, a genuinely newer bundle.
func TestResolveManifestNewWhenHashDiffers(t *testing.T) {
	_, status := resolveNoBad(bundlesFixture(), "ios", "1.13.7", "embedded-1.13.7", "OTHERHASH")
	if status != manifestNew {
		t.Fatalf("status = %v, want manifestNew", status)
	}
}

func TestResolveManifestRuntimeMismatch(t *testing.T) {
	_, status := resolveNoBad(bundlesFixture(), "ios", "1.14.0", "build-100-ios", "")
	if status != manifestNoMatch {
		t.Fatalf("status = %v, want manifestNoMatch", status)
	}
}

func TestResolveManifestPlatformMissing(t *testing.T) {
	_, status := resolveNoBad(bundlesFixture(), "android", "1.13.7", "x", "")
	if status != manifestNoMatch {
		t.Fatalf("status = %v, want manifestNoMatch", status)
	}
}

// A bundle a client reported bad must be skipped: by id, and independently by
// hash. With the only matching bundle skipped, the result is manifestNoMatch
// (nothing to offer) rather than pushing the known-bricking bundle again.
func TestResolveManifestSkipsBadBundleByID(t *testing.T) {
	badIDs := map[string]bool{"build-200-ios": true}
	_, status := resolveManifest(bundlesFixture(), "ios", "1.13.7", "build-100-ios", "", badIDs, nil)
	if status != manifestNoMatch {
		t.Fatalf("status = %v, want manifestNoMatch (bad bundle skipped by id)", status)
	}
}

func TestResolveManifestSkipsBadBundleByHash(t *testing.T) {
	badHashes := map[string]bool{"HASH": true}
	_, status := resolveManifest(bundlesFixture(), "ios", "1.13.7", "build-100-ios", "", nil, badHashes)
	if status != manifestNoMatch {
		t.Fatalf("status = %v, want manifestNoMatch (bad bundle skipped by hash)", status)
	}
}

// A non-bad bundle is still offered normally when an unrelated id is in the bad
// set — the skip must be specific, not a blanket "any bad bundle blocks updates".
func TestResolveManifestUnrelatedBadDoesNotBlock(t *testing.T) {
	badIDs := map[string]bool{"build-999-ios": true}
	_, status := resolveManifest(bundlesFixture(), "ios", "1.13.7", "build-100-ios", "", badIDs, nil)
	if status != manifestNew {
		t.Fatalf("status = %v, want manifestNew (unrelated bad id must not block)", status)
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

func TestBuildIDPatternRejectsTraversal(t *testing.T) {
	valid := []string{"build-123", "build-1717000000000", "build-base"}
	for _, v := range valid {
		if !buildIDPattern.MatchString(v) {
			t.Errorf("expected %q to be a valid build id", v)
		}
	}
	// These are exactly the shapes a percent-decoded path segment could carry
	// into serveBuildFile; all must be rejected before the path join.
	bad := []string{
		"../../../etc", "..", "build-1/../..", "build-", "build-abc",
		"build-123/x", "", "..%2f..", "/etc/passwd",
	}
	for _, b := range bad {
		if buildIDPattern.MatchString(b) {
			t.Errorf("expected %q to be rejected as a build id", b)
		}
	}
}
