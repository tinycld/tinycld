package coreserver

import (
	"fmt"
	"os"
	"path/filepath"

	"tinycld.org/core/pkgbuild"
)

// The assemble step itself lives in pkgbuild/assemble.go. What stays here is
// the single-tenant host's MemberSource: fetch via npm pack, and — the part
// only this host can do — copy unchanged members out of the currently-active
// build.

// currentWorkspaceRoot returns the workspace root of the running build: the
// binary lives at <root>/tinycld/tinycld, so resolveServerDir() == <root>/tinycld
// and its parent is the root.
func currentWorkspaceRoot() string {
	return filepath.Dir(resolveServerDir())
}

// hostMemberSource is the single-tenant MemberSource: changed members are
// fetched with the standard npm-pack source, unchanged (FromCurrent) members
// are copied from the live build.
type hostMemberSource struct{}

var _ pkgbuild.MemberSource = hostMemberSource{}

func (hostMemberSource) Fetch(ms MemberSpec, buildDir string) error {
	return pkgbuild.NpmPackSource().Fetch(ms, buildDir)
}

func (hostMemberSource) CopyCurrent(ms MemberSpec, buildDir string) error {
	return copyMemberFromCurrent(ms, buildDir)
}

// copyMemberFromCurrent copies an unchanged member from the live build's
// workspace root (the parent of resolveServerDir(), i.e. /workspace/current/..)
// into buildDir. This keeps unchanged members byte-identical to what's running
// instead of re-fetching (which could drift their spec below the running
// version and drop migrations the running build ships).
func copyMemberFromCurrent(ms MemberSpec, buildDir string) error {
	src := filepath.Join(currentWorkspaceRoot(), ms.Slug)
	if _, err := os.Stat(src); err != nil {
		// The current build doesn't carry this member (shouldn't happen for an
		// unchanged member). Surface it rather than silently producing a broken
		// build — the caller fails the rebuild.
		return fmt.Errorf("current build missing member %q at %s: %w", ms.Slug, src, err)
	}
	dest := filepath.Join(buildDir, ms.Slug)
	if err := os.RemoveAll(dest); err != nil {
		return err
	}
	return copyDir(src, dest)
}

// assembleBuild materializes the desired member set into buildDir. Kept as the
// host-side wrapper so the production wiring reads at one glance: the job's
// SSE/install-log sink, the host MemberSource, and the active root as the
// scaffold source.
func assembleBuild(job *installJob, m RebuildManifest, buildDir string) error {
	return pkgbuild.AssembleBuild(installJobSink{job}, m, buildDir,
		hostMemberSource{}, currentWorkspaceRoot(), pkgbuild.ScaffoldOptions{})
}
