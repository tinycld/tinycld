package webdav

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// testSource mirrors the shape drive uses, so these tests exercise the same
// configuration the real package ships.
func testSource() Source {
	return Source{
		Slug:       "testdrive",
		Prefix:     "/files",
		Collection: "tree_items",
		Fields: FieldMap{
			Name:     "name",
			Parent:   "parent",
			IsFolder: "is_folder",
			Size:     "size",
			MimeType: "mime_type",
			File:     "file",
			Owner:    "created_by",
			Updated:  "updated",
		},
	}
}

// setupTree builds a real collection shaped like a file tree, plus two users.
func setupTree(t *testing.T) (*tests.TestApp, *core.Record, *core.Record) {
	t.Helper()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatalf("NewTestApp: %v", err)
	}
	t.Cleanup(app.Cleanup)

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatalf("users collection: %v", err)
	}

	mkUser := func(email string) *core.Record {
		u := core.NewRecord(users)
		u.Set("email", email)
		// Explicit, unique per email: PocketBase otherwise auto-fills username
		// with a small random suffix, which can collide across the two users
		// and fail the save ("username: Value must be unique") — a flake.
		u.Set("username", strings.SplitN(email, "@", 2)[0])
		u.Set("password", "password123")
		if err := app.Save(u); err != nil {
			t.Fatalf("save user %s: %v", email, err)
		}
		return u
	}
	alice := mkUser("alice@example.com")
	bob := mkUser("bob@example.com")

	items := core.NewBaseCollection("tree_items")
	items.Fields.Add(&core.TextField{Name: "name", Required: true})
	items.Fields.Add(&core.BoolField{Name: "is_folder"})
	items.Fields.Add(&core.TextField{Name: "mime_type"})
	items.Fields.Add(&core.NumberField{Name: "size"})
	items.Fields.Add(&core.FileField{Name: "file", MaxSelect: 1, MaxSize: 5 << 20})
	items.Fields.Add(&core.RelationField{
		Name: "created_by", Required: true, CollectionId: users.Id, MaxSelect: 1,
	})
	items.Fields.Add(&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true})
	if err := app.Save(items); err != nil {
		t.Fatalf("save tree_items: %v", err)
	}

	// Self-relation added after the collection exists.
	withParent, err := app.FindCollectionByNameOrId("tree_items")
	if err != nil {
		t.Fatalf("refetch tree_items: %v", err)
	}
	withParent.Fields.Add(&core.RelationField{
		Name: "parent", CollectionId: withParent.Id, MaxSelect: 1,
	})
	if err := app.Save(withParent); err != nil {
		t.Fatalf("add parent field: %v", err)
	}

	return app, alice, bob
}

func mkItem(t *testing.T, app *tests.TestApp, owner *core.Record, name, parent string, isFolder bool) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("tree_items")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	r.Set("parent", parent)
	r.Set("is_folder", isFolder)
	r.Set("created_by", owner.Id)
	if err := app.Save(r); err != nil {
		t.Fatalf("save item %q: %v", name, err)
	}
	return r
}

func mkFile(t *testing.T, app *tests.TestApp, owner *core.Record, name, parent, body string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("tree_items")
	if err != nil {
		t.Fatal(err)
	}
	f, err := filesystem.NewFileFromBytes([]byte(body), name)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	r.Set("parent", parent)
	r.Set("is_folder", false)
	r.Set("created_by", owner.Id)
	r.Set("file", f)
	r.Set("size", f.Size)
	if err := app.Save(r); err != nil {
		t.Fatalf("save file %q: %v", name, err)
	}
	return r
}

// restrictToOwner stamps real PocketBase rules on the test collection: you may
// see and change only what you created.
//
// This is what the authorization tests exercise now — the same rule engine the
// REST API uses, not a Go closure standing in for it. If WebDAV ever stopped
// consulting the rules, these tests would go red rather than quietly passing
// against a parallel implementation.
// allowAuthenticated is the permissive baseline: any signed-in user may do
// anything. A freshly created collection has nil rules, which PocketBase reads
// as SUPERUSERS ONLY — so a test that wants a plain authenticated tree has to
// say so, exactly as a real package's migration would.
func allowAuthenticated(t *testing.T, app *tests.TestApp) {
	t.Helper()
	setRules(t, app, ruleSet{
		list: "@request.auth.id != \"\"", view: "@request.auth.id != \"\"",
		create: "@request.auth.id != \"\"", update: "@request.auth.id != \"\"",
		del: "@request.auth.id != \"\"",
	})
}

