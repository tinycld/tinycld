package coreserver

import (
	"testing"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
)

// resetBinders clears the package-global binder registry so tests don't leak
// into each other. Returns a restore func.
func resetBinders(t *testing.T) {
	t.Helper()
	bindersMu.Lock()
	saved := binders
	binders = nil
	bindersMu.Unlock()
	t.Cleanup(func() {
		bindersMu.Lock()
		binders = saved
		bindersMu.Unlock()
	})
}

func TestBuildJsvmOnInit_RunsBindersAndExposesCallable(t *testing.T) {
	resetBinders(t)

	RegisterJSVMBinder(func(vm *sobek.Runtime, _ *pocketbase.PocketBase) error {
		ns, err := NewBindNamespace(vm, map[string]any{
			"echo": func(s string) string { return "got:" + s },
		})
		if err != nil {
			return err
		}
		return vm.Set("$probe", ns)
	})

	vm := sobek.New()
	buildJsvmOnInit(nil)(vm)

	v, err := vm.RunString(`$probe.echo("hi")`)
	if err != nil {
		t.Fatalf("calling bound func: %v", err)
	}
	if got := v.String(); got != "got:hi" {
		t.Fatalf("got %q, want %q", got, "got:hi")
	}
}

func TestBuildJsvmOnInit_BinderErrorPanics(t *testing.T) {
	resetBinders(t)

	RegisterJSVMBinder(func(_ *sobek.Runtime, _ *pocketbase.PocketBase) error {
		return errTestBinder
	})

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic on binder error, got none")
		}
	}()

	buildJsvmOnInit(nil)(sobek.New())
}

func TestBuildJsvmOnInit_NoBindersIsNoop(t *testing.T) {
	resetBinders(t)
	// Must not panic with an empty registry.
	buildJsvmOnInit(nil)(sobek.New())
}

var errTestBinder = testErr("boom")

type testErr string

func (e testErr) Error() string { return string(e) }
