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

func contains(haystack, needle string) bool { return strings.Contains(haystack, needle) }

// newSandboxApp registers a sandboxed jsvm plugin over a single hook file and
// returns the (already-bootstrapped) test app.
func newSandboxApp(t *testing.T, hookSrc string) *tests.TestApp {
	t.Helper()
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(hookSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	MustRegister(app, Config{HooksDir: hooksDir, Sandboxed: true})
	return app
}

// serveRoute builds the app's serve mux (firing OnServe, which registers hook
// routes) and issues one request against it.
func serveRoute(t *testing.T, app *tests.TestApp, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux, err := apis.BuildServeMux(app, apis.ServeConfig{})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestSandboxProcessEnvEmpty(t *testing.T) {
	t.Setenv("SANDBOX_SECRET", "leak-me")

	hook := `
		routerAdd('GET', '/leak', (e) => {
			return e.json(200, { secret: process.env.SANDBOX_SECRET ?? null, keys: Object.keys(process.env).length })
		})
	`
	app := newSandboxApp(t, hook)
	rec := serveRoute(t, app, "GET", "/leak")
	if rec.Code != 200 {
		t.Fatalf("route status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !contains(body, `"secret":null`) {
		t.Fatalf("expected sandboxed process.env.SANDBOX_SECRET to be null, got %s", body)
	}
	if !contains(body, `"keys":0`) {
		t.Fatalf("expected sandboxed process.env to be empty, got %s", body)
	}
}

func TestSandboxHostBindingsAbsent(t *testing.T) {
	// Each global must be undefined under Sandboxed. The hook reports typeof for
	// each dangerous global via a route.
	hook := `
		routerAdd('GET', '/caps', (e) => {
			return e.json(200, {
				os:         typeof $os,
				http:       typeof $http,
				filesystem: typeof $filesystem,
				filepath:   typeof $filepath,
			})
		})
	`
	app := newSandboxApp(t, hook)
	rec := serveRoute(t, app, "GET", "/caps")
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, cap := range []string{"os", "http", "filesystem", "filepath"} {
		want := `"` + cap + `":"undefined"`
		if !contains(rec.Body.String(), want) {
			t.Fatalf("expected $%s undefined under sandbox, got %s", cap, rec.Body.String())
		}
	}
}

func TestSandboxSafeBindingsPresent(t *testing.T) {
	// The safe subset must still work: routing already proven by the routes above;
	// assert $security (crypto) and $app (DB) are present and callable.
	hook := `
		routerAdd('GET', '/safe', (e) => {
			const token = $security.randomString(10)
			return e.json(200, { security: typeof $security, app: typeof $app, tokenLen: token.length })
		})
	`
	app := newSandboxApp(t, hook)
	rec := serveRoute(t, app, "GET", "/safe")
	if rec.Code != 200 {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{`"security":"object"`, `"app":"object"`, `"tokenLen":10`} {
		if !contains(body, want) {
			t.Fatalf("expected %s in safe-bindings body, got %s", want, body)
		}
	}
}

func TestNonSandboxedStillHasHostBindings(t *testing.T) {
	// Regression: with Sandboxed unset, $os must still be present (full API).
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := `routerAdd('GET','/caps',(e)=>e.json(200,{os:typeof $os}))`
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(hook), 0o644); err != nil {
		t.Fatal(err)
	}
	MustRegister(app, Config{HooksDir: hooksDir}) // Sandboxed defaults false
	rec := serveRoute(t, app, "GET", "/caps")
	if !contains(rec.Body.String(), `"os":"object"`) {
		t.Fatalf("expected $os present when not sandboxed, got %s", rec.Body.String())
	}
}

func TestSandboxMigrationHasNoHostBindings(t *testing.T) {
	// The jsvm `migrate` binding registers into the process-global
	// core.AppMigrations list; snapshot/restore it so this test's migration
	// (and its captured VM) doesn't leak into other tests' RunAllMigrations calls
	// (which the parallel *AppReset tests run concurrently -> data race).
	savedMigrations := core.AppMigrations
	defer func() { core.AppMigrations = savedMigrations }()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	migDir := filepath.Join(t.TempDir(), "pb_migrations")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// registerMigrations runs the migration file's TOP-LEVEL code via vm.RunScript
	// at Register() time; the migrate(up, down) call only registers callbacks (the
	// up body runs later, during RunAllMigrations). So the $os reference must sit at
	// top level to be exercised at load. Under sandbox, $os is undefined and the
	// top-level access must throw a ReferenceError, failing registration.
	mig := `$os.exec('id'); migrate((app) => {}, (app) => {})`
	if err := os.WriteFile(filepath.Join(migDir, "1700000000_evil.js"), []byte(mig), 0o644); err != nil {
		t.Fatal(err)
	}

	err = Register(app, Config{MigrationsDir: migDir, Sandboxed: true})
	if err == nil {
		t.Fatal("expected sandboxed migration referencing $os to fail registration, got nil")
	}
	if !contains(err.Error(), "os") && !contains(err.Error(), "not defined") && !contains(err.Error(), "ReferenceError") {
		t.Fatalf("expected a $os-not-defined error, got %v", err)
	}
}

func TestNonSandboxedMigrationHasHostBindings(t *testing.T) {
	// Snapshot/restore the process-global migrations list (see the sandbox
	// migration test above) so this registration doesn't leak into other tests.
	savedMigrations := core.AppMigrations
	defer func() { core.AppMigrations = savedMigrations }()

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	migDir := filepath.Join(t.TempDir(), "pb_migrations")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// References $os only INSIDE the up callback (not executed at load), so a
	// non-sandboxed load succeeds — proving $os exists in the non-sandbox path.
	mig := `migrate((app) => { const _ = typeof $os }, (app) => {})`
	if err := os.WriteFile(filepath.Join(migDir, "1700000000_ok.js"), []byte(mig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Register(app, Config{MigrationsDir: migDir}); err != nil {
		t.Fatalf("non-sandboxed migration load failed: %v", err)
	}
}

// $apis.static mounts an author-chosen host directory (os.DirFS on an arbitrary
// path) and can serve any host file — so it is withheld entirely under sandbox
// rather than merely traversal-guarded. A sandboxed hook that references it must
// fail to load (the symbol is undefined). See TestSandboxApisStaticAbsent for
// the positive assertion that the rest of $apis stays available.
func TestSandboxApisStaticUnavailable(t *testing.T) {
	root := t.TempDir()
	served := filepath.Join(root, "public")
	if err := os.MkdirAll(served, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(served, "ok.txt"), []byte("PUBLIC"), 0o644); err != nil {
		t.Fatal(err)
	}

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Referencing $apis.static runs at hook LOAD; under sandbox it is undefined,
	// so registration must return an error (a tenant cannot mount a host dir).
	hook := `routerAdd('GET','/assets/{path...}', $apis.static(` + "`" + served + "`" + `, false))`
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(hook), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Register(app, Config{HooksDir: hooksDir, Sandboxed: true}); err == nil {
		t.Fatal("SECURITY: sandboxed hook using $apis.static registered without error")
	}
}

func TestSandboxHookThrowAtLoadReturnsError(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Top-level code runs at hook LOAD. Under sandbox $os is undefined, so this
	// throws at load. It must be RETURNED as an error, not panic the process.
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(`$os.exec('id')`), 0o644); err != nil {
		t.Fatal(err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Register panicked on a load-time hook error under sandbox: %v", r)
		}
	}()
	if err := Register(app, Config{HooksDir: hooksDir, Sandboxed: true}); err == nil {
		t.Fatal("expected Register to return an error for a load-throwing sandboxed hook, got nil")
	}
}

func TestSandboxApisStaticAbsent(t *testing.T) {
	hook := `routerAdd('GET','/s',(e)=>e.json(200,{static: typeof ($apis && $apis.static)}))`
	app := newSandboxApp(t, hook)
	rec := serveRoute(t, app, "GET", "/s")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !contains(rec.Body.String(), `"static":"undefined"`) {
		t.Fatalf("expected $apis.static undefined under sandbox, got %s", rec.Body.String())
	}
}

func TestSandboxRequireCannotReadHostFile(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "creds.json")
	if err := os.WriteFile(secret, []byte(`{"key":"HOST-SECRET"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := "routerAdd('GET','/r',(e)=>{ try { const c = require(" + "`" + secret + "`" + "); return e.json(200,{leaked:c.key}) } catch (err) { return e.json(200,{blocked:true}) } })"
	app := newSandboxApp(t, hook)
	rec := serveRoute(t, app, "GET", "/r")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if contains(rec.Body.String(), "HOST-SECRET") {
		t.Fatalf("SECURITY: require read a host file under sandbox: %s", rec.Body.String())
	}
	if !contains(rec.Body.String(), `"blocked":true`) {
		t.Fatalf("expected require to be blocked, got %s", rec.Body.String())
	}
}

func TestSandboxTemplateNoFileLoad(t *testing.T) {
	secret := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(secret, []byte("TEMPLATE-SECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := "routerAdd('GET','/t',(e)=>{" +
		" const lf = typeof ($template && $template.loadFiles);" +
		" let render='';" +
		" try { render = $template.loadString('hi {{.}}').render('x') } catch (err) { render = 'ERR' }" +
		" let leaked=false;" +
		" try { const r = $template.loadFiles(" + "`" + secret + "`" + "); if (r.render({}).indexOf('TEMPLATE-SECRET')>=0) leaked=true } catch (err) {}" +
		" return e.json(200,{loadFiles: lf, render: render, leaked: leaked}) })"
	app := newSandboxApp(t, hook)
	rec := serveRoute(t, app, "GET", "/t")
	if rec.Code != 200 {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if contains(body, `"leaked":true`) || contains(body, "TEMPLATE-SECRET") {
		t.Fatalf("SECURITY: $template read a host file under sandbox: %s", body)
	}
	if !contains(body, `"loadFiles":"undefined"`) {
		t.Fatalf("expected $template.loadFiles undefined under sandbox, got %s", body)
	}
	if !contains(body, `"render":"hi x"`) {
		t.Fatalf("expected loadString to still render, got %s", body)
	}
}