func restrictToOwner(t *testing.T, app *tests.TestApp) {
	t.Helper()
	own := "created_by = @request.auth.id"
	setRules(t, app, ruleSet{
		list: own, view: own, update: own, del: own,
		create: "@request.auth.id != \"\"",
	})
}

// ruleSet is the collection's five access rules. An empty string means "the
// PocketBase default for a fresh collection" — nil, i.e. superusers only —
// which is what a rule this fixture does not name should be, so that a code
// path failing to consult it cannot pass by accident.
type ruleSet struct {
	list, view, create, update, del string
}

func setRules(t *testing.T, app *tests.TestApp, rs ruleSet) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("tree_items")
	if err != nil {
		t.Fatal(err)
	}
	ptr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	col.ListRule = ptr(rs.list)
	col.ViewRule = ptr(rs.view)
	col.CreateRule = ptr(rs.create)
	col.UpdateRule = ptr(rs.update)
	col.DeleteRule = ptr(rs.del)
	if err := app.Save(col); err != nil {
		t.Fatalf("set access rules: %v", err)
	}
}

func newFS(t *testing.T, app core.App, src Source) *FileSystem {
	t.Helper()
	fs, err := NewFileSystem(app, src)
	if err != nil {
		t.Fatalf("NewFileSystem: %v", err)
	}
	return fs
}

func ctxAs(user *core.Record) context.Context {
	return context.WithValue(context.Background(), userKey, user)
}

func TestParsePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"root no slash", "/files", nil},
		{"root trailing slash", "/files/", nil},
		{"single segment", "/files/Documents", []string{"Documents"}},
		{"nested", "/files/Documents/report.pdf", []string{"Documents", "report.pdf"}},
		{"trailing slash on dir", "/files/Documents/", []string{"Documents"}},
		{"double slashes collapse", "/files//Documents//a.txt", []string{"Documents", "a.txt"}},
		{"dot segments collapse", "/files/./Documents", []string{"Documents"}},
		{"traversal escaping the mount yields root", "/files/../../etc/passwd", nil},
		{"unrelated prefix", "/other/Documents", nil},
		// Guards that the prefix check is a path boundary, not a bare
		// strings.HasPrefix — "/filesXYZ" must not be served as "/files".
		{"prefix is a path boundary", "/filesXYZ/Documents", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePath("/files", tc.in)
			if strings.Join(got, "/") != strings.Join(tc.want, "/") {
				t.Fatalf("parsePath(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateSourceRejectsUnsafeIdentifiers(t *testing.T) {
	// Source names are interpolated into the recursive-CTE SQL, so a
	// malformed identifier must be refused at construction.
	base := testSource()

	bad := []struct {
		name  string
		mutit func(*Source)
	}{
		{"sql injection in collection", func(s *Source) { s.Collection = "tree_items; DROP TABLE users--" }},
		{"quote in field", func(s *Source) { s.Fields.Name = `name"` }},
		{"space in field", func(s *Source) { s.Fields.Parent = "parent id" }},
		{"empty collection", func(s *Source) { s.Collection = "" }},
		{"empty required field", func(s *Source) { s.Fields.Owner = "" }},
		{"unsafe optional field", func(s *Source) { s.Fields.MimeType = "mime;--" }},
		{"relative prefix", func(s *Source) { s.Prefix = "files" }},
		{"prefix with trailing slash", func(s *Source) { s.Prefix = "/files/" }},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			src := base
			tc.mutit(&src)
			if err := validateSource(src); err == nil {
				t.Fatal("expected validateSource to reject this Source")
			}
		})
	}

	if err := validateSource(base); err != nil {
		t.Fatalf("the reference Source must validate, got %v", err)
	}
}

