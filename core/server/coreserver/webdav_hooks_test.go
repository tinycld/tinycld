package coreserver

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/webdav"
)

// End-to-end proof of the WebDAV TS seam: a real .pb.ts on disk calls
// webdavHook({...}), and the resulting handlers land on the hook points a
// FileSystem consults. Everything below the binding — transpile, compile, pool
// borrow, value export — is the real machinery.

func testWebDAVSource() webdav.Source {
	return webdav.Source{
		Slug:       "drive",
		Prefix:     "/drive",
		Collection: "drive_items",
		Fields: webdav.FieldMap{
			Name:     "name",
			Parent:   "parent",
			IsFolder: "is_folder",
			Size:     "size",
			File:     "file",
			Owner:    "created_by",
		},
	}
}

// bootHooks writes a .pb.ts, boots jsvm with core's bindings, and returns the
// TSHooks the source's FileSystem would carry.
func bootHooks(t *testing.T, hookSource string) webdav.TSHooks {
	t.Helper()

	resetHookPointsForTesting()
	resetLoaderBindersForTesting()

	hooksDir := t.TempDir()
	if hookSource != "" {
		if err := os.WriteFile(filepath.Join(hooksDir, "drive.pb.ts"), []byte(hookSource), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// This is what webdav.Register does when handed the host bindings.
	hooks := webdav.RegisterTSHooks(WebDAVHostBindings(), testWebDAVSource())

	testApp, _ := tests.NewTestApp()
	t.Cleanup(testApp.Cleanup)

	pbApp := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: testApp.DataDir()})
	err := jsvm.Register(pbApp, jsvm.Config{
		HooksDir:      hooksDir,
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 2,
		OnInit:        buildJsvmOnInit(pbApp),
		OnLoaderInit:  buildJsvmOnLoaderInit(pbApp),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	return hooks
}

// With no hook file at all, every point must report disabled — the fast path.
func TestWebDAVHooksDisabledWithoutHookFile(t *testing.T) {
	hooks := bootHooks(t, "")

	for name, hp := range map[string]webdav.TSHookPoint{
		"beforeWrite":  hooks.BeforeWrite,
		"beforeDelete": hooks.BeforeDelete,
		"beforeMove":   hooks.BeforeMove,
		"canRead":      hooks.CanRead,
		"filterList":   hooks.FilterList,
	} {
		if hp == nil {
			t.Fatalf("%s point is nil; it must exist even with no handlers", name)
		}
		if hp.Enabled() {
			t.Fatalf("%s reports enabled with no hook file registered", name)
		}
	}
}

// A .pb.ts registering only beforeWrite must enable exactly that point — the
// others stay on the free path.
func TestWebDAVHookRegistersOnlyDeclaredPoints(t *testing.T) {
	hooks := bootHooks(t, `
        webdavHook({
            beforeWrite(e) {
                if (e.name.endsWith('.exe')) {
                    throw new Error('executables are not allowed')
                }
            },
        })
    `)

	if !hooks.BeforeWrite.Enabled() {
		t.Fatal("beforeWrite must be enabled after the hook file registers it")
	}
	for name, hp := range map[string]webdav.TSHookPoint{
		"beforeDelete": hooks.BeforeDelete,
		"beforeMove":   hooks.BeforeMove,
		"canRead":      hooks.CanRead,
		"filterList":   hooks.FilterList,
	} {
		if hp.Enabled() {
			t.Fatalf("%s must stay disabled when the hook file does not declare it", name)
		}
	}
}

// The veto path, driven by a real handler: a thrown error must reach Go.
func TestWebDAVBeforeWriteHandlerVetoes(t *testing.T) {
	hooks := bootHooks(t, `
        webdavHook({
            beforeWrite(e) {
                if (e.name.endsWith('.exe')) {
                    throw new Error('executables are not allowed')
                }
            },
        })
    `)

	// An allowed name passes.
	if _, _, err := hooks.BeforeWrite.Call(map[string]any{"name": "report.pdf"}); err != nil {
		t.Fatalf("an allowed name must not be rejected: %v", err)
	}

	// A blocked name throws, and the message survives the boundary.
	_, _, err := hooks.BeforeWrite.Call(map[string]any{"name": "malware.exe"})
	if err == nil {
		t.Fatal("a blocked name must produce an error")
	}
	if got := err.Error(); got == "" || !contains(got, "executables are not allowed") {
		t.Fatalf("error %q must carry the handler's message", got)
	}
}

// filterList returning a subset is the listing-hide path.
func TestWebDAVFilterListHandlerReturnsSubset(t *testing.T) {
	hooks := bootHooks(t, `
        webdavHook({
            filterList(e) {
                return e.items.filter(function (n) { return n[0] !== '.' })
            },
        })
    `)

	v, handled, err := hooks.FilterList.Call(map[string]any{
		"items": []any{"a.txt", ".hidden", "b.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !handled {
		t.Fatal("Handled must be true when a handler ran")
	}
	items, ok := v.([]any)
	if !ok {
		t.Fatalf("handler returned %T, want a list", v)
	}
	if len(items) != 2 || items[0] != "a.txt" || items[1] != "b.txt" {
		t.Fatalf("filtered = %#v, want [a.txt b.txt]", items)
	}
}

// canRead returning a bool is the per-entry narrowing path.
func TestWebDAVCanReadHandlerReturnsBool(t *testing.T) {
	hooks := bootHooks(t, `
        webdavHook({
            canRead(e) { return e.name !== 'secret.txt' },
        })
    `)

	for _, tc := range []struct {
		name string
		want bool
	}{
		{"ok.txt", true},
		{"secret.txt", false},
	} {
		v, _, err := hooks.CanRead.Call(map[string]any{"name": tc.name})
		if err != nil {
			t.Fatal(err)
		}
		if v != tc.want {
			t.Fatalf("%s: got %#v, want %v", tc.name, v, tc.want)
		}
	}
}

// All four points registered at once, from one call.
func TestWebDAVAllHookPointsRegister(t *testing.T) {
	hooks := bootHooks(t, `
        webdavHook({
            beforeWrite(e) {},
            beforeDelete(e) {},
            beforeMove(e) {},
            canRead(e) { return true },
            filterList(e) { return e.items },
        })
    `)

	for name, hp := range map[string]webdav.TSHookPoint{
		"beforeWrite":  hooks.BeforeWrite,
		"beforeDelete": hooks.BeforeDelete,
		"beforeMove":   hooks.BeforeMove,
		"canRead":      hooks.CanRead,
		"filterList":   hooks.FilterList,
	} {
		if !hp.Enabled() {
			t.Fatalf("%s must be enabled when the hook file declares it", name)
		}
	}
}

// All three ways of writing a handler must work. They stringify differently —
// method shorthand yields "beforeWrite(e) {...}", which is a method definition
// and NOT a valid standalone expression, while the function and arrow forms are
// already expressions. Shorthand is what people naturally write, and getting it
// wrong is a boot-time panic in the hook loader, not a quiet degradation.
func TestWebDAVHookAcceptsAllHandlerForms(t *testing.T) {
	hooks := bootHooks(t, `
        webdavHook({
            beforeWrite(e) { if (e.name === 'no-shorthand') { throw new Error('shorthand ran') } },
            canRead: function (e) { return e.name !== 'no-function' },
            filterList: (e) => e.items.filter(function (n) { return n !== 'no-arrow' }),
        })
    `)

	// shorthand
	if _, _, err := hooks.BeforeWrite.Call(map[string]any{"name": "no-shorthand"}); err == nil {
		t.Fatal("method-shorthand handler did not run")
	}

	// function expression
	v, _, err := hooks.CanRead.Call(map[string]any{"name": "no-function"})
	if err != nil {
		t.Fatal(err)
	}
	if v != false {
		t.Fatalf("function-expression handler returned %#v, want false", v)
	}

	// arrow function
	got, _, err := hooks.FilterList.Call(map[string]any{"items": []any{"keep", "no-arrow"}})
	if err != nil {
		t.Fatal(err)
	}
	items, ok := got.([]any)
	if !ok || len(items) != 1 || items[0] != "keep" {
		t.Fatalf("arrow handler returned %#v, want [keep]", got)
	}
}

// A typo in a hook name must fail loudly at load rather than silently never
// firing — a hook that quietly does nothing is the worst outcome here.
func TestWebDAVHookRejectsUnknownName(t *testing.T) {
	resetHookPointsForTesting()
	resetLoaderBindersForTesting()

	hooksDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooksDir, "typo.pb.ts"), []byte(`
        webdavHook({ beforeWrit(e) {} })
    `), 0o644); err != nil {
		t.Fatal(err)
	}

	webdav.RegisterTSHooks(WebDAVHostBindings(), testWebDAVSource())

	testApp, _ := tests.NewTestApp()
	t.Cleanup(testApp.Cleanup)
	pbApp := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: testApp.DataDir()})

	// The hook loader panics on a throwing hook file; recover so the test can
	// assert on the message rather than dying.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("a misspelled hook name must fail the hook file, not be ignored")
		}
		if msg := fmt.Sprint(r); !contains(msg, "unknown hook") {
			t.Fatalf("panic %q must name the unknown hook", msg)
		}
	}()

	_ = jsvm.Register(pbApp, jsvm.Config{
		HooksDir:      hooksDir,
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 1,
		OnInit:        buildJsvmOnInit(pbApp),
		OnLoaderInit:  buildJsvmOnLoaderInit(pbApp),
	})
	_ = pbApp.Bootstrap()
}

