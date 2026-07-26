package jsvm

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestIsTypeScript(t *testing.T) {
	cases := map[string]bool{
		"main.pb.ts":  true,
		"001_init.ts": true,
		"main.pb.js":  false,
		"001_init.js": false,
		"notes.txt":   false,
	}
	for name, want := range cases {
		if got := isTypeScript(name); got != want {
			t.Errorf("isTypeScript(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestTransformSource_TranspilesTS(t *testing.T) {
	// Type annotations + enum are TS-only syntax goja cannot parse.
	src := []byte("enum E { A, B }\nconst x: number = E.A\nrouterAdd('GET','/x',()=>{})")
	out, err := transformSource("main.pb.ts", src)
	if err != nil {
		t.Fatalf("transformSource: %v", err)
	}
	js := string(out)
	if strings.Contains(js, ": number") {
		t.Fatalf("type annotation not stripped: %s", js)
	}
	if !strings.Contains(js, "routerAdd") {
		t.Fatalf("expected routerAdd call preserved: %s", js)
	}
}

func TestTransformSource_PassesThroughJS(t *testing.T) {
	src := []byte("const x = 1 // plain js")
	out, err := transformSource("main.pb.js", src)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(src) {
		t.Fatalf(".js must pass through byte-identical; got %q", out)
	}
}

func TestTransformSource_WarningStillTranspiles(t *testing.T) {
	// `typeof x === "strnig"` is a typo esbuild flags as a warning (the string is
	// never a valid typeof result) but does NOT treat as an error. The transform
	// must still succeed and emit runnable JS.
	src := []byte("const x: number = 1\nif (typeof x === \"strnig\") { routerAdd('GET','/x',()=>{}) }")
	out, err := transformSource("warn.pb.ts", src)
	if err != nil {
		t.Fatalf("warning-only input must not fail: %v", err)
	}
	js := string(out)
	if strings.Contains(js, ": number") {
		t.Fatalf("type annotation not stripped: %s", js)
	}
	if !strings.Contains(js, "routerAdd") {
		t.Fatalf("expected routerAdd call preserved: %s", js)
	}
}

func TestTransformSource_EmptyPassesThrough(t *testing.T) {
	out, err := transformSource("main.pb.ts", []byte{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 0 {
		t.Fatalf("empty .pb.ts must stay empty (0 bytes); got %d bytes: %q", len(out), out)
	}
}

func TestTransformSource_SyntaxErrorIsClear(t *testing.T) {
	out, err := transformSource("bad.pb.ts", []byte("const x: = "))
	if err == nil {
		t.Fatalf("expected transpile error, got out=%q", out)
	}
	if !strings.Contains(err.Error(), "bad.pb.ts") {
		t.Fatalf("error should name the file: %v", err)
	}
}

// TestTSHook_EndToEnd proves a `.pb.ts` hook file with TS-only syntax is
// transpiled by the filesContent seam, loaded through the real hooks loader
// (Register -> registerHooks), and serves a live HTTP route.
//
// Not parallel: Register mutates process-global state (hook executors bound to
// $app, plus the shared registries) and the assertion drives a real route.
func TestTSHook_EndToEnd(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	hooksDir := filepath.Join(testApp.DataDir(), "ts_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// `interface` and the `as Payload` assertion are TS-only syntax goja cannot
	// parse; if the seam didn't transpile, registerHooks would fail to compile.
	hookSrc := `
		interface Payload { ok: boolean }
		routerAdd('GET', '/tstest', (e) => e.json(200, { ok: true } as Payload))
	`
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.ts"), []byte(hookSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// point MigrationsDir at an empty dir so only the hook under test loads
	emptyMigrations := filepath.Join(testApp.DataDir(), "ts_hooks_empty_migrations")
	if err := os.MkdirAll(emptyMigrations, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Register(testApp, Config{
		HooksDir:      hooksDir,
		MigrationsDir: emptyMigrations,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	baseRouter, err := apis.NewRouter(testApp)
	if err != nil {
		t.Fatal(err)
	}

	// trigger the serve event so the JS-registered routes are attached
	serveEvent := new(core.ServeEvent)
	serveEvent.App = testApp
	serveEvent.Router = baseRouter

	// The finalizer callback only does the serve-dependent work and stashes the
	// result into these outer-scope vars. Assertions run AFTER Trigger returns so
	// a callback that never fires fails loudly (via `served`) instead of vacuously.
	var served bool
	var recorder *httptest.ResponseRecorder
	err = testApp.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		req := httptest.NewRequest("GET", "/tstest", nil)
		recorder = httptest.NewRecorder()

		mux, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		mux.ServeHTTP(recorder, req)

		served = true
		return nil
	})
	if err != nil {
		t.Fatalf("OnServe Trigger: %v", err)
	}

	if !served {
		t.Fatal("OnServe finalizer callback never ran; assertions would have been skipped")
	}

	if recorder.Code != 200 {
		t.Fatalf("Expected status code %d, got %d (body: %q)", 200, recorder.Code, recorder.Body.String())
	}

	body := strings.TrimSpace(recorder.Body.String())
	if body != `{"ok":true}` {
		t.Fatalf("Expected body %q, got %q", `{"ok":true}`, body)
	}
}

// TestSobekNodeCompat_EndToEnd proves the vendored Node-compat modules
// (console/process/buffer, plus require) are wired and functional inside a hook
// running on the sobek engine. A `.pb.js` hook exercises the globals and a
// require(...) call, then serves them back over a live HTTP route.
//
// Not parallel: same reason as TestTSHook_EndToEnd (Register mutates
// process-global hook/registry state and the assertion drives a real route).
func TestSobekNodeCompat_EndToEnd(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	hooksDir := filepath.Join(testApp.DataDir(), "node_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// console.log proves the console module; Buffer.from proves the buffer module;
	// typeof process proves the process module; require('buffer') proves the
	// require registry resolves a vendored core module.
	//
	// Everything the handler needs is resolved INSIDE the handler body:
	// routerAdd handlers are serialized (handler.String()) and re-run in a fresh
	// pooled executor VM, so they cannot close over top-level module variables —
	// that's the upstream PocketBase design, not a sobek behavior. The vendored
	// modules are enabled on every executor VM, so require/Buffer/process/console
	// all resolve from within the handler.
	hookSrc := `
		console.log('node-compat check')
		routerAdd('GET', '/nodecheck', (e) => {
			const bufMod = require('buffer')
			return e.json(200, {
				bufLen: Buffer.from('abc').length,
				hasProcess: typeof process !== 'undefined',
				requireWorks: typeof bufMod.Buffer === 'function',
			})
		})
	`
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(hookSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	emptyMigrations := filepath.Join(testApp.DataDir(), "node_hooks_empty_migrations")
	if err := os.MkdirAll(emptyMigrations, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Register(testApp, Config{
		HooksDir:      hooksDir,
		MigrationsDir: emptyMigrations,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	baseRouter, err := apis.NewRouter(testApp)
	if err != nil {
		t.Fatal(err)
	}

	serveEvent := new(core.ServeEvent)
	serveEvent.App = testApp
	serveEvent.Router = baseRouter

	var served bool
	var recorder *httptest.ResponseRecorder
	err = testApp.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		req := httptest.NewRequest("GET", "/nodecheck", nil)
		recorder = httptest.NewRecorder()

		mux, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		mux.ServeHTTP(recorder, req)

		served = true
		return nil
	})
	if err != nil {
		t.Fatalf("OnServe Trigger: %v", err)
	}

	if !served {
		t.Fatal("OnServe finalizer callback never ran; assertions would have been skipped")
	}

	if recorder.Code != 200 {
		t.Fatalf("Expected status code %d, got %d (body: %q)", 200, recorder.Code, recorder.Body.String())
	}

	body := strings.TrimSpace(recorder.Body.String())
	want := `{"bufLen":3,"hasProcess":true,"requireWorks":true}`
	if body != want {
		t.Fatalf("Expected body %q, got %q", want, body)
	}
}

// TestES2020Feature_EndToEnd proves the sobek engine parses and executes ES2020
// syntax (optional chaining + nullish coalescing) directly. The hook is a
// `.pb.js` file so it passes through the transpile seam byte-identical (see
// TestTransformSource_PassesThroughJS) — the feature therefore runs on the
// ENGINE, not via esbuild down-leveling, which is what proves sobek handles it.
//
// Not parallel: same reason as TestTSHook_EndToEnd.
func TestES2020Feature_EndToEnd(t *testing.T) {
	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	hooksDir := filepath.Join(testApp.DataDir(), "es2020_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// `o?.a ?? 'ok'` is ES2020 optional-chaining + nullish-coalescing. On an
	// engine without ES2020 support this would fail to parse at compile time.
	hookSrc := `
		routerAdd('GET', '/es', (e) => {
			const o = {}
			return e.json(200, { v: o?.a ?? 'ok' })
		})
	`
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(hookSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	emptyMigrations := filepath.Join(testApp.DataDir(), "es2020_hooks_empty_migrations")
	if err := os.MkdirAll(emptyMigrations, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Register(testApp, Config{
		HooksDir:      hooksDir,
		MigrationsDir: emptyMigrations,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	baseRouter, err := apis.NewRouter(testApp)
	if err != nil {
		t.Fatal(err)
	}

	serveEvent := new(core.ServeEvent)
	serveEvent.App = testApp
	serveEvent.Router = baseRouter

	var served bool
	var recorder *httptest.ResponseRecorder
	err = testApp.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		req := httptest.NewRequest("GET", "/es", nil)
		recorder = httptest.NewRecorder()

		mux, err := e.Router.BuildMux()
		if err != nil {
			return err
		}
		mux.ServeHTTP(recorder, req)

		served = true
		return nil
	})
	if err != nil {
		t.Fatalf("OnServe Trigger: %v", err)
	}

	if !served {
		t.Fatal("OnServe finalizer callback never ran; assertions would have been skipped")
	}

	if recorder.Code != 200 {
		t.Fatalf("Expected status code %d, got %d (body: %q)", 200, recorder.Code, recorder.Body.String())
	}

	body := strings.TrimSpace(recorder.Body.String())
	if body != `{"v":"ok"}` {
		t.Fatalf("Expected body %q, got %q", `{"v":"ok"}`, body)
	}
}

// TestTSMigration_EndToEnd proves a `.ts` migration file with TS-only syntax is
// transpiled by the filesContent seam and runs through the real migration path.
//
// Migrations bypass p.compile and run via vm.RunScript directly, so this only
// passes if the transpile seam lives in filesContent (feeding registerMigrations),
// not on p.compile. If the hook test passes but this fails, the seam is misplaced.
//
// Not parallel: the jsvm `migrate` binding registers into the process-global
// core.AppMigrations list, so we snapshot/restore it to avoid leaking the
// migration (and its captured VM) into other tests' RunAllMigrations calls.
func TestTSMigration_EndToEnd(t *testing.T) {
	savedMigrations := core.AppMigrations
	defer func() { core.AppMigrations = savedMigrations }()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	migrationsDir := filepath.Join(testApp.DataDir(), "ts_migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// `as any` is TS-only syntax; if the seam didn't transpile, registerMigrations'
	// vm.RunScript would fail to parse this file.
	migrationSrc := `
		migrate((app) => {
			const c = new Collection({ id: 'pbc_tsmig_01', name: 'ts_widgets', type: 'base' } as any)
			app.save(c)
		}, (app) => {
			app.delete(app.findCollectionByNameOrId('ts_widgets'))
		})
	`
	if err := os.WriteFile(filepath.Join(migrationsDir, "001_ts_widgets.ts"), []byte(migrationSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	// point HooksDir at an empty dir so only the migration under test loads
	emptyHooks := filepath.Join(testApp.DataDir(), "ts_migrations_empty_hooks")
	if err := os.MkdirAll(emptyHooks, 0o755); err != nil {
		t.Fatal(err)
	}

	err := Register(testApp, Config{
		HooksDir:      emptyHooks,
		MigrationsDir: migrationsDir,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := testApp.RunAllMigrations(); err != nil {
		t.Fatalf("RunAllMigrations: %v", err)
	}

	if _, err := testApp.FindCollectionByNameOrId("ts_widgets"); err != nil {
		t.Fatalf("expected ts_widgets collection to be created by the .ts migration, got %v", err)
	}
}
