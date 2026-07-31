package pkgbuild

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// Compatibility solver (the pure half — the DB-backed pre-flight gates live in
// the coreserver host).
//
// A version change is applied as a SET: the operator selects several packages
// and target versions and applies them together. Each package version declares
// peerVersions — semver ranges it requires of other packages and @tinycld/core.
// The solver resolves the proposed set against the versions that will be live
// after the change (proposed targets override, untouched packages keep their
// current version) and reports every declared range the resolved set violates.
//
// peerVersions can differ between versions of the same package. The host's
// pre-flight gates resolve peerVersions from the CURRENTLY installed manifests,
// so a target version that tightens its OWN requirements relative to its
// installed manifest is invisible to them. VerifyTargetPeerVersions closes that
// gap: after assemble fetches the real files, the rebuild re-runs the solve
// against the manifests on disk in the build dir and refuses before the build
// pipeline runs (build dir discarded, live state untouched). The pre-flight
// gate remains the friendly synchronous refusal for what it can see; the
// post-assemble verify is the guarantee.

// CorePackageKey is the peerVersions key packages use to constrain the core
// platform's version.
const CorePackageKey = "@tinycld/core"

// Violation is one unsatisfied peer requirement.
type Violation struct {
	Package  string `json:"package"`  // the package whose peerVersions declared the requirement
	Requires string `json:"requires"` // the peer slug/@tinycld/core that must match
	Range    string `json:"range"`    // the declared semver range
	Found    string `json:"found"`    // the version the resolved set provides (or "" if absent)
}

// SolveCompat checks every entry of every package's peerVersions against the
// resolved version map. resolved maps package slug (and CorePackageKey) to the
// version that will be live. peerVersionsBySlug maps a package slug to its
// declared peerVersions. A requirement on a package absent from resolved is a
// violation with Found "". Returns violations sorted for stable output.
func SolveCompat(
	resolved map[string]string,
	peerVersionsBySlug map[string]map[string]string,
) []Violation {
	violations := []Violation{}

	slugs := make([]string, 0, len(peerVersionsBySlug))
	for slug := range peerVersionsBySlug {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		peers := peerVersionsBySlug[slug]
		peerKeys := make([]string, 0, len(peers))
		for k := range peers {
			peerKeys = append(peerKeys, k)
		}
		sort.Strings(peerKeys)

		for _, peerKey := range peerKeys {
			rangeStr := peers[peerKey]
			constraint, err := semver.NewConstraint(rangeStr)
			if err != nil {
				// An unparsable declared range is itself a violation — surface it
				// rather than silently passing.
				violations = append(violations, Violation{
					Package:  slug,
					Requires: peerKey,
					Range:    rangeStr,
					Found:    "",
				})
				continue
			}
			found, present := resolved[peerKey]
			if !present {
				violations = append(violations, Violation{
					Package: slug, Requires: peerKey, Range: rangeStr, Found: "",
				})
				continue
			}
			foundVer, verErr := semver.NewVersion(found)
			if verErr != nil || !constraint.Check(foundVer) {
				violations = append(violations, Violation{
					Package: slug, Requires: peerKey, Range: rangeStr, Found: found,
				})
			}
		}
	}
	return violations
}

// VerifyTargetPeerVersions re-runs the peer-version solve against what
// assemble actually fetched into the build dir, reading every member's
// manifest fresh from disk. This is the authoritative post-fetch check: the
// pre-flight gates see only the installed manifests, so a target version that
// tightens its own requirements — or an uninstall that strands a remaining
// package's declared peer (the uninstall path runs no pre-flight solve at
// all) — is caught only here. A member whose manifest cannot be read fails
// the verify outright: requirements we cannot see are not requirements we can
// vouch for.
//
// The base (tinycld) member is special: it ships no root manifest.ts, and the
// version peer ranges on @tinycld/core compare against is CORE's — the nested
// tinycld/core/package.json — mirroring the host's changedMemberVersion.
func VerifyTargetPeerVersions(m RebuildManifest, buildDir string) error {
	members, err := readBuildMembers(m, buildDir)
	if err != nil {
		return fmt.Errorf("verify peer versions: %w", err)
	}
	resolved := map[string]string{}
	peerVersionsBySlug := map[string]map[string]string{}
	for _, bm := range members {
		regSlug := MemberSlugToRegistry(bm.Slug)
		resolved[regSlug] = bm.Version
		if len(bm.PeerVersions) > 0 {
			peerVersionsBySlug[regSlug] = bm.PeerVersions
		}
	}
	if coreVer, ok := resolved[BaseRegistrySlug]; ok {
		resolved[CorePackageKey] = coreVer
	}
	return ViolationsError(SolveCompat(resolved, peerVersionsBySlug))
}

// PeerVersionsFromManifest extracts the peerVersions map from a stored
// manifest_json blob. Returns nil if absent or malformed.
func PeerVersionsFromManifest(manifestJSON string) map[string]string {
	if manifestJSON == "" {
		return nil
	}
	var parsed struct {
		PeerVersions map[string]string `json:"peerVersions"`
	}
	if err := json.Unmarshal([]byte(manifestJSON), &parsed); err != nil {
		return nil
	}
	return parsed.PeerVersions
}

// ViolationsError renders violations into a single human-readable error for
// pipeline failures (the UI uses the structured list; the pipeline surfaces
// prose). Returns nil for an empty slice so callers can
// `return ViolationsError(v)` unconditionally.
func ViolationsError(violations []Violation) error {
	if len(violations) == 0 {
		return nil
	}
	lines := make([]string, 0, len(violations))
	for _, v := range violations {
		found := v.Found
		if found == "" {
			found = "not installed"
		}
		lines = append(lines, fmt.Sprintf(
			"%s requires %s %s (found: %s)", v.Package, v.Requires, v.Range, found,
		))
	}
	return fmt.Errorf(
		"package compatibility check failed:\n  %s", strings.Join(lines, "\n  "),
	)
}
