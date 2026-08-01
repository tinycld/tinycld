package pkgbuild_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/pkgbuild/pkgbuildtest"
)

// fakeSource is a MemberSource whose behaviors the test supplies inline.
type fakeSource struct {
	fetch func(ms pkgbuild.MemberSpec, buildDir string) (string, error)
	copy  func(ms pkgbuild.MemberSpec, buildDir string) (string, error)
}

func (s fakeSource) Fetch(ms pkgbuild.MemberSpec, buildDir string) (string, error) {
	return s.fetch(ms, buildDir)
}

func (s fakeSource) CopyCurrent(ms pkgbuild.MemberSpec, buildDir string) (string, error) {
	return s.copy(ms, buildDir)
}

// materializeMember stands in for a real fetch/copy: it writes the member
// content resolve later reads (the base's nested core/package.json, a feature
// member's manifest.ts) exactly as a real materialization would.
func materializeMember(t *testing.T, ms pkgbuild.MemberSpec, dir, version string) {
	t.Helper()
	if ms.Slug == pkgbuild.BaseMemberSlug {
		pkgbuildtest.WriteBuildBase(t, dir, version)
		return
	}
	pkgbuildtest.WriteBuildMember(t, dir, ms.Slug, version, nil)
}

func TestWriteWorkspaceScaffold(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	pkgbuildtest.WriteTestOverrides(t, src) // scaffold copies it into dir before YAML is written
	members := []string{"tinycld", "mail", "calc"}
	if err := pkgbuild.WriteWorkspaceScaffold(dir, members, src, pkgbuild.ScaffoldOptions{}); err != nil {
		t.Fatal(err)
	}

	// package.json present + parses + has packageManager + nested members.
	pj, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var pkg map[string]any
	if err := json.Unmarshal(pj, &pkg); err != nil {
		t.Fatalf("package.json invalid: %v", err)
	}
	if _, ok := pkg["packageManager"]; !ok {
		t.Fatal("package.json missing packageManager")
	}

	// pnpm-workspace.yaml lists every member (incl nested) + fixed store dir.
	ws, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(ws)
	for _, m := range []string{"tinycld", "tinycld/core", "tinycld/package-scripts", "mail", "calc"} {
		if !strings.Contains(s, m) {
			t.Fatalf("pnpm-workspace.yaml missing member %q", m)
		}
	}
	if !strings.Contains(s, "/workspace/.pnpm-store") {
		t.Fatal("pnpm-workspace.yaml missing storeDir /workspace/.pnpm-store")
	}
	if !strings.Contains(s, "nodeLinker: hoisted") {
		t.Fatal("pnpm-workspace.yaml missing nodeLinker: hoisted")
	}
	// The pins from package-versions.json are transcribed into the `overrides:`
	// block — without this the OTA rebuild's --no-frozen-lockfile install drifts
	// uniwind/tailwind off the embedded binary. The doc `//` key must NOT appear.
	if !strings.Contains(s, "\noverrides:\n") {
		t.Fatal("pnpm-workspace.yaml missing overrides: block")
	}
	if !strings.Contains(s, "  uniwind: 1.8.0\n") {
		t.Fatal("pnpm-workspace.yaml overrides missing uniwind pin")
	}
	if !strings.Contains(s, "  '@sentry/react-native': 7.11.0\n") {
		t.Fatal("pnpm-workspace.yaml overrides missing quoted scoped pin")
	}
	if strings.Contains(s, "//") {
		t.Fatal("pnpm-workspace.yaml leaked the // doc key from package-versions.json")
	}
}