func TestStatAndListRoot(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())
	mkItem(t, app, alice, "Documents", "", true)

	info, err := fs.Stat(ctxAs(alice), "/files/")
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() || info.Name() != "files" {
		t.Fatalf("root Stat = %q isDir=%v", info.Name(), info.IsDir())
	}

	f, err := fs.OpenFile(ctxAs(alice), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := f.Readdir(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "Documents" {
		t.Fatalf("root listing = %v", names(entries))
	}
}

func TestStatNestedPath(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())
	docs := mkItem(t, app, alice, "Documents", "", true)
	mkFile(t, app, alice, "report.pdf", docs.Id, "hello")

	info, err := fs.Stat(ctxAs(alice), "/files/Documents/report.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() || info.Name() != "report.pdf" {
		t.Fatalf("Stat = %q isDir=%v", info.Name(), info.IsDir())
	}
	if info.Size() != int64(len("hello")) {
		t.Fatalf("Size = %d, want %d", info.Size(), len("hello"))
	}
	// Sys() exposes the backing record — the seam features reach through.
	if rec, ok := info.Sys().(*core.Record); !ok || rec == nil {
		t.Fatal("Sys() must return the backing *core.Record")
	}
}

func TestStatMissingPath(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())

	if _, err := fs.Stat(ctxAs(alice), "/files/nope.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

// An entry the viewer may not read must be INVISIBLE, not forbidden: answering
// not-found is what stops a probe confirming the path exists.
func TestUnreadableEntryIsNotFoundNotForbidden(t *testing.T) {
	app, alice, bob := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	mkFile(t, app, alice, "secret.txt", "", "classified")

	_, err := fs.Stat(ctxAs(bob), "/files/secret.txt")
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist (leaking existence?)", err)
	}
	if errors.Is(err, os.ErrPermission) {
		t.Fatal("a denied read must not surface as permission-denied")
	}
}