// A typo'd key must fail the WHOLE call: a valid sibling handler in the same
// webdavHook({...}) must not survive it. Validation therefore has to run
// before any handler is compiled and added — validating after (webdav's
// original order) left the valid handler registered on a hook file that
// errored, i.e. partial registration.
func TestWebDAVHookTypoRegistersNothing(t *testing.T) {
	resetHookPointsForTesting()
	resetLoaderBindersForTesting()

	hooksDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooksDir, "typo.pb.ts"), []byte(`
        webdavHook({ beforeWrite(e) {}, beforeWrit(e) {} })
    `), 0o644); err != nil {
		t.Fatal(err)
	}

	hooks := webdav.RegisterTSHooks(WebDAVHostBindings(), testWebDAVSource())

	testApp, _ := tests.NewTestApp()
	t.Cleanup(testApp.Cleanup)
	pbApp := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: testApp.DataDir()})

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("a misspelled hook name must fail the hook file, not be ignored")
			}
		}()
		_ = jsvm.Register(pbApp, jsvm.Config{
			HooksDir:      hooksDir,
			MigrationsDir: t.TempDir(),
			HooksPoolSize: 1,
			OnInit:        buildJsvmOnInit(pbApp),
			OnLoaderInit:  buildJsvmOnLoaderInit(pbApp),
		})
		_ = pbApp.Bootstrap()
	}()

	if hooks.BeforeWrite.Enabled() {
		t.Fatal("valid handler from a failed webdavHook call was registered — partial registration")
	}
}

// Two packages serving trees must not share handlers: points are namespaced by
// slug, so registering for "drive" leaves another source's points untouched.
func TestWebDAVHookPointsAreNamespacedBySlug(t *testing.T) {
	driveHooks := bootHooks(t, `
        webdavHook({ beforeWrite(e) {} })
    `)

	other := testWebDAVSource()
	other.Slug = "photos"
	other.Prefix = "/photos"
	otherHooks := webdav.RegisterTSHooks(WebDAVHostBindings(), other)

	if !driveHooks.BeforeWrite.Enabled() {
		t.Fatal("drive's beforeWrite must be enabled")
	}
	if otherHooks.BeforeWrite.Enabled() {
		t.Fatal("another source's beforeWrite must not pick up drive's handler")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle ||
		len(needle) == 0 ||
		indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
