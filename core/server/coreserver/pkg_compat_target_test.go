package coreserver

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
)

// writeBuildMember fabricates a fetched member in the build dir: a manifest.ts
// whose object literal parseManifestViaNode evaluates, exactly what assemble
// leaves at <buildDir>/<memberSlug>.
func writeBuildMember(t *testing.T, buildDir, slug, version string, peerVersions map[string]string) {
	t.Helper()
	dir := filepath.Join(buildDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	peers := ""
	if len(peerVersions) > 0 {
		entries := make([]string, 0, len(peerVersions))
		for k, v := range peerVersions {
			entries = append(entries, fmt.Sprintf("'%s': '%s'", k, v))
		}
		peers = fmt.Sprintf("    peerVersions: { %s },\n", strings.Join(entries, ", "))
	}
	manifest := fmt.Sprintf(
		"export default {\n    name: '@tinycld/%s',\n    slug: '%s',\n    version: '%s',\n    description: 'test member',\n%s}\n",
		slug, slug, version, peers,
	)
	if err := os.WriteFile(filepath.Join(dir, "manifest.ts"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeBuildBase fabricates the tinycld base member, which ships no root
// manifest.ts — its version lives at core/package.json (the version peer ranges
// on @tinycld/core compare against).
func writeBuildBase(t *testing.T, buildDir, coreVersion string) {
	t.Helper()
	dir := filepath.Join(buildDir, baseMemberSlug, "core")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := fmt.Sprintf("{\n    \"name\": \"@tinycld/core\",\n    \"version\": \"%s\"\n}\n", coreVersion)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The gap this function exists to close: a target version whose FRESH manifest
// tightens its own peerVersions beyond what the installed set satisfies. The
// pre-flight gates read only pkg_registry.manifest_json, so this is invisible
// to them — the post-assemble verify must refuse from the on-disk manifest.
func TestVerifyTargetPeerVersions_TightenedTargetRefused(t *testing.T) {
	buildDir := t.TempDir()
	writeBuildBase(t, buildDir, "0.0.4")
	writeBuildMember(t, buildDir, "mail", "2.0.0",
		map[string]string{corePackageKey: ">=0.5.0 <0.6.0"})

	m := RebuildManifest{BuildID: "b", Members: []MemberSpec{
		// Deliberately-wrong manifest versions: the verify must read what is
		// actually ON DISK, not trust the delta's version strings.
		{Slug: baseMemberSlug, Version: "9.9.9", FromCurrent: true},
		{Slug: "mail", Version: "9.9.9"},
	}}
	err := verifyTargetPeerVersions(m, buildDir)
	if err == nil {
		t.Fatal("expected refusal: mail's fetched manifest requires core >=0.5.0 <0.6.0 against core 0.0.4")
	}
	for _, want := range []string{"mail", ">=0.5.0 <0.6.0", "0.0.4"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should name %q, got: %v", want, err)
		}
	}
}

// Positive control: the same shape with a satisfied range passes.
func TestVerifyTargetPeerVersions_SatisfiedSetPasses(t *testing.T) {
	buildDir := t.TempDir()
	writeBuildBase(t, buildDir, "0.0.4")
	writeBuildMember(t, buildDir, "mail", "2.0.0",
		map[string]string{corePackageKey: ">=0.0.4 <0.1.0"})

	m := RebuildManifest{BuildID: "b", Members: []MemberSpec{
		{Slug: baseMemberSlug, FromCurrent: true},
		{Slug: "mail"},
	}}
	if err := verifyTargetPeerVersions(m, buildDir); err != nil {
		t.Fatalf("expected satisfied set to pass, got %v", err)
	}
}

// A cross-package requirement on a member ABSENT from the build (e.g. an
// uninstall that would break a remaining package) is a violation. The
// pre-flight uninstall path runs no compat check at all, so this is the only
// gate for it.
func TestVerifyTargetPeerVersions_MissingPeerRefused(t *testing.T) {
	buildDir := t.TempDir()
	writeBuildBase(t, buildDir, "0.0.4")
	writeBuildMember(t, buildDir, "calendar-slots", "1.0.0",
		map[string]string{"calendar": "^1.0.0"})

	m := RebuildManifest{BuildID: "b", Members: []MemberSpec{
		{Slug: baseMemberSlug, FromCurrent: true},
		{Slug: "calendar-slots", FromCurrent: true},
	}}
	err := verifyTargetPeerVersions(m, buildDir)
	if err == nil {
		t.Fatal("expected refusal: calendar-slots requires calendar, which the build set lacks")
	}
	if !strings.Contains(err.Error(), "calendar") || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error should name the missing peer, got: %v", err)
	}
}

// A member whose manifest cannot be read is a hard failure, not a pass — the
// verify cannot vouch for requirements it cannot see.
func TestVerifyTargetPeerVersions_UnreadableManifestFailsClosed(t *testing.T) {
	buildDir := t.TempDir()
	writeBuildBase(t, buildDir, "0.0.4")
	if err := os.MkdirAll(filepath.Join(buildDir, "mail"), 0o755); err != nil {
		t.Fatal(err)
	} // member dir exists, no manifest.ts

	m := RebuildManifest{BuildID: "b", Members: []MemberSpec{
		{Slug: baseMemberSlug, FromCurrent: true},
		{Slug: "mail"},
	}}
	err := verifyTargetPeerVersions(m, buildDir)
	if err == nil {
		t.Fatal("expected an unreadable member manifest to fail the verify")
	}
	if !strings.Contains(err.Error(), "mail") {
		t.Fatalf("error should name the unreadable member, got: %v", err)
	}
}

// The production wiring: rebuild()'s dependency set must bind verifyCompat to
// the real checker. Without this, the orchestrator tests above could pass with
// verifyCompat left nil in production and the whole gate silently absent.
func TestProductionRebuildDeps_WireVerifyCompat(t *testing.T) {
	buildDir := t.TempDir()
	writeBuildBase(t, buildDir, "0.0.4")
	writeBuildMember(t, buildDir, "mail", "2.0.0",
		map[string]string{corePackageKey: ">=0.5.0 <0.6.0"})
	m := RebuildManifest{BuildID: "b", Members: []MemberSpec{
		{Slug: baseMemberSlug, FromCurrent: true},
		{Slug: "mail"},
	}}

	deps := productionRebuildDeps(pocketbase.New(), &installJob{ID: "j"}, m, nil)
	if deps.verifyCompat == nil {
		t.Fatal("production deps must wire verifyCompat")
	}
	err := deps.verifyCompat(m, buildDir)
	if err == nil || !strings.Contains(err.Error(), "mail") {
		t.Fatalf("production verifyCompat must refuse the violating build, got: %v", err)
	}
}
