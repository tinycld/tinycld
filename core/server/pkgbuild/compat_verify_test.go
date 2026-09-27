package pkgbuild_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/pkgbuild/pkgbuildtest"
)

// The gap this function exists to close: a target version whose FRESH manifest
// tightens its own peerVersions beyond what the installed set satisfies. The
// pre-flight gates read only pkg_registry.manifest_json, so this is invisible
// to them — the post-assemble verify must refuse from the on-disk manifest.
func TestVerifyTargetPeerVersions_TightenedTargetRefused(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.4")
	pkgbuildtest.WriteBuildMember(t, buildDir, "gizmos", "2.0.0",
		map[string]string{pkgbuild.CorePackageKey: ">=0.5.0 <0.6.0"})

	m := pkgbuild.RebuildManifest{BuildID: "b", Members: []pkgbuild.MemberSpec{
		// Deliberately-wrong manifest versions: the verify must read what is
		// actually ON DISK, not trust the delta's version strings.
		{Slug: pkgbuild.BaseMemberSlug, Version: "9.9.9", FromCurrent: true},
		{Slug: "gizmos", Version: "9.9.9"},
	}}
	err := pkgbuild.VerifyTargetPeerVersions(m, buildDir)
	if err == nil {
		t.Fatal("expected refusal: gizmos's fetched manifest requires core >=0.5.0 <0.6.0 against core 0.0.4")
	}
	for _, want := range []string{"gizmos", ">=0.5.0 <0.6.0", "0.0.4"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error should name %q, got: %v", want, err)
		}
	}
}

// Positive control: the same shape with a satisfied range passes.
func TestVerifyTargetPeerVersions_SatisfiedSetPasses(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.4")
	pkgbuildtest.WriteBuildMember(t, buildDir, "gizmos", "2.0.0",
		map[string]string{pkgbuild.CorePackageKey: ">=0.0.4 <0.1.0"})

	m := pkgbuild.RebuildManifest{BuildID: "b", Members: []pkgbuild.MemberSpec{
		{Slug: pkgbuild.BaseMemberSlug, FromCurrent: true},
		{Slug: "gizmos"},
	}}
	if err := pkgbuild.VerifyTargetPeerVersions(m, buildDir); err != nil {
		t.Fatalf("expected satisfied set to pass, got %v", err)
	}
}

// A cross-package requirement on a member ABSENT from the build (e.g. an
// uninstall that would break a remaining package) is a violation. The
// pre-flight uninstall path runs no compat check at all, so this is the only
// gate for it.
func TestVerifyTargetPeerVersions_MissingPeerRefused(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.4")
	pkgbuildtest.WriteBuildMember(t, buildDir, "sprockets-slots", "1.0.0",
		map[string]string{"sprockets": "^1.0.0"})

	m := pkgbuild.RebuildManifest{BuildID: "b", Members: []pkgbuild.MemberSpec{
		{Slug: pkgbuild.BaseMemberSlug, FromCurrent: true},
		{Slug: "sprockets-slots", FromCurrent: true},
	}}
	err := pkgbuild.VerifyTargetPeerVersions(m, buildDir)
	if err == nil {
		t.Fatal("expected refusal: sprockets-slots requires sprockets, which the build set lacks")
	}
	if !strings.Contains(err.Error(), "sprockets") || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("error should name the missing peer, got: %v", err)
	}
}

// A member whose manifest cannot be read is a hard failure, not a pass — the
// verify cannot vouch for requirements it cannot see.
func TestVerifyTargetPeerVersions_UnreadableManifestFailsClosed(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.4")
	if err := os.MkdirAll(filepath.Join(buildDir, "gizmos"), 0o755); err != nil {
		t.Fatal(err)
	} // member dir exists, no manifest.ts

	m := pkgbuild.RebuildManifest{BuildID: "b", Members: []pkgbuild.MemberSpec{
		{Slug: pkgbuild.BaseMemberSlug, FromCurrent: true},
		{Slug: "gizmos"},
	}}
	err := pkgbuild.VerifyTargetPeerVersions(m, buildDir)
	if err == nil {
		t.Fatal("expected an unreadable member manifest to fail the verify")
	}
	if !strings.Contains(err.Error(), "gizmos") {
		t.Fatalf("error should name the unreadable member, got: %v", err)
	}
}
