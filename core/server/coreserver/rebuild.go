package coreserver

// MemberSpec describes one workspace member to assemble into a build.
// Spec is the fetch spec passed to `npm pack` — an npm name+range
// (@tinycld/mail@0.3.1), a git URL (git+https://…#tag), or a local
// git+file:// remote (used by the integration test). Every member,
// including the tinycld app shell + core, is fetched this way.
type MemberSpec struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Spec    string `json:"spec"`
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