// The read verbs mask existence correctly (see above), but the write verbs did
// not: DELETE and MOVE answered 403 and MKCOL/MOVE-onto answered ErrExist for a
// record the caller cannot see. Either answer confirms "something is here",
// so a client that cannot read a byte of another user's tree could still map
// it by probing names — the exact fact the 404 on reads exists to withhold.
func TestWriteVerbsDoNotLeakExistence(t *testing.T) {
	t.Run("RemoveAll on an invisible entry", func(t *testing.T) {
		app, alice, bob := setupTree(t)
		restrictToOwner(t, app)
		fs := newFS(t, app, testSource())
		mkFile(t, app, alice, "secret.txt", "", "classified")

		err := fs.RemoveAll(ctxAs(bob), "/files/secret.txt")
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want os.ErrNotExist", err)
		}
		if errors.Is(err, os.ErrPermission) {
			t.Fatal("a denied delete must not surface as permission-denied")
		}
	})

	t.Run("Rename of an invisible entry", func(t *testing.T) {
		app, alice, bob := setupTree(t)
		restrictToOwner(t, app)
		fs := newFS(t, app, testSource())
		mkFile(t, app, alice, "secret.txt", "", "classified")

		err := fs.Rename(ctxAs(bob), "/files/secret.txt", "/files/moved.txt")
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err = %v, want os.ErrNotExist", err)
		}
	})

	t.Run("Mkdir onto an invisible name", func(t *testing.T) {
		app, alice, bob := setupTree(t)
		restrictToOwner(t, app)
		fs := newFS(t, app, testSource())
		mkItem(t, app, alice, "SecretFolder", "", true)

		// The (parent, name) namespace is globally unique, so this create
		// genuinely cannot succeed — but the refusal must not distinguish
		// "taken by someone you cannot see" from any other conflict.
		err := fs.Mkdir(ctxAs(bob), "/files/SecretFolder", 0o755)
		if err == nil {
			t.Fatal("creating over an existing invisible name must fail")
		}
		if errors.Is(err, os.ErrExist) {
			t.Fatal("ErrExist confirms another user's folder exists; " +
				"a generic conflict must be returned instead")
		}
	})

	t.Run("Rename onto an invisible destination", func(t *testing.T) {
		app, alice, bob := setupTree(t)
		restrictToOwner(t, app)
		fs := newFS(t, app, testSource())
		mkItem(t, app, alice, "SecretFolder", "", true)
		mkFile(t, app, bob, "mine.txt", "", "bob's")

		err := fs.Rename(ctxAs(bob), "/files/mine.txt", "/files/SecretFolder")
		if err == nil {
			t.Fatal("moving onto an existing invisible name must fail")
		}
		if errors.Is(err, os.ErrExist) {
			t.Fatal("ErrExist confirms another user's entry exists")
		}
	})

	t.Run("PUT onto an invisible name", func(t *testing.T) {
		app, alice, bob := setupTree(t)
		restrictToOwner(t, app)
		fs := newFS(t, app, testSource())
		mkFile(t, app, alice, "secret.txt", "", "classified")

		// Without masking this is treated as an overwrite and refused by the
		// update rule — a different outcome from a clean create, which is
		// itself the leak.
		_, err := fs.OpenFile(ctxAs(bob), "/files/secret.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		if err == nil {
			t.Fatal("writing over an invisible entry must fail")
		}
		if errors.Is(err, os.ErrExist) {
			t.Fatal("ErrExist confirms another user's file exists")
		}

		// And nothing may have been written to the victim's record.
		rec, rErr := fs.resolveByPath([]string{"secret.txt"})
		if rErr != nil {
			t.Fatal(rErr)
		}
		if rec.GetString("created_by") != alice.Id {
			t.Fatal("the victim's record was taken over")
		}
	})

	// The positive controls: the same verbs still work on the caller's own
	// entries, and a genuine visible conflict still reports ErrExist (which is
	// what the WebDAV handler maps to 405 and clients rely on).
	t.Run("owner can still delete, move, and sees real conflicts", func(t *testing.T) {
		app, alice, _ := setupTree(t)
		restrictToOwner(t, app)
		fs := newFS(t, app, testSource())
		mkFile(t, app, alice, "mine.txt", "", "hello")
		mkItem(t, app, alice, "Existing", "", true)

		if err := fs.Rename(ctxAs(alice), "/files/mine.txt", "/files/renamed.txt"); err != nil {
			t.Fatalf("owner rename: %v", err)
		}
		if err := fs.RemoveAll(ctxAs(alice), "/files/renamed.txt"); err != nil {
			t.Fatalf("owner delete: %v", err)
		}
		if err := fs.Mkdir(ctxAs(alice), "/files/Existing", 0o755); !errors.Is(err, os.ErrExist) {
			t.Fatalf("a visible conflict should still be ErrExist, got %v", err)
		}
	})
}

func TestListingHidesUnreadableEntries(t *testing.T) {
	app, alice, bob := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	mkItem(t, app, alice, "alice-only", "", true)
	mkItem(t, app, bob, "bob-only", "", true)

	f, err := fs.OpenFile(ctxAs(bob), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, _ := f.Readdir(0)

	got := names(entries)
	if len(got) != 1 || got[0] != "bob-only" {
		t.Fatalf("bob's listing = %v; must not include alice's entries", got)
	}
}

func TestReadFileContent(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())
	mkFile(t, app, alice, "a.txt", "", "file body here")

	f, err := fs.OpenFile(ctxAs(alice), "/files/a.txt", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	body, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "file body here" {
		t.Fatalf("body = %q", body)
	}
}

func TestMkdirCreatesFolder(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())

	if err := fs.Mkdir(ctxAs(alice), "/files/NewFolder", 0o755); err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat(ctxAs(alice), "/files/NewFolder")
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Fatal("created entry is not a directory")
	}

	// The owner field must be stamped, or the entry belongs to nobody.
	rec := info.Sys().(*core.Record)
	if rec.GetString("created_by") != alice.Id {
		t.Fatalf("created_by = %q, want %q", rec.GetString("created_by"), alice.Id)
	}
}

func TestMkdirExistingIsErrExist(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())
	mkItem(t, app, alice, "Dup", "", true)

	if err := fs.Mkdir(ctxAs(alice), "/files/Dup", 0o755); !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want os.ErrExist", err)
	}
}

