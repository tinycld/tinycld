package webdav

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeHookPoint is a TSHookPoint whose behaviour a test controls directly. It
// also counts invocations, which is what proves the fast path never reaches it.
type fakeHookPoint struct {
	enabled bool
	calls   int
	lastArg map[string]any

	value any
	err   error
}

func (f *fakeHookPoint) Enabled() bool { return f.enabled }

func (f *fakeHookPoint) Call(payload map[string]any) (any, bool, error) {
	f.calls++
	f.lastArg = payload
	if f.err != nil {
		return nil, false, f.err
	}
	return f.value, true, nil
}

// THE performance property: with no TS handler registered, a full request path
// must never invoke a hook point. A counter that stays at zero is what proves
// an org customizing nothing never pays for a VM borrow.
func TestFastPathNeverCallsDisabledHooks(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())

	points := map[string]*fakeHookPoint{
		"beforeWrite":  {enabled: false},
		"beforeDelete": {enabled: false},
		"beforeMove":   {enabled: false},
		"canRead":      {enabled: false},
		"filterList":   {enabled: false},
	}
	fs.SetTSHooks(TSHooks{
		BeforeWrite:  points["beforeWrite"],
		BeforeDelete: points["beforeDelete"],
		BeforeMove:   points["beforeMove"],
		CanRead:      points["canRead"],
		FilterList:   points["filterList"],
	})

	// Drive the whole surface: create, list, move, delete.
	if err := fs.Mkdir(ctxAs(alice), "/files/Dir", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.OpenFile(ctxAs(alice), "/files/a.txt", os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("x"))
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	dir, err := fs.OpenFile(ctxAs(alice), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	dir.Readdir(0)
	dir.Close()
	if err := fs.Rename(ctxAs(alice), "/files/a.txt", "/files/b.txt"); err != nil {
		t.Fatal(err)
	}
	if err := fs.RemoveAll(ctxAs(alice), "/files/b.txt"); err != nil {
		t.Fatal(err)
	}

	for name, hp := range points {
		if hp.calls != 0 {
			t.Fatalf("%s was called %d times on the fast path; want 0", name, hp.calls)
		}
	}
}

func TestBeforeWriteHookVetoesPut(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())

	hp := &fakeHookPoint{enabled: true, err: errors.New("blocked by policy")}
	fs.SetTSHooks(TSHooks{BeforeWrite: hp})

	_, err := fs.OpenFile(ctxAs(alice), "/files/malware.exe", os.O_WRONLY|os.O_CREATE, 0o644)
	if err == nil {
		t.Fatal("a throwing beforeWrite handler must reject the write")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v, want it to wrap os.ErrPermission", err)
	}
	if !strings.Contains(err.Error(), "blocked by policy") {
		t.Fatalf("err %q must carry the handler's message", err)
	}
	if hp.lastArg["name"] != "malware.exe" || hp.lastArg["isCreate"] != true {
		t.Fatalf("handler payload = %#v", hp.lastArg)
	}
}

func TestBeforeWriteHookVetoesMkcol(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())
	fs.SetTSHooks(TSHooks{BeforeWrite: &fakeHookPoint{enabled: true, err: errors.New("no folders")}})

	if err := fs.Mkdir(ctxAs(alice), "/files/Nope", 0o755); err == nil {
		t.Fatal("a throwing beforeWrite handler must reject MKCOL")
	}
	if _, err := fs.Stat(ctxAs(alice), "/files/Nope"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a vetoed MKCOL must not leave a record behind")
	}
}

func TestBeforeDeleteHookVetoes(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())
	mkFile(t, app, alice, "keep.txt", "", "x")

	hp := &fakeHookPoint{enabled: true, err: errors.New("retention policy")}
	fs.SetTSHooks(TSHooks{BeforeDelete: hp})

	if err := fs.RemoveAll(ctxAs(alice), "/files/keep.txt"); err == nil {
		t.Fatal("a throwing beforeDelete handler must reject the delete")
	}
	if _, err := fs.Stat(ctxAs(alice), "/files/keep.txt"); err != nil {
		t.Fatal("a vetoed delete must leave the entry in place")
	}
	if hp.lastArg["name"] != "keep.txt" {
		t.Fatalf("handler payload = %#v", hp.lastArg)
	}
}

func TestBeforeMoveHookVetoes(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())
	mkFile(t, app, alice, "pinned.txt", "", "x")

	hp := &fakeHookPoint{enabled: true, err: errors.New("pinned in place")}
	fs.SetTSHooks(TSHooks{BeforeMove: hp})

	if err := fs.Rename(ctxAs(alice), "/files/pinned.txt", "/files/moved.txt"); err == nil {
		t.Fatal("a throwing beforeMove handler must reject the move")
	}
	if _, err := fs.Stat(ctxAs(alice), "/files/pinned.txt"); err != nil {
		t.Fatal("a vetoed move must leave the entry at its original path")
	}
	if hp.lastArg["from"] != "/files/pinned.txt" || hp.lastArg["to"] != "/files/moved.txt" {
		t.Fatalf("handler payload = %#v", hp.lastArg)
	}
}

