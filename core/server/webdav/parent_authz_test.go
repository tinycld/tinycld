package webdav

import (
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// parent_authz_test.go covers the destination side of a write.
//
// Every write verb resolved its parent folder by path and used only the id, so
// the caller's right to READ that folder was never checked. Two consequences,
// both reopening the existence oracle the leaf-name masking closes:
//
//   - A member can plant a record inside another user's unreadable folder,
//     squatting the globally-unique (parent, name) namespace inside a tree they
//     cannot see.
//   - The refusal distinguishes "that parent folder exists" (proceed, then fail
//     on the create rule) from "it doesn't" (ErrNotExist), so probing paths maps
//     another user's directory structure.
//
// drive's createRule carries no parent clause, so the rule engine does not
// close this on its own.

func TestWriteVerbsRequireReadableParent(t *testing.T) {
	// Alice owns a folder Bob cannot read. Bob may create (the permissive
	// createRule every package ships) but must not reach inside it.
	setup := func(t *testing.T) (*FileSystem, *tests.TestApp, *core.Record, *core.Record) {
		t.Helper()
		app, alice, bob := setupTree(t)
		restrictToOwner(t, app)
		mkItem(t, app, alice, "AliceFolder", "", true)
		return newFS(t, app, testSource()), app, alice, bob
	}

	t.Run("Mkdir inside an unreadable parent", func(t *testing.T) {
		fs, _, _, bob := setup(t)
		err := fs.Mkdir(ctxAs(bob), "/files/AliceFolder/planted", 0o755)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want os.ErrNotExist (the parent must read as absent)", err)
		}
		if rec, _ := fs.resolveByPath([]string{"AliceFolder", "planted"}); rec != nil {
			t.Fatal("a folder was planted inside an unreadable parent")
		}
	})

	t.Run("PUT inside an unreadable parent", func(t *testing.T) {
		fs, _, _, bob := setup(t)
		_, err := fs.OpenFile(ctxAs(bob), "/files/AliceFolder/planted.txt",
			os.O_WRONLY|os.O_CREATE, 0o644)
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want os.ErrNotExist", err)
		}
		if rec, _ := fs.resolveByPath([]string{"AliceFolder", "planted.txt"}); rec != nil {
			t.Fatal("a file was planted inside an unreadable parent")
		}
	})

	t.Run("MOVE into an unreadable parent", func(t *testing.T) {
		fs, app, _, bob := setup(t)
		mkFile(t, app, bob, "mine.txt", "", "bob's")

		err := fs.Rename(ctxAs(bob), "/files/mine.txt", "/files/AliceFolder/mine.txt")
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want os.ErrNotExist", err)
		}
		moved, rErr := fs.resolveByPath([]string{"mine.txt"})
		if rErr != nil || moved == nil {
			t.Fatal("the source file must stay where it was")
		}
	})

	// The oracle itself: reaching into a parent that exists but is unreadable
	// must be indistinguishable from reaching into one that does not exist.
	t.Run("existing-but-unreadable is indistinguishable from missing", func(t *testing.T) {
		fs, _, _, bob := setup(t)

		hidden := fs.Mkdir(ctxAs(bob), "/files/AliceFolder/x", 0o755)
		absent := fs.Mkdir(ctxAs(bob), "/files/NoSuchFolder/x", 0o755)
		if hidden == nil || absent == nil {
			t.Fatal("both cases must fail")
		}
		if hidden.Error() != absent.Error() {
			t.Fatalf("unreadable parent yields %q but missing parent yields %q — "+
				"the difference maps another user's tree", hidden, absent)
		}
	})

	// Positive control: a parent the caller CAN read still accepts writes, so
	// the tests above fail on the authorization and not on a broken fixture.
	t.Run("owner can still write inside their own folder", func(t *testing.T) {
		fs, app, _, bob := setup(t)
		mkItem(t, app, bob, "BobFolder", "", true)

		if err := fs.Mkdir(ctxAs(bob), "/files/BobFolder/sub", 0o755); err != nil {
			t.Fatalf("owner Mkdir in own folder: %v", err)
		}
		f, err := fs.OpenFile(ctxAs(bob), "/files/BobFolder/note.txt",
			os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			t.Fatalf("owner PUT in own folder: %v", err)
		}
		if _, err := f.Write([]byte("hello")); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
		if rec, _ := fs.resolveByPath([]string{"BobFolder", "note.txt"}); rec == nil {
			t.Fatal("the owner's own write did not persist")
		}
	})

	// Writing at the mount root is unaffected: there is no parent record to
	// authorize, and the create rule remains the only gate.
	t.Run("root writes are unaffected", func(t *testing.T) {
		fs, _, _, bob := setup(t)
		if err := fs.Mkdir(ctxAs(bob), "/files/BobRoot", 0o755); err != nil {
			t.Fatalf("Mkdir at root: %v", err)
		}
	})
}