func TestWriteCreatesFileWithMime(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())

	f, err := fs.OpenFile(ctxAs(alice), "/files/notes.txt", os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("written via dav")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := fs.Stat(ctxAs(alice), "/files/notes.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len("written via dav")) {
		t.Fatalf("size = %d", info.Size())
	}
	rec := info.Sys().(*core.Record)
	if got := rec.GetString("mime_type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("mime_type = %q, want a text/plain variant", got)
	}
	if rec.GetString("created_by") != alice.Id {
		t.Fatal("created_by not stamped on write")
	}
}

// PUT-of-a-new-file and MKCOL are creates, and CreateRule is where a package
// puts the clauses that apply only to creates — drive's guest exclusion and
// the disabled-user clause both live there and nowhere else. Evaluating no
// rule on those two verbs means a user the collection forbids from creating
// anything creates it anyway, over WebDAV.
func TestCreateDeniedByCreateRule(t *testing.T) {
	// A rule nobody satisfies: a create that consults it must fail, and one
	// that consults nothing will sail through.
	deny := "created_by = \"nobody\""
	permissive := "@request.auth.id != \"\""

	t.Run("PUT of a new file", func(t *testing.T) {
		app, alice, _ := setupTree(t)
		setRules(t, app, ruleSet{
			list: permissive, view: permissive, update: permissive, del: permissive,
			create: deny,
		})
		fs := newFS(t, app, testSource())

		f, err := fs.OpenFile(ctxAs(alice), "/files/blocked.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			// Refusing at open is equally correct.
			if !errors.Is(err, os.ErrPermission) {
				t.Fatalf("open err = %v, want os.ErrPermission", err)
			}
			return
		}
		if _, err := f.Write([]byte("should not persist")); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("close err = %v, want os.ErrPermission", err)
		}

		// And nothing may be left behind: a rolled-back create must not leave
		// a half-written row for the next PROPFIND to find.
		if _, err := fs.resolveByPath([]string{"blocked.txt"}); err == nil {
			t.Fatal("a denied create persisted a record")
		}
	})

	t.Run("MKCOL", func(t *testing.T) {
		app, alice, _ := setupTree(t)
		setRules(t, app, ruleSet{
			list: permissive, view: permissive, update: permissive, del: permissive,
			create: deny,
		})
		fs := newFS(t, app, testSource())

		if err := fs.Mkdir(ctxAs(alice), "/files/Blocked", 0o755); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("err = %v, want os.ErrPermission", err)
		}
		if _, err := fs.resolveByPath([]string{"Blocked"}); err == nil {
			t.Fatal("a denied MKCOL persisted a folder")
		}
	})

	// The positive control: with a create rule the user does satisfy, both
	// verbs still work. Without this, deleting the rule check entirely would
	// leave the deny-tests red but tell us nothing about over-blocking.
	t.Run("allowed creates still succeed", func(t *testing.T) {
		app, alice, _ := setupTree(t)
		allowAuthenticated(t, app)
		fs := newFS(t, app, testSource())

		if err := fs.Mkdir(ctxAs(alice), "/files/Allowed", 0o755); err != nil {
			t.Fatalf("MKCOL: %v", err)
		}
		f, err := fs.OpenFile(ctxAs(alice), "/files/allowed.txt", os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		if _, err := f.Write([]byte("ok")); err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})
}

func TestWriteWithoutCreateFlagOnMissingFails(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())

	_, err := fs.OpenFile(ctxAs(alice), "/files/absent.txt", os.O_WRONLY, 0o644)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want os.ErrNotExist", err)
	}
}

func TestRenameMovesEntry(t *testing.T) {
	app, alice, _ := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	dst := mkItem(t, app, alice, "Dest", "", true)
	mkFile(t, app, alice, "move-me.txt", "", "x")

	if err := fs.Rename(ctxAs(alice), "/files/move-me.txt", "/files/Dest/moved.txt"); err != nil {
		t.Fatal(err)
	}

	if _, err := fs.Stat(ctxAs(alice), "/files/move-me.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source still present: %v", err)
	}
	info, err := fs.Stat(ctxAs(alice), "/files/Dest/moved.txt")
	if err != nil {
		t.Fatal(err)
	}
	if rec := info.Sys().(*core.Record); rec.GetString("parent") != dst.Id {
		t.Fatal("moved entry was not reparented")
	}
}

