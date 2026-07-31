package pkgbuild_test

import (
	"testing"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/pkgbuild/pkgbuildtest"
)

func TestMembersLock_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := []pkgbuild.ResolvedMember{
		{Slug: "tinycld", Name: "@tinycld/core", Version: "0.0.4", Integrity: "sha256:aa", FromCurrent: true},
		{Slug: "mail", Name: "@tinycld/mail", Version: "1.2.3", Integrity: "sha256:bb"},
	}
	if err := pkgbuild.WriteMembersLock(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := pkgbuild.ReadMembersLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0] != in[0] || out[1] != in[1] {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestReadMembersLock_AbsentIsNil(t *testing.T) {
	out, err := pkgbuild.ReadMembersLock(t.TempDir())
	if err != nil || out != nil {
		t.Fatalf("absent lock should read as (nil, nil), got (%+v, %v)", out, err)
	}
}

func TestLockedIntegrity(t *testing.T) {
	dir := t.TempDir()
	if err := pkgbuild.WriteMembersLock(dir, []pkgbuild.ResolvedMember{
		{Slug: "mail", Name: "@tinycld/mail", Version: "1.0.0", Integrity: "sha256:cc"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := pkgbuild.LockedIntegrity(dir, "mail"); got != "sha256:cc" {
		t.Fatalf("LockedIntegrity = %q, want sha256:cc", got)
	}
	if got := pkgbuild.LockedIntegrity(dir, "absent"); got != "" {
		t.Fatalf("absent member should yield empty integrity, got %q", got)
	}
	if got := pkgbuild.LockedIntegrity(t.TempDir(), "mail"); got != "" {
		t.Fatalf("absent lock should yield empty integrity, got %q", got)
	}
}

// ResolveMembers must read names/versions from what is ON DISK — the same
// resolution walk the peer-version verify uses — never from the manifest's
// possibly-floating spec strings.
func TestResolveMembers_ReadsDiskFacts(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.7")
	pkgbuildtest.WriteBuildMember(t, buildDir, "mail", "2.1.0", nil)

	m := pkgbuild.RebuildManifest{BuildID: "b", Members: []pkgbuild.MemberSpec{
		// Deliberately-wrong versions: resolution must not trust them.
		{Slug: pkgbuild.BaseMemberSlug, Version: "9.9.9", FromCurrent: true},
		{Slug: "mail", Version: "9.9.9", Spec: "@tinycld/mail@latest"},
	}}
	resolved, err := pkgbuild.ResolveMembers(m, buildDir, map[string]string{
		"tinycld": "sha256:base",
		"mail":    "sha256:mail",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 2 {
		t.Fatalf("resolved = %+v", resolved)
	}
	base, mail := resolved[0], resolved[1]
	if base.Name != pkgbuild.CorePackageKey || base.Version != "0.0.7" || base.Integrity != "sha256:base" || !base.FromCurrent {
		t.Fatalf("base = %+v", base)
	}
	if mail.Name != "@tinycld/mail" || mail.Version != "2.1.0" || mail.Integrity != "sha256:mail" || mail.FromCurrent {
		t.Fatalf("mail = %+v", mail)
	}
}

// A member whose manifest cannot be read fails resolution outright — an
// unidentifiable member must never produce a partially-described lock.
func TestResolveMembers_UnreadableManifestFails(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteBuildBase(t, buildDir, "0.0.7")
	m := pkgbuild.RebuildManifest{BuildID: "b", Members: []pkgbuild.MemberSpec{
		{Slug: pkgbuild.BaseMemberSlug, FromCurrent: true},
		{Slug: "mail"}, // dir never materialized
	}}
	if _, err := pkgbuild.ResolveMembers(m, buildDir, nil); err == nil {
		t.Fatal("expected resolution of an unreadable member to fail")
	}
}