// TestConcurrentUploadsOfSameBasenameDoNotSwapContent covers F1.
//
// persistWrite renamed each upload to $TMPDIR/<user-chosen basename> — one
// shared process temp dir — because PocketBase derives the stored blob's name
// from the source file's basename. Two concurrent PUTs of the same basename by
// ANY two users therefore collided on one path, and os.Rename silently
// replaces: user A's record could ingest user B's bytes.
func TestConcurrentUploadsOfSameBasenameDoNotSwapContent(t *testing.T) {
	app, alice, bob := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())

	// Distinct parents so the two writes are not fighting over one (parent,
	// name) row — only over the temp path, which is the bug under test.
	mkItem(t, app, alice, "alice", "", true)
	mkItem(t, app, bob, "bob", "", true)

	bodies := map[string]string{
		"alice": strings.Repeat("A", 4096),
		"bob":   strings.Repeat("B", 4096),
	}

	// Many rounds: the window between the rename and NewFileFromPath reading the
	// path is small, so one pass can pass by luck even with the bug present.
	for round := 0; round < 40; round++ {
		var wg sync.WaitGroup
		writeAs := func(user *core.Record, dir string) {
			defer wg.Done()
			f, err := fs.OpenFile(ctxAs(user), "/files/"+dir+"/report.txt",
				os.O_WRONLY|os.O_CREATE, 0o644)
			if err != nil {
				t.Errorf("open %s: %v", dir, err)
				return
			}
			if _, err := f.Write([]byte(bodies[dir])); err != nil {
				t.Errorf("write %s: %v", dir, err)
				return
			}
			if err := f.Close(); err != nil {
				t.Errorf("close %s: %v", dir, err)
			}
		}
		wg.Add(2)
		go writeAs(alice, "alice")
		go writeAs(bob, "bob")
		wg.Wait()

		for dir, want := range bodies {
			rec, err := fs.resolveByPath([]string{dir, "report.txt"})
			if err != nil || rec == nil {
				t.Fatalf("round %d: %s/report.txt missing: %v", round, dir, err)
			}
			f, err := fs.OpenFile(ctxAs(alice), "/files/"+dir+"/report.txt", os.O_RDONLY, 0)
			if err != nil {
				t.Fatalf("round %d: reopen %s: %v", round, dir, err)
			}
			got, err := io.ReadAll(f)
			_ = f.Close()
			if err != nil {
				t.Fatalf("round %d: read %s: %v", round, dir, err)
			}
			if string(got) != want {
				t.Fatalf("round %d: %s/report.txt content starts %q, want %q — "+
					"concurrent uploads swapped bytes through the shared temp path",
					round, dir, string(got[:min(8, len(got))]), want[:8])
			}
			// The stored blob must still carry the user's filename, which is the
			// whole reason the rename exists.
			if stored := rec.GetString(fs.src.Fields.File); !strings.HasPrefix(stored, "report") {
				t.Fatalf("round %d: stored blob name %q lost the user's basename",
					round, stored)
			}
			if err := app.Delete(rec); err != nil {
				t.Fatal(err)
			}
		}
	}
}
