// Golden test for the recipe-hash canonical form.
//
// PAIRING RULE (same contract as the transpile goldens shared with the
// multi-org router): the fixture + golden constant below are duplicated
// byte-for-byte in multi-org/internal/recipeparity/recipehash_parity_test.go.
// If the canonical form changes (recipeFormatVersion bump), THIS golden goes
// red here and the stale twin goes red in multi-org's CI — fix by changing
// BOTH sides and regenerating BOTH goldens together, never one alone.
package pkgbuild_test

import (
	"fmt"
	"strings"
	"testing"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/pkgbuild/pkgbuildtest"
)

// goldenRecipeFixture is the frozen cross-repo fixture: three members
// including a third-party one (undistinguished by design), two overrides plus
// the "//"-style doc-key-free map shape, and a fixed toolchain.
func goldenRecipeFixture() ([]pkgbuild.ResolvedMember, map[string]string, pkgbuild.Toolchain) {
	members := []pkgbuild.ResolvedMember{
		{Slug: "tinycld", Name: "@tinycld/core", Version: "0.0.9", Integrity: "sha256:ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34ab12cd34", FromCurrent: true},
		{Slug: "mail", Name: "@tinycld/mail", Version: "0.3.1", Integrity: "sha256:cd34ef56cd34ef56cd34ef56cd34ef56cd34ef56cd34ef56cd34ef56cd34ef56"},
		{Slug: "todo", Name: "some-third-party-todo", Version: "2.1.0", Integrity: "sha256:ef56ab78ef56ab78ef56ab78ef56ab78ef56ab78ef56ab78ef56ab78ef56ab78"},
	}
	overrides := map[string]string{
		"uniwind":              "1.8.0",
		"@sentry/react-native": "7.11.0",
	}
	tc := pkgbuild.Toolchain{Go: "go1.26.3", Node: "v22.12.0", Pnpm: "pnpm@11.3.0+sha512.f00dfeedf00dfeed"}
	return members, overrides, tc
}

const goldenRecipeHash = "sha256:1e01f41d23eb4ffc9bd6c590bc7497b0d3ece1ad1a08df21e1f3d4628f361329"

func TestRecipeHash_MatchesGolden(t *testing.T) {
	members, overrides, tc := goldenRecipeFixture()
	got, err := pkgbuild.RecipeHash(members, overrides, tc)
	if err != nil {
		t.Fatal(err)
	}
	if got != goldenRecipeHash {
		t.Fatalf("recipe hash drifted from golden:\n  got  %s\n  want %s\nIf the canonical form changed intentionally, bump the format version and regenerate BOTH goldens (here and multi-org/internal/recipeparity).", got, goldenRecipeHash)
	}
}

func TestRecipeHash_OrderIndependent(t *testing.T) {
	members, overrides, tc := goldenRecipeFixture()
	permuted := []pkgbuild.ResolvedMember{members[2], members[0], members[1]}
	got, err := pkgbuild.RecipeHash(permuted, overrides, tc)
	if err != nil {
		t.Fatal(err)
	}
	if got != goldenRecipeHash {
		t.Fatalf("member order must not affect the hash: got %s", got)
	}
}

// Every input fact must be load-bearing: mutating any single field yields a
// different hash.
func TestRecipeHash_SensitiveToEachField(t *testing.T) {
	mutations := map[string]func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain){
		"member version":   func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { m[1].Version = "0.3.2" },
		"member integrity": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { m[1].Integrity = "sha256:0000" },
		"member name":      func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { m[2].Name = "renamed-pkg" },
		"override version": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { o["uniwind"] = "1.9.0" },
		"override added":   func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { o["extra-pin"] = "1.0.0" },
		"toolchain go":     func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { tc.Go = "go1.27.0" },
		"toolchain node":   func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { tc.Node = "v23.0.0" },
		"toolchain pnpm":   func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { tc.Pnpm = "pnpm@12.0.0+sha512.x" },
	}
	for label, mutate := range mutations {
		members, overrides, tc := goldenRecipeFixture()
		mutate(members, overrides, &tc)
		got, err := pkgbuild.RecipeHash(members, overrides, tc)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if got == goldenRecipeHash {
			t.Errorf("mutating %s did not change the hash", label)
		}
	}
}

