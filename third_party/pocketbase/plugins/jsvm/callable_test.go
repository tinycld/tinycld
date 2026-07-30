package jsvm

import (
	"strings"
	"sync"
	"testing"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/tests"
)

// newTestCompiler builds a plugin + pool wired the way registerHooks does, and
// returns the Compiler plus the pool so a test can inspect executor state.
func newTestCompiler(t *testing.T, poolSize int) (Compiler, *vmsPool, func()) {
	t.Helper()

	testApp, _ := tests.NewTestApp()

	createVM := func() *sobek.Runtime {
		vm := sobek.New()
		vm.SetFieldNameMapper(FieldMapper{})
		vm.Set("$app", testApp)
		return vm
	}

	pool := newPool(poolSize, 0, createVM)
	p := &plugin{app: testApp}

	return p.newCompiler(pool), pool, testApp.Cleanup
}

func TestCallableReturnsHandlerValue(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 2)
	defer cleanup()

	fn, err := compile(`function (e) { return e.name + "!" }`)
	if err != nil {
		t.Fatal(err)
	}

	got, err := fn(map[string]any{"name": "drive"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "drive!" {
		t.Fatalf("got %#v, want %q", got, "drive!")
	}
}

// A handler that reshapes a list is the filterList shape: Go hands in a batch,
// TS hands back a subset.
func TestCallableReturnsFilteredList(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 2)
	defer cleanup()

	fn, err := compile(`function (e) { return e.items.filter(function (n) { return n[0] !== "." }) }`)
	if err != nil {
		t.Fatal(err)
	}

	got, err := fn(map[string]any{"items": []any{"a.txt", ".hidden", "b.txt"}})
	if err != nil {
		t.Fatal(err)
	}

	items, ok := got.([]any)
	if !ok {
		t.Fatalf("got %T, want []any", got)
	}
	if len(items) != 2 || items[0] != "a.txt" || items[1] != "b.txt" {
		t.Fatalf("got %#v, want [a.txt b.txt]", items)
	}
}

// A boolean return is the canRead/canWrite shape.
func TestCallableReturnsBool(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 2)
	defer cleanup()

	fn, err := compile(`function (e) { return e.name !== "secret.txt" }`)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		want bool
	}{
		{"ok.txt", true},
		{"secret.txt", false},
	} {
		got, err := fn(map[string]any{"name": tc.name})
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %#v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCallableSurfacesThrownError(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 2)
	defer cleanup()

	fn, err := compile(`function () { throw new Error("blocked by policy") }`)
	if err != nil {
		t.Fatal(err)
	}

	_, callErr := fn(map[string]any{})
	if callErr == nil {
		t.Fatal("expected an error from a throwing handler, got nil")
	}
	if !strings.Contains(callErr.Error(), "blocked by policy") {
		t.Fatalf("error %q does not carry the handler's message", callErr)
	}
}

func TestCallableRejectsEmptySource(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 1)
	defer cleanup()

	if _, err := compile(""); err == nil {
		t.Fatal("expected an error for an empty handler source")
	}
}

func TestCallableRejectsUncompilableSource(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 1)
	defer cleanup()

	if _, err := compile(`function ( {`); err == nil {
		t.Fatal("expected a compile error for malformed handler source")
	}
}

// The pool hands a VM to one caller at a time, but a Callable must be safe to
// invoke concurrently — a WebDAV PROPFIND fans out. This also exercises the
// pool's one-off factory fallback (more callers than pool slots).
func TestCallableConcurrent(t *testing.T) {
	t.Parallel()

	compile, _, cleanup := newTestCompiler(t, 2)
	defer cleanup()

	fn, err := compile(`function (e) { return e.n * 2 }`)
	if err != nil {
		t.Fatal(err)
	}

	const callers = 16
	var wg sync.WaitGroup
	results := make([]any, callers)
	errs := make([]error, callers)

	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = fn(map[string]any{"n": i})
		}(i)
	}
	wg.Wait()

	for i := 0; i < callers; i++ {
		if errs[i] != nil {
			t.Fatalf("caller %d: %v", i, errs[i])
		}
		want := int64(i * 2)
		if results[i] != want {
			t.Fatalf("caller %d: got %#v, want %d", i, results[i], want)
		}
	}
}

// $app must be restored after a call, mirroring hooksBinds' contract — a
// handler that clobbers it must not poison the executor for the next caller.
func TestCallableResetsAppGlobal(t *testing.T) {
	t.Parallel()

	compile, pool, cleanup := newTestCompiler(t, 1)
	defer cleanup()

	fn, err := compile(`function () { $app = 123; return true }`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn(map[string]any{}); err != nil {
		t.Fatal(err)
	}

	pool.run(func(vm *sobek.Runtime) error {
		val, err := vm.RunScript("verify", `$app`)
		if err != nil {
			t.Fatal(err)
		}
		if exported := val.Export(); exported == int64(123) {
			t.Fatal("$app was left clobbered by the handler")
		}
		return nil
	})
}

// __args/__result must not leak between calls: a handler reading them without
// having been passed anything must see undefined, not the previous call's data.
func TestCallableClearsScratchGlobals(t *testing.T) {
	t.Parallel()

	compile, pool, cleanup := newTestCompiler(t, 1)
	defer cleanup()

	fn, err := compile(`function (e) { return e.secret }`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fn(map[string]any{"secret": "classified"}); err != nil {
		t.Fatal(err)
	}

	pool.run(func(vm *sobek.Runtime) error {
		for _, name := range []string{"__args", "__result"} {
			val, err := vm.RunScript("verify", name)
			if err != nil {
				t.Fatal(err)
			}
			if !sobek.IsUndefined(val) {
				t.Fatalf("%s leaked between calls: %#v", name, val.Export())
			}
		}
		return nil
	})
}