func TestWriteWorkspaceScaffold_CustomStoreDir(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir()
	pkgbuildtest.WriteTestOverrides(t, src)
	if err := pkgbuild.WriteWorkspaceScaffold(dir, []string{"tinycld"}, src, pkgbuild.ScaffoldOptions{PnpmStoreDir: "/builder/store"}); err != nil {
		t.Fatal(err)
	}
	ws, err := os.ReadFile(filepath.Join(dir, "pnpm-workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ws), "storeDir: /builder/store\n") {
		t.Fatal("ScaffoldOptions.PnpmStoreDir not honored")
	}
}

func TestAssembleBuild_FetchesAllAndScaffolds(t *testing.T) {
	build := t.TempDir()
	srcRoot := t.TempDir()
	pkgbuildtest.WriteTestOverrides(t, srcRoot)
	manifest := pkgbuild.RebuildManifest{
		BuildID: "build-1",
		Members: []pkgbuild.MemberSpec{
			{Slug: "tinycld", Spec: "git+file:///x/tinycld"},
			{Slug: "mail", Spec: "@tinycld/mail@1"},
		},
	}
	var fetched []string
	src := fakeSource{
		fetch: func(ms pkgbuild.MemberSpec, dir string) (string, error) {
			fetched = append(fetched, ms.Slug)
			materializeMember(t, ms, dir, "1.0.0")
			return "sha256:fake-" + ms.Slug, nil
		},
		copy: func(ms pkgbuild.MemberSpec, dir string) (string, error) {
			return "", fmt.Errorf("copy should not be called when no member is FromCurrent")
		},
	}
	if err := pkgbuild.AssembleBuild(nil, manifest, build, src, srcRoot, pkgbuild.ScaffoldOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(fetched) != 2 {
		t.Fatalf("expected 2 fetches, got %v", fetched)
	}

	// The assemble records the resolved set: on-disk versions + the integrities
	// the source reported.
	locked, err := pkgbuild.ReadMembersLock(build)
	if err != nil {
		t.Fatal(err)
	}
	if len(locked) != 2 {
		t.Fatalf("members.lock.json should list both members, got %+v", locked)
	}
	bySlug := map[string]pkgbuild.ResolvedMember{}
	for _, rm := range locked {
		bySlug[rm.Slug] = rm
	}
	if bySlug["mail"].Integrity != "sha256:fake-mail" || bySlug["mail"].Name != "@tinycld/mail" {
		t.Fatalf("mail lock entry = %+v", bySlug["mail"])
	}
	if bySlug["tinycld"].Name != pkgbuild.CorePackageKey || bySlug["tinycld"].Version != "1.0.0" {
		t.Fatalf("base lock entry = %+v", bySlug["tinycld"])
	}
}

func TestAssembleBuild_CopiesFromCurrentVsFetch(t *testing.T) {
	build := t.TempDir()
	srcRoot := t.TempDir()
	manifest := pkgbuild.RebuildManifest{
		BuildID: "build-2",
		Members: []pkgbuild.MemberSpec{
			{Slug: "tinycld", Spec: "github:tinycld/tinycld", FromCurrent: true}, // unchanged → copy
			{Slug: "mail", Spec: "@tinycld/mail@2"},                              // changed → fetch
		},
	}
	var fetched, copied []string
	src := fakeSource{
		fetch: func(ms pkgbuild.MemberSpec, dir string) (string, error) {
			fetched = append(fetched, ms.Slug)
			materializeMember(t, ms, dir, "2.0.0")
			return "sha256:fresh", nil
		},
		copy: func(ms pkgbuild.MemberSpec, dir string) (string, error) {
			copied = append(copied, ms.Slug)
			materializeMember(t, ms, dir, "0.0.9")
			// The unchanged "tinycld" member, copied from the current build, carries
			// the workspace-root override file the scaffold writer reads.
			pkgbuildtest.WriteTestOverrides(t, dir)
			// Carried forward from the active build's lock.
			return "sha256:carried", nil
		},
	}
	if err := pkgbuild.AssembleBuild(nil, manifest, build, src, srcRoot, pkgbuild.ScaffoldOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(fetched) != 1 || fetched[0] != "mail" {
		t.Fatalf("expected only mail fetched, got %v", fetched)
	}
	if len(copied) != 1 || copied[0] != "tinycld" {
		t.Fatalf("expected only tinycld copied from current, got %v", copied)
	}
	if _, err := os.Stat(filepath.Join(build, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(build, "pnpm-workspace.yaml")); err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}

	// The copied member's carried-forward integrity lands in the lock.
	if got := pkgbuild.LockedIntegrity(build, "tinycld"); got != "sha256:carried" {
		t.Fatalf("carried integrity = %q, want sha256:carried", got)
	}
	if got := pkgbuild.LockedIntegrity(build, "mail"); got != "sha256:fresh" {
		t.Fatalf("fetched integrity = %q, want sha256:fresh", got)
	}
}

func TestFetchMember_PlacesExtractedDirAndReturnsIntegrity(t *testing.T) {
	build := t.TempDir()
	// Fake an already-extracted "package" dir as if npm pack + untar ran.
	fakeExtract := t.TempDir()
	pkgDir := filepath.Join(fakeExtract, "package")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "manifest.ts"), []byte("export default {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	packer := func(spec, into string) (string, string, error) {
		return pkgDir, "sha256:abc123", nil // pretend we packed+hashed+untarred
	}
	integrity, err := pkgbuild.FetchMemberWith(pkgbuild.MemberSpec{Slug: "mail", Spec: "@tinycld/mail@0.3.1"}, build, packer)
	if err != nil {
		t.Fatal(err)
	}
	if integrity != "sha256:abc123" {
		t.Fatalf("integrity = %q, want the packer's hash", integrity)
	}
	if _, err := os.Stat(filepath.Join(build, "mail", "manifest.ts")); err != nil {
		t.Fatalf("expected build/mail/manifest.ts: %v", err)
	}
}

func TestNpmPackSource_CopyCurrentRefuses(t *testing.T) {
	_, err := pkgbuild.NpmPackSource().CopyCurrent(pkgbuild.MemberSpec{Slug: "mail"}, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "mail") {
		t.Fatalf("fetch-only source must refuse CopyCurrent, got: %v", err)
	}
}

func TestCopyScaffoldExtras_CopiesFilesAndDirs(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	// One file extra and one dir extra.
	if err := os.WriteFile(filepath.Join(src, ".npmrc"), []byte("x=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "scripts", "link-members.ts"), []byte("// x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := pkgbuild.CopyScaffoldExtras(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".npmrc")); err != nil {
		t.Fatalf(".npmrc not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "scripts", "link-members.ts")); err != nil {
		t.Fatalf("scripts/ not copied: %v", err)
	}
}
