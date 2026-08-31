package pkgbuild

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// RecipeHash is the single definition of the build-cache key
// (hosting/docs/DESIGN-org-package-agency.md, D4/D5): two orgs whose
// resolved package sets, version pins, and toolchains match hit the same
// artifact by construction, on whichever host computes the hash.
//
// The hash covers RESOLVED FACTS ONLY — never floating fetch specs (a git
// tag or `latest` resolves differently over time; identity is the resolved
// bytes). Deliberately excluded, documented so the exclusions stay decisions
// rather than accidents:
//   - MemberSpec.Spec — floats; the tarball integrity IS the resolved spec.
//   - Slug — derived from the manifest (i.e. from the hashed tarball), and
//     keeping it out keeps World A's core/tinycld slug duality out of the key.
//   - FromCurrent — a World-A materialization strategy, not identity; the
//     builder always fetches.
//   - Build target — D5's dual-mode binary decision: one artifact serves both
//     host and tenant, so the recipe needs no target dimension.
//
// Canonical form v1 (hand-rolled sorted lines, newline-terminated — language-
// portable and diff-readable when a golden test fails):
//
//	tinycld-recipe/v1
//	go go1.26.3
//	node v22.12.0
//	pnpm pnpm@11.3.0+sha512…
//	override <name> <version>     (sorted by name; the "//" doc key never appears)
//	member <name>@<version> <integrity>   (sorted; third-party undistinguished)
//
// Changing the canonical form REQUIRES bumping recipeFormatVersion and
// regenerating BOTH golden tests (pkgbuild/recipehash_test.go and hosting's
// internal/recipeparity) together.
const recipeFormatVersion = "tinycld-recipe/v1"

// Toolchain pins the tool versions that shape build output. All three fields
// are required by RecipeHash: an unknown toolchain must never alias two
// different build environments to one cache key.
type Toolchain struct {
	Go   string // "go1.26.3" — the PATH toolchain that compiles the workspace
	Node string // "v22.12.0"
	Pnpm string // the full PackageManagerSpec (version + integrity)
}

// DetectToolchain resolves the toolchain the build will actually use: `go
// version` / `node --version` from PATH through the given runner (so the
// builder can run it inside its jail and tests can stub it; nil means RunCmd),
// and the pinned PackageManagerSpec for pnpm — the assembled root pins the
// spec, integrity included, which is more precise than `pnpm --version`.
//
// The Go version is read from PATH, NOT runtime.Version(): the builder
// binary's own compiler is irrelevant to what compiles the workspace.
func DetectToolchain(run CmdRunner) (Toolchain, error) {
	if run == nil {
		run = RunCmd
	}
	goOut, err := run(".", "go", "version")
	if err != nil {
		return Toolchain{}, fmt.Errorf("detect go version: %w", err)
	}
	// "go version go1.26.3 darwin/arm64" → third field.
	goFields := strings.Fields(goOut)
	if len(goFields) < 3 || !strings.HasPrefix(goFields[2], "go") {
		return Toolchain{}, fmt.Errorf("unrecognized `go version` output: %q", strings.TrimSpace(goOut))
	}
	nodeOut, err := run(".", "node", "--version")
	if err != nil {
		return Toolchain{}, fmt.Errorf("detect node version: %w", err)
	}
	nodeVer := strings.TrimSpace(nodeOut)
	if !strings.HasPrefix(nodeVer, "v") {
		return Toolchain{}, fmt.Errorf("unrecognized `node --version` output: %q", nodeVer)
	}
	return Toolchain{Go: goFields[2], Node: nodeVer, Pnpm: PackageManagerSpec}, nil
}

// RecipeHash computes "sha256:<hex>" over the canonical form of the resolved
// member set, the version-pin overrides (package-versions.json's pins), and
// the toolchain.
//
// It REFUSES incomplete or ambiguous input instead of degrading: an empty
// name/version/integrity/toolchain field, a duplicate member name, or a token
// containing whitespace (which could forge canonical-form line boundaries)
// is an error. A silently-partial hash would poison the shared build cache —
// wrong-key reuse is strictly worse than a failed hash.
func RecipeHash(members []ResolvedMember, overrides map[string]string, tc Toolchain) (string, error) {
	if err := checkToken("toolchain go", tc.Go); err != nil {
		return "", err
	}
	if err := checkToken("toolchain node", tc.Node); err != nil {
		return "", err
	}
	if err := checkToken("toolchain pnpm", tc.Pnpm); err != nil {
		return "", err
	}
	if len(members) == 0 {
		return "", fmt.Errorf("recipe hash: empty member set")
	}

	var sb strings.Builder
	sb.WriteString(recipeFormatVersion + "\n")
	sb.WriteString("go " + tc.Go + "\n")
	sb.WriteString("node " + tc.Node + "\n")
	sb.WriteString("pnpm " + tc.Pnpm + "\n")

	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := checkToken("override name", name); err != nil {
			return "", err
		}
		if err := checkToken(fmt.Sprintf("override %s version", name), overrides[name]); err != nil {
			return "", err
		}
		sb.WriteString("override " + name + " " + overrides[name] + "\n")
	}

	seen := map[string]bool{}
	lines := make([]string, 0, len(members))
	for _, m := range members {
		if err := checkToken("member name", m.Name); err != nil {
			return "", err
		}
		if err := checkToken(fmt.Sprintf("member %s version", m.Name), m.Version); err != nil {
			return "", err
		}
		if err := checkToken(fmt.Sprintf("member %s integrity", m.Name), m.Integrity); err != nil {
			return "", err
		}
		if seen[m.Name] {
			return "", fmt.Errorf("recipe hash: duplicate member %q", m.Name)
		}
		seen[m.Name] = true
		lines = append(lines, "member "+m.Name+"@"+m.Version+" "+m.Integrity+"\n")
	}
	sort.Strings(lines)
	for _, l := range lines {
		sb.WriteString(l)
	}

	sum := sha256.Sum256([]byte(sb.String()))
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// checkToken rejects an empty token or one containing whitespace/control
// characters — any of which could collapse two distinct recipes onto one
// canonical form (line-boundary forgery) or hide a missing fact.
func checkToken(kind, v string) error {
	if v == "" {
		return fmt.Errorf("recipe hash: %s is empty", kind)
	}
	if strings.ContainsAny(v, " \t\n\r") {
		return fmt.Errorf("recipe hash: %s %q contains whitespace", kind, v)
	}
	return nil
}

// RecipeHashForBuild computes the recipe hash of an ASSEMBLED build dir from
// what assemble recorded: members.lock.json and the overrides file. This is
// the form hosts use — the single-tenant breadcrumb log today, the builder's
// cache key at publish time.
func RecipeHashForBuild(buildDir string, tc Toolchain) (string, error) {
	members, err := ReadMembersLock(buildDir)
	if err != nil {
		return "", err
	}
	if members == nil {
		return "", fmt.Errorf("recipe hash: %s has no %s", buildDir, MembersLockFile)
	}
	overrides, err := readOverrides(buildDir)
	if err != nil {
		return "", err
	}
	return RecipeHash(members, overrides, tc)
}