// Incomplete or ambiguous input must refuse, never degrade — a silently
// partial hash would poison the shared build cache.
func TestRecipeHash_RefusesBadInput(t *testing.T) {
	cases := map[string]func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain){
		"empty integrity": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { m[0].Integrity = "" },
		"empty version":   func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { m[0].Version = "" },
		"empty name":      func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { m[0].Name = "" },
		"empty toolchain": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) { tc.Node = "" },
		"duplicate member": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) {
			m[2].Name = m[1].Name
		},
		"newline in version": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) {
			m[1].Version = "0.3.1\nmember forged@1.0.0 sha256:beef"
		},
		"space in override": func(m []pkgbuild.ResolvedMember, o map[string]string, tc *pkgbuild.Toolchain) {
			o["uniwind"] = "1.8.0 extra"
		},
	}
	for label, mutate := range cases {
		members, overrides, tc := goldenRecipeFixture()
		mutate(members, overrides, &tc)
		if _, err := pkgbuild.RecipeHash(members, overrides, tc); err == nil {
			t.Errorf("%s: expected refusal, got a hash", label)
		}
	}
	if _, err := pkgbuild.RecipeHash(nil, nil, pkgbuild.Toolchain{Go: "go1.26.3", Node: "v22.12.0", Pnpm: "pnpm@1"}); err == nil {
		t.Error("empty member set: expected refusal")
	}
}

func TestDetectToolchain_ParsesRunnerOutput(t *testing.T) {
	run := func(dir, name string, args ...string) (string, error) {
		switch name {
		case "go":
			return "go version go1.26.3 darwin/arm64\n", nil
		case "node":
			return "v22.12.0\n", nil
		}
		return "", fmt.Errorf("unexpected command %s", name)
	}
	tc, err := pkgbuild.DetectToolchain(run)
	if err != nil {
		t.Fatal(err)
	}
	if tc.Go != "go1.26.3" || tc.Node != "v22.12.0" {
		t.Fatalf("toolchain = %+v", tc)
	}
	if tc.Pnpm != pkgbuild.PackageManagerSpec {
		t.Fatalf("pnpm component must be the pinned PackageManagerSpec, got %q", tc.Pnpm)
	}
	if !strings.HasPrefix(tc.Pnpm, "pnpm@") {
		t.Fatalf("PackageManagerSpec shape changed: %q", tc.Pnpm)
	}
}

func TestDetectToolchain_RefusesGarbage(t *testing.T) {
	run := func(dir, name string, args ...string) (string, error) {
		return "not a version", nil
	}
	if _, err := pkgbuild.DetectToolchain(run); err == nil {
		t.Fatal("unparseable toolchain output must refuse")
	}
}

// RecipeHashForBuild reads what assemble recorded (members.lock.json + the
// overrides file) — the host-facing form of the hash.
func TestRecipeHashForBuild_FromAssembledDir(t *testing.T) {
	buildDir := t.TempDir()
	pkgbuildtest.WriteTestOverrides(t, buildDir)
	members, _, tc := goldenRecipeFixture()
	if err := pkgbuild.WriteMembersLock(buildDir, members); err != nil {
		t.Fatal(err)
	}
	got, err := pkgbuild.RecipeHashForBuild(buildDir, tc)
	if err != nil {
		t.Fatal(err)
	}
	want, err := pkgbuild.RecipeHash(members, map[string]string{
		"uniwind": "1.8.0", "@sentry/react-native": "7.11.0",
	}, tc)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("RecipeHashForBuild = %s, want %s", got, want)
	}

	// A member with unknown integrity (current build predating the lock)
	// refuses rather than producing a degraded key.
	members[0].Integrity = ""
	if err := pkgbuild.WriteMembersLock(buildDir, members); err != nil {
		t.Fatal(err)
	}
	if _, err := pkgbuild.RecipeHashForBuild(buildDir, tc); err == nil {
		t.Fatal("expected refusal for a lock entry without integrity")
	}

	// No lock at all → refuse.
	if _, err := pkgbuild.RecipeHashForBuild(t.TempDir(), tc); err == nil {
		t.Fatal("expected refusal when members.lock.json is absent")
	}
}
