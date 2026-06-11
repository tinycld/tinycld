package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteWorkspaceScaffold(t *testing.T) {
	dir := t.TempDir()
	src := t.TempDir() // empty src root → scaffold extras skipped
	members := []string{"tinycld", "mail", "calc"}
	if err := writeWorkspaceScaffoldFrom(dir, members, src); err != nil {
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
}

func TestAssembleBuild_FetchesAllAndScaffolds(t *testing.T) {
	build := t.TempDir()
	manifest := RebuildManifest{
		BuildID: "build-1",
		Members: []MemberSpec{
			{Slug: "tinycld", Spec: "git+file:///x/tinycld"},
			{Slug: "mail", Spec: "@tinycld/mail@1"},
		},
	}
	var fetched []string
	fakeFetch := func(ms MemberSpec, dir string) error {
		fetched = append(fetched, ms.Slug)
		return os.MkdirAll(filepath.Join(dir, ms.Slug), 0o755)
	}
	if err := assembleBuildWith(manifest, build, fakeFetch); err != nil {
		t.Fatal(err)
	}
	if len(fetched) != 2 {
		t.Fatalf("expected 2 fetches, got %v", fetched)
	}
	if _, err := os.Stat(filepath.Join(build, "manifest.json")); err != nil {
		t.Fatalf("manifest.json not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(build, "pnpm-workspace.yaml")); err != nil {
		t.Fatalf("scaffold not written: %v", err)
	}
}

func TestFetchMember_PlacesExtractedDir(t *testing.T) {
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

	packer := func(spec, into string) (string, error) {
		return pkgDir, nil // pretend we packed+untarred
	}
	if err := fetchMemberWith(MemberSpec{Slug: "mail", Spec: "@tinycld/mail@0.3.1"}, build, packer); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(build, "mail", "manifest.ts")); err != nil {
		t.Fatalf("expected build/mail/manifest.ts: %v", err)
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
	if err := copyScaffoldExtras(src, dst); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, ".npmrc")); err != nil {
		t.Fatalf(".npmrc not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "scripts", "link-members.ts")); err != nil {
		t.Fatalf("scripts/ not copied: %v", err)
	}
}
