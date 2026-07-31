// Package pkgbuildtest holds shared on-disk fixtures for tests that exercise
// the pkgbuild pipeline: fabricated members exactly as assemble leaves them in
// a build dir. Both pkgbuild's own tests and the coreserver host's wiring
// tests use these, so the fixture shape cannot drift between the two.
package pkgbuildtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tinycld.org/core/pkgbuild"
)

// WriteBuildMember fabricates a fetched member in the build dir: a manifest.ts
// whose object literal ParseManifestViaNode evaluates, exactly what assemble
// leaves at <buildDir>/<memberSlug>.
func WriteBuildMember(t *testing.T, buildDir, slug, version string, peerVersions map[string]string) {
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

// WriteTestOverrides drops a minimal package-versions.json into root so the
// scaffold writer (which reads it to emit the `overrides:` block) has its
// required source. A real build always carries this file (baked into the
// image, copied from the active build); tests must supply it explicitly.
func WriteTestOverrides(t *testing.T, root string) {
	t.Helper()
	body := `{"//":"doc","uniwind":"1.8.0","@sentry/react-native":"7.11.0"}`
	if err := os.WriteFile(filepath.Join(root, pkgbuild.OverridesFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// WriteBuildBase fabricates the tinycld base member, which ships no root
// manifest.ts — its version lives at core/package.json (the version peer
// ranges on @tinycld/core compare against).
func WriteBuildBase(t *testing.T, buildDir, coreVersion string) {
	t.Helper()
	dir := filepath.Join(buildDir, pkgbuild.BaseMemberSlug, "core")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pkg := fmt.Sprintf("{\n    \"name\": \"@tinycld/core\",\n    \"version\": \"%s\"\n}\n", coreVersion)
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(pkg), 0o644); err != nil {
		t.Fatal(err)
	}
}
