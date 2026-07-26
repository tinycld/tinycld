package coreserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/tests"
)

// End-to-end proof of the Go→TS seam: a real .pb.ts file on disk registers a
// handler through a loader binding, and Go invokes it and uses the value it
// returned. Everything below the binding — transpile, compile, pool borrow,
// value export — is the real machinery, not a stub.
//
// This is the test that would catch a regression in the fork's OnLoaderInit
// wiring, which the unit tests (which construct a Compiler directly) cannot.
func TestLoaderBinderRegistersHandlerFromDiskTS(t *testing.T) {
	resetHookPointsForTesting()
	resetLoaderBindersForTesting()

	hooksDir := t.TempDir()

	// A hook file in the real on-disk shape. Note the handler is
	// self-contained: the transpile wraps each callback and recompiles it
	// standalone, so a top-level const would NOT be in scope at call time.
	hookFile := filepath.Join(hooksDir, "webdav.pb.ts")
	err := os.WriteFile(hookFile, []byte(`
        testHook(function (e) {
            if (e.name.endsWith('.exe')) {
                return 'blocked'
            }
            return 'allowed'
        })
    `), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	hp := RegisterHookPoint("test.integration")

	RegisterLoaderBinder(func(loader *sobek.Runtime, compile jsvm.Compiler, _ *pocketbase.PocketBase) error {
		return loader.Set("testHook", func(handler string) error {
			fn, cerr := compile(handler)
			if cerr != nil {
				return cerr
			}
			hp.Add(fn)
			return nil
		})
	})

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	pbApp := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: testApp.DataDir(),
	})

	// Register jsvm the way coreserver.Register does, pointing at our hooks dir.
	err = jsvm.Register(pbApp, jsvm.Config{
		HooksDir:      hooksDir,
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 2,
		OnInit:        buildJsvmOnInit(pbApp),
		OnLoaderInit:  buildJsvmOnLoaderInit(pbApp),
	})
	if err != nil {
		t.Fatal(err)
	}

	// jsvm defers registerHooks to OnBootstrap; drive it directly.
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if !hp.Enabled() {
		t.Fatal("the .pb.ts file did not register a handler through the loader binding")
	}

	for _, tc := range []struct{ name, want string }{
		{"report.pdf", "allowed"},
		{"malware.exe", "blocked"},
	} {
		res, err := hp.Call(map[string]any{"name": tc.name})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if !res.Handled {
			t.Fatalf("%s: Handled=false, want true", tc.name)
		}
		if res.Value != tc.want {
			t.Fatalf("%s: got %#v, want %q", tc.name, res.Value, tc.want)
		}
	}
}

// Registration must happen EXACTLY once, and this asserts it at BOTH levels,
// because they fail independently:
//
//   - The binder itself must be invoked once. Installing it on the per-VM path
//     (OnInit, or an OnLoaderInit wired into sharedBinds) invokes it once per
//     pooled runtime. That alone is usually harmless — a binder typically only
//     does vm.Set — which is exactly why asserting only the handler count would
//     let the mis-wiring through unnoticed.
//   - The handler must be registered once. Anything that causes the hook FILE
//     to execute on more than one runtime would double-register, and then every
//     hook point would fire N times per request.
//
// A pool of 8 makes any per-VM leak show up as 9 rather than 1.
func TestLoaderBinderRegistersExactlyOnce(t *testing.T) {
	resetHookPointsForTesting()
	resetLoaderBindersForTesting()

	hooksDir := t.TempDir()
	err := os.WriteFile(filepath.Join(hooksDir, "once.pb.ts"), []byte(`
        countingHook(function () { return 1 })
    `), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	var binderInvocations, handlerRegistrations int
	hp := RegisterHookPoint("test.once")

	RegisterLoaderBinder(func(loader *sobek.Runtime, compile jsvm.Compiler, _ *pocketbase.PocketBase) error {
		binderInvocations++
		return loader.Set("countingHook", func(handler string) error {
			fn, cerr := compile(handler)
			if cerr != nil {
				return cerr
			}
			handlerRegistrations++
			hp.Add(fn)
			return nil
		})
	})

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	pbApp := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: testApp.DataDir(),
	})

	err = jsvm.Register(pbApp, jsvm.Config{
		HooksDir:      hooksDir,
		MigrationsDir: t.TempDir(),
		HooksPoolSize: 8,
		OnInit:        buildJsvmOnInit(pbApp),
		OnLoaderInit:  buildJsvmOnLoaderInit(pbApp),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := pbApp.Bootstrap(); err != nil {
		t.Fatal(err)
	}

	if binderInvocations != 1 {
		t.Fatalf("loader binder invoked %d times; want exactly 1 "+
			"(is OnLoaderInit wired into the per-VM path instead of the loader?)",
			binderInvocations)
	}
	if handlerRegistrations != 1 {
		t.Fatalf("handler registered %d times; want exactly 1", handlerRegistrations)
	}

	// And the consequence that actually matters at runtime: one registration
	// means one invocation per call, not N.
	var calls int
	hp2 := RegisterHookPoint("test.once.effect")
	hp2.Add(func(_ ...any) (any, error) { calls++; return nil, nil })
	if _, err := hp2.Call(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("hook fired %d times for one Call; want 1", calls)
	}
}
