package pkgbuild

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MembersLockFile records, at the build-dir root, the RESOLVED member set an
// assemble actually produced: the npm package name and semver read from the
// on-disk manifests (never the possibly-floating fetch specs), plus the
// integrity of the exact tarball bytes each fetched member came from. It is
// the raw material for RecipeHash and the carry-forward source for
// FromCurrent members' integrity on the next build.
const MembersLockFile = "members.lock.json"

// ResolvedMember is one workspace member of a completed assemble, as resolved
// facts.
type ResolvedMember struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`      // npm package name (@tinycld/mail); the base records @tinycld/core
	Version     string `json:"version"`   // semver read from the build dir, not the delta's target string
	Integrity   string `json:"integrity"` // "sha256:<hex>" of the fetched tarball; "" when unknown (pre-lock current build)
	FromCurrent bool   `json:"fromCurrent,omitempty"`
}

type membersLock struct {
	Members []ResolvedMember `json:"members"`
}

// WriteMembersLock writes the resolved member set to
// <buildDir>/members.lock.json.
func WriteMembersLock(buildDir string, members []ResolvedMember) error {
	b, err := json.MarshalIndent(membersLock{Members: members}, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(buildDir, MembersLockFile), append(b, '\n'), 0o644)
}

// ReadMembersLock reads <root>/members.lock.json. A missing file returns
// (nil, nil): every build predating the lock's introduction lacks one, and
// callers treat that as "integrity unknown" (RecipeHash then refuses, which
// is the intended fail-closed behavior).
func ReadMembersLock(root string) ([]ResolvedMember, error) {
	raw, err := os.ReadFile(filepath.Join(root, MembersLockFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var lock membersLock
	if err := json.Unmarshal(raw, &lock); err != nil {
		return nil, fmt.Errorf("parse %s: %w", MembersLockFile, err)
	}
	return lock.Members, nil
}

// LockedIntegrity returns the recorded integrity of one member in
// <root>/members.lock.json, or "" when the lock or the member is absent —
// the carry-forward read a host performs for FromCurrent members.
func LockedIntegrity(root, slug string) string {
	members, err := ReadMembersLock(root)
	if err != nil {
		return ""
	}
	for _, m := range members {
		if m.Slug == slug {
			return m.Integrity
		}
	}
	return ""
}

// buildMember is the shared result of reading one member out of an assembled
// build dir — the resolution walk VerifyTargetPeerVersions and ResolveMembers
// both perform, kept in one place so the compat gate and the recipe identity
// can never disagree about what a build contains.
type buildMember struct {
	ResolvedMember
	PeerVersions map[string]string
}

// readBuildMembers resolves every manifest member against what is actually on
// disk in buildDir. The base (tinycld) member is special: it ships no root
// manifest.ts, and its version is CORE's — the nested tinycld/core/package.json
// — recorded under the name @tinycld/core (the key peer ranges constrain). A
// member whose manifest cannot be read is a hard error: facts we cannot read
// are not facts we can vouch for.
func readBuildMembers(m RebuildManifest, buildDir string) ([]buildMember, error) {
	out := make([]buildMember, 0, len(m.Members))
	for _, ms := range m.Members {
		if ms.Slug == BaseMemberSlug {
			version := PackageJSONVersion(filepath.Join(buildDir, BaseMemberSlug, "core", "package.json"))
			if version == "" {
				version = ms.Version
			}
			out = append(out, buildMember{ResolvedMember: ResolvedMember{
				Slug: ms.Slug, Name: CorePackageKey, Version: version, FromCurrent: ms.FromCurrent,
			}})
			continue
		}
		manifest, err := ParseManifestViaNode(filepath.Join(buildDir, ms.Slug))
		if err != nil {
			return nil, fmt.Errorf("read %s's manifest from the build: %w", ms.Slug, err)
		}
		version := manifest.Version
		if version == "" {
			version = ms.Version
		}
		// Name is the member's NPM identity from its package.json — the
		// manifest's `name` is a display string ("Mail"), which an earlier
		// revision recorded here and which broke every consumer that treats
		// Name as the lockfile/npm key. Every npm tarball (and workspace
		// member) carries a named package.json; one we cannot read is not a
		// member we can identify.
		npmName := PackageJSONName(filepath.Join(buildDir, ms.Slug, "package.json"))
		if npmName == "" {
			return nil, fmt.Errorf("member %s has no readable package.json name — cannot establish its npm identity", ms.Slug)
		}
		out = append(out, buildMember{
			ResolvedMember: ResolvedMember{
				Slug: ms.Slug, Name: npmName, Version: version, FromCurrent: ms.FromCurrent,
			},
			PeerVersions: manifest.PeerVersions,
		})
	}
	return out, nil
}

// ResolveMembers reads the resolved member set out of an assembled build dir,
// attaching the per-slug integrities the MemberSource reported during
// assemble. Members without a reported integrity record "".
func ResolveMembers(m RebuildManifest, buildDir string, integrities map[string]string) ([]ResolvedMember, error) {
	members, err := readBuildMembers(m, buildDir)
	if err != nil {
		return nil, fmt.Errorf("resolve members: %w", err)
	}
	out := make([]ResolvedMember, 0, len(members))
	for _, bm := range members {
		rm := bm.ResolvedMember
		rm.Integrity = integrities[rm.Slug]
		out = append(out, rm)
	}
	return out, nil
}
