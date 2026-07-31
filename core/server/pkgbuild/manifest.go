package pkgbuild

import (
	"encoding/json"
	"os"
)

// MemberSpec describes one workspace member to assemble into a build.
// Spec is the fetch spec passed to `npm pack` — an npm name+range
// (@tinycld/mail@0.3.1), a git URL (git+https://…#tag), or a local
// git+file:// remote (used by the integration test).
//
// FromCurrent marks a member that should be COPIED from the currently-active
// build rather than re-fetched. Only the member(s) a delta actually changes are
// fetched fresh; everything else is copied from the live build so an install of
// one package can't silently re-resolve another member's spec to a drifted
// remote state (e.g. re-fetching the tinycld base from github HEAD, which may be
// behind the running base — and would drop migrations the running base ships).
type MemberSpec struct {
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	Spec        string `json:"spec"`
	FromCurrent bool   `json:"fromCurrent,omitempty"`
}

// RebuildManifest is the complete desired package set for one build. It is
// written verbatim to builds/<id>/manifest.json before the build runs and
// serves as the build's input AND its rollback record.
type RebuildManifest struct {
	BuildID string       `json:"buildId"`
	Members []MemberSpec `json:"members"`
}

// MemberBySlug returns the member spec for slug, if present.
func (m RebuildManifest) MemberBySlug(slug string) (MemberSpec, bool) {
	for _, ms := range m.Members {
		if ms.Slug == slug {
			return ms, true
		}
	}
	return MemberSpec{}, false
}

// The base platform is the `tinycld` workspace member, but its pkg_registry row
// (and the /admin UI delta) uses the historical slug "core". These map between
// the two namespaces so the desired set always speaks member slugs while the
// registry keeps its slug.
const BaseRegistrySlug = "core"
const BaseMemberSlug = "tinycld"

// RegistrySlugToMember maps a registry slug to its workspace-member slug.
func RegistrySlugToMember(slug string) string {
	if slug == BaseRegistrySlug {
		return BaseMemberSlug
	}
	return slug
}

// MemberSlugToRegistry maps a workspace-member slug to its registry slug.
func MemberSlugToRegistry(slug string) string {
	if slug == BaseMemberSlug {
		return BaseRegistrySlug
	}
	return slug
}

// PackageJSONVersion reads the "version" field from a package.json, or "" on
// any error. A small, dependency-free read (no node subprocess).
func PackageJSONVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Version
}

// PackageJSONName reads the "name" field from a package.json, or "" on any
// error. This — not the manifest's display `name` ("Mail") — is a member's
// npm identity (@tinycld/mail): the org-lockfile key, the version-discovery
// source, and the ResolvedMember.Name contract.
func PackageJSONName(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Name
}
