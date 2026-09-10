package jsvm

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/pocketbase/pocketbase/tests"
)

// TestFilesContentFS_ReadsAndTransforms verifies the fs.FS loader matches
// filesContent's contract: pattern filtering, directory skipping, esbuild
// transformation, and base-filename keys.
func TestFilesContentFS_ReadsAndTransforms(t *testing.T) {
	fsys := fstest.MapFS{
		"a.js":        {Data: []byte("migrate((app) => {})")},
		"b.js":        {Data: []byte("migrate((app) => {})")},
		"skip.txt":    {Data: []byte("not a migration")},
		"sub/deep.js": {Data: []byte("migrate((app) => {})")},
	}

	got, err := filesContentFS(fsys, `^.*(\.js|\.ts)$`)
	if err != nil {
		t.Fatalf("filesContentFS: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 files (a.js, b.js), got %d: %v", len(got), keysOf(got))
	}
	for _, name := range []string{"a.js", "b.js"} {
		if len(got[name]) == 0 {
			t.Errorf("expected transformed content for %q", name)
		}
	}
	if _, ok := got["skip.txt"]; ok {
		t.Error("pattern must exclude skip.txt")
	}
	if _, ok := got["deep.js"]; ok {
		t.Error("loader must not recurse into subdirectories")
	}
}

// TestFilesContentFS_MissingDirIsEmpty mirrors filesContent's behavior of
// treating a missing directory as "no files" rather than an error.
func TestFilesContentFS_MissingDirIsEmpty(t *testing.T) {
	got, err := filesContentFS(fstest.MapFS{}, `^.*(\.js|\.ts)$`)
	if err != nil {
		t.Fatalf("expected nil error for empty FS, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 files, got %d", len(got))
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestRegisterHooks_HooksFSNeverWritesToDisk pins the one behavior that cannot
// work against an embedded FS: registerHooks normally writes a types-reference
// directive into EMPTY hook files. With HooksFS set, HooksDir is empty, so that
// path would resolve names against the process working directory and clobber an
// unrelated same-named empty file. The guard must skip the prepend entirely.
func TestRegisterHooks_HooksFSNeverWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	// A same-named empty file in the working directory is what the unguarded
	// prepend would overwrite.
	bystander := filepath.Join(dir, "a.pb.ts")
	if err := os.WriteFile(bystander, nil, 0o644); err != nil {
		t.Fatalf("seed bystander: %v", err)
	}

	p := &plugin{app: testApp(t), config: Config{
		// The embedded file is empty on purpose: a non-empty one is skipped by
		// the prepend loop anyway, so only an empty one can prove the guard.
		HooksFS:           fstest.MapFS{"a.pb.ts": {Data: []byte("")}},
		HooksFilesPattern: `^.*(\.pb\.js|\.pb\.ts)$`,
	}}

	if err := p.registerHooks(); err != nil {
		t.Fatalf("registerHooks: %v", err)
	}

	info, err := os.Stat(bystander)
	if err != nil {
		t.Fatalf("stat bystander: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("registerHooks wrote %d bytes to a disk file while using HooksFS", info.Size())
	}
}

// TestRegisterHooks_DiskStillPrepends is the other half of the guard: the
// path-based mode must keep bootstrapping the types directive.
func TestRegisterHooks_DiskStillPrepends(t *testing.T) {
	dir := t.TempDir()
	hook := filepath.Join(dir, "a.pb.ts")
	if err := os.WriteFile(hook, nil, 0o644); err != nil {
		t.Fatalf("seed hook: %v", err)
	}

	p := &plugin{app: testApp(t), config: Config{
		HooksDir:          dir,
		HooksFilesPattern: `^.*(\.pb\.js|\.pb\.ts)$`,
	}}

	if err := p.registerHooks(); err != nil {
		t.Fatalf("registerHooks: %v", err)
	}

	data, err := os.ReadFile(hook)
	if err != nil {
		t.Fatalf("read hook: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected the types reference directive to be prepended on disk")
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func testApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)
	return app
}