func TestRenameOntoExistingIsErrExist(t *testing.T) {
	app, alice, _ := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	mkFile(t, app, alice, "a.txt", "", "a")
	mkFile(t, app, alice, "b.txt", "", "b")

	if err := fs.Rename(ctxAs(alice), "/files/a.txt", "/files/b.txt"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("err = %v, want os.ErrExist", err)
	}
}

// Reparenting a folder under its own descendant would make every recursive walk
// of the tree loop forever.
func TestRenameRejectsCycle(t *testing.T) {
	app, alice, _ := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	parent := mkItem(t, app, alice, "Parent", "", true)
	mkItem(t, app, alice, "Child", parent.Id, true)

	err := fs.Rename(ctxAs(alice), "/files/Parent", "/files/Parent/Child/Parent")
	if err == nil {
		t.Fatal("moving a folder into its own descendant must be refused")
	}
}

func TestRemoveAllDeletes(t *testing.T) {
	app, alice, _ := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	mkFile(t, app, alice, "gone.txt", "", "bye")

	if err := fs.RemoveAll(ctxAs(alice), "/files/gone.txt"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Stat(ctxAs(alice), "/files/gone.txt"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("entry survived delete: %v", err)
	}
}

func TestHooksDenyWriteAndDelete(t *testing.T) {
	app, alice, bob := setupTree(t)
	restrictToOwner(t, app)
	fs := newFS(t, app, testSource())

	mkFile(t, app, alice, "alices.txt", "", "hers")

	if err := fs.RemoveAll(ctxAs(bob), "/files/alices.txt"); err == nil {
		t.Fatal("bob must not be able to delete alice's file")
	}
	if err := fs.Rename(ctxAs(bob), "/files/alices.txt", "/files/stolen.txt"); err == nil {
		t.Fatal("bob must not be able to move alice's file")
	}
}

// Overwriting must call BeforeOverwrite so the feature can archive the outgoing
// version. (Quota moved to core/quota, which enforces it as a record hook on
// every write path — see that package's tests for the delta semantics.)
func TestOverwriteCallsBeforeOverwrite(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	src := testSource()

	var overwrote bool
	var sawRecord string
	src.Hooks.BeforeOverwrite = func(_ core.App, _ string, rec *core.Record) error {
		overwrote = true
		sawRecord = rec.Id
		return nil
	}
	fs := newFS(t, app, src)

	existing := mkFile(t, app, alice, "doc.txt", "", "12345")

	f, err := fs.OpenFile(ctxAs(alice), "/files/doc.txt", os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if !overwrote {
		t.Fatal("BeforeOverwrite was not called on an overwrite")
	}
	if sawRecord != existing.Id {
		t.Fatalf("BeforeOverwrite saw %q, want the existing record %q", sawRecord, existing.Id)
	}

	info, err := fs.Stat(ctxAs(alice), "/files/doc.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 10 {
		t.Fatalf("size after overwrite = %d, want 10", info.Size())
	}
}

// A failing BeforeOverwrite is best-effort: losing a version snapshot must not
// lose the write the user actually asked for.
func TestBeforeOverwriteFailureDoesNotFailWrite(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)
	src := testSource()
	src.Hooks.BeforeOverwrite = func(_ core.App, _ string, _ *core.Record) error {
		return errors.New("snapshot store unavailable")
	}
	fs := newFS(t, app, src)

	mkFile(t, app, alice, "doc.txt", "", "old")

	f, err := fs.OpenFile(ctxAs(alice), "/files/doc.txt", os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("new content")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("write must survive a failed snapshot, got %v", err)
	}

	info, _ := fs.Stat(ctxAs(alice), "/files/doc.txt")
	if info.Size() != int64(len("new content")) {
		t.Fatalf("write did not land: size = %d", info.Size())
	}
}

// A FileSystem method reached without an authenticated user is a programming
// error — the middleware must always have run.
func TestMissingUserInContextFails(t *testing.T) {
	app, _, _ := setupTree(t)
	allowAuthenticated(t, app)
	fs := newFS(t, app, testSource())

	if _, err := fs.Stat(context.Background(), "/files/"); err == nil {
		t.Fatal("expected an error when no user is on the context")
	}
}

func names(infos []os.FileInfo) []string {
	out := make([]string, len(infos))
	for i, e := range infos {
		out[i] = e.Name()
	}
	return out
}

// The .well-known alias names the PROTOCOL, not the mount point: a client looks
// for /.well-known/webdav wherever the tree lives. Deriving it from the prefix
// would serve /.well-known/drive and leave the path clients actually request
// unhandled — which is exactly what the lift briefly did.
func TestWellKnownPathIsProtocolNotPrefix(t *testing.T) {
	if wellKnownPath != "/.well-known/webdav" {
		t.Fatalf("wellKnownPath = %q, want /.well-known/webdav", wellKnownPath)
	}

	src := testSource() // Prefix "/files"
	prefixes := Prefixes([]Source{src})

	var sawWellKnown, sawPrefix bool
	for _, p := range prefixes {
		switch p {
		case "/.well-known/webdav":
			sawWellKnown = true
		case "/files":
			sawPrefix = true
		case "/.well-known/files":
			t.Fatal("the alias must not be derived from the mount prefix")
		}
	}
	if !sawWellKnown || !sawPrefix {
		t.Fatalf("Prefixes() = %v, want both the mount and the protocol alias", prefixes)
	}

	if !HasPrefix([]Source{src}, "/.well-known/webdav") {
		t.Fatal("HasPrefix must claim the protocol alias")
	}
}

// Two sources must not double-register the single alias (ServeMux panics on a
// duplicate pattern).
func TestMultipleSourcesRegisterOneWellKnown(t *testing.T) {
	app, _, _ := setupTree(t)
	allowAuthenticated(t, app)

	a := testSource()
	b := testSource()
	b.Slug, b.Prefix = "second", "/second"

	// HandlerFor is where a duplicate pattern would panic.
	h, _, err := HandlerFor(app, []Source{a, b}, HostBindings{})
	if err != nil {
		t.Fatal(err)
	}
	if h == nil {
		t.Fatal("expected a handler")
	}
}

// A TENANT's sources are materialized from JSON, so they arrive with zero
// Hooks — a Go closure cannot travel through config. RegisterSourceHooks is
// the seam that closes the gap (R7): feature Go (RegisterExtras, which runs
// before the sources mount) registers its hooks by slug, and the constructor
// adopts them onto the matching source. Without this, a tenant-served
// overwrite silently skipped version archiving.
func TestMaterializedSourceAdoptsRegisteredHooks(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)

	// Shaped like a tenant mount: same source, but Hooks zero.
	src := testSource()
	src.Hooks = Hooks{}

	var overwrote bool
	RegisterSourceHooks(app, src.Slug, Hooks{
		BeforeOverwrite: func(_ core.App, _ string, _ *core.Record) error {
			overwrote = true
			return nil
		},
	})

	fs := newFS(t, app, src)
	mkFile(t, app, alice, "doc.txt", "", "12345")

	f, err := fs.OpenFile(ctxAs(alice), "/files/doc.txt", os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if !overwrote {
		t.Fatal("registered hooks were not adopted: BeforeOverwrite did not fire on a materialized-shaped source")
	}
}

// Explicit hooks on the source win — the registry only fills the gap left by
// materialization, it must not override a composition that set its own.
func TestExplicitSourceHooksBeatRegistered(t *testing.T) {
	app, alice, _ := setupTree(t)
	allowAuthenticated(t, app)

	var explicitRan, registeredRan bool
	src := testSource()
	src.Hooks.BeforeOverwrite = func(_ core.App, _ string, _ *core.Record) error {
		explicitRan = true
		return nil
	}
	RegisterSourceHooks(app, src.Slug, Hooks{
		BeforeOverwrite: func(_ core.App, _ string, _ *core.Record) error {
			registeredRan = true
			return nil
		},
	})

	fs := newFS(t, app, src)
	mkFile(t, app, alice, "doc.txt", "", "12345")

	f, err := fs.OpenFile(ctxAs(alice), "/files/doc.txt", os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("1234567890")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	if !explicitRan {
		t.Fatal("explicit source hook did not run")
	}
	if registeredRan {
		t.Fatal("registered hook overrode the source's explicit hook")
	}
}