func TestCanReadHookNarrowsListing(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())
	mkItem(t, app, alice, "visible", "", true)
	mkItem(t, app, alice, "hidden", "", true)

	// Deny exactly one entry by name.
	hp := &denyByName{enabled: true, deny: "hidden"}
	fs.SetTSHooks(TSHooks{CanRead: hp})

	dir, err := fs.OpenFile(ctxAs(alice), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	entries, _ := dir.Readdir(0)

	got := names(entries)
	if len(got) != 1 || got[0] != "visible" {
		t.Fatalf("listing = %v, want only [visible]", got)
	}
}

// SECURITY: a TS hook may hide entries, never reveal them. A handler that
// returns names Go never authorized must not add them to the listing.
func TestFilterListCannotRevealUnauthorizedEntries(t *testing.T) {
	app, alice, bob := setupTree(t)
	src := testSource()
	src.Hooks = ownerOnlyHooks()
	fs := newFS(t, app, src)

	mkItem(t, app, alice, "alice-secret", "", true)
	mkItem(t, app, bob, "bob-own", "", true)

	// A malicious/buggy handler tries to add an entry bob was never granted,
	// plus one that does not exist at all.
	fs.SetTSHooks(TSHooks{FilterList: &fakeHookPoint{
		enabled: true,
		value:   []any{"bob-own", "alice-secret", "invented.txt"},
	}})

	dir, err := fs.OpenFile(ctxAs(bob), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	entries, _ := dir.Readdir(0)

	got := names(entries)
	if len(got) != 1 || got[0] != "bob-own" {
		t.Fatalf("listing = %v; a hook must not be able to reveal entries Go denied", got)
	}
}

func TestFilterListHidesEntries(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())
	mkItem(t, app, alice, "shown", "", true)
	mkItem(t, app, alice, ".hidden", "", true)

	fs.SetTSHooks(TSHooks{FilterList: &fakeHookPoint{
		enabled: true,
		value:   []any{"shown"},
	}})

	dir, err := fs.OpenFile(ctxAs(alice), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	entries, _ := dir.Readdir(0)

	got := names(entries)
	if len(got) != 1 || got[0] != "shown" {
		t.Fatalf("listing = %v, want only [shown]", got)
	}
}

// One VM borrow per directory, not one per entry — the property that keeps the
// hot path affordable.
func TestFilterListIsCalledOncePerDirectory(t *testing.T) {
	app, alice, _ := setupTree(t)
	fs := newFS(t, app, testSource())
	for _, n := range []string{"a", "b", "c", "d", "e"} {
		mkItem(t, app, alice, n, "", true)
	}

	hp := &fakeHookPoint{enabled: true, value: []any{"a", "b", "c", "d", "e"}}
	fs.SetTSHooks(TSHooks{FilterList: hp})

	dir, err := fs.OpenFile(ctxAs(alice), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	dir.Readdir(0)

	if hp.calls != 1 {
		t.Fatalf("filterList called %d times for one listing of 5 entries; want 1", hp.calls)
	}
	items, _ := hp.lastArg["items"].([]any)
	if len(items) != 5 {
		t.Fatalf("handler received %d items; want the whole batch of 5", len(items))
	}
}

// A handler returning a non-list is a bug in the hook. It must not fail the
// request, and — more importantly — must not fall back to the unfiltered set in
// a way that leaks anything Go had already excluded.
func TestFilterListIgnoresNonListReturn(t *testing.T) {
	app, alice, bob := setupTree(t)
	src := testSource()
	src.Hooks = ownerOnlyHooks()
	fs := newFS(t, app, src)

	mkItem(t, app, alice, "alice-only", "", true)
	mkItem(t, app, bob, "bob-only", "", true)

	fs.SetTSHooks(TSHooks{FilterList: &fakeHookPoint{enabled: true, value: "not a list"}})

	dir, err := fs.OpenFile(ctxAs(bob), "/files/", os.O_RDONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	entries, _ := dir.Readdir(0)

	got := names(entries)
	if len(got) != 1 || got[0] != "bob-only" {
		t.Fatalf("listing = %v; a malformed hook return must keep Go's answer", got)
	}
}

// denyByName denies exactly one entry, to exercise the per-entry CanRead path.
type denyByName struct {
	enabled bool
	deny    string
	calls   int
}

func (d *denyByName) Enabled() bool { return d.enabled }

func (d *denyByName) Call(payload map[string]any) (any, bool, error) {
	d.calls++
	name, _ := payload["name"].(string)
	return name != d.deny, true, nil
}

var _ TSHookPoint = (*fakeHookPoint)(nil)
var _ TSHookPoint = (*denyByName)(nil)
