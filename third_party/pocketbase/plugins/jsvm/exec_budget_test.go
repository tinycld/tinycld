package jsvm

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// exec_budget_test.go pins the sandbox's execution budget. Sandboxed mode
// withholds capabilities, but withheld bindings do not bound COMPUTE: a
// hostile package's `while(true){}` at the top of a hook or migration file
// spins the loading goroutine forever — for a tenant, that is a boot that
// never completes and a spawn timeout burned on every retry. The budget
// interrupts any single JS execution (load-time evaluation, migration
// up/down runs, pooled handler invocations) so the failure is an error the
// host can classify, not a wedge.

// registerWithTimeout runs Register in a goroutine and fails the test if it
// neither returns nor errors within the deadline — the wedge this file exists
// to prevent.
func registerWithTimeout(t *testing.T, app core.App, config Config) error {
	t.Helper()
	errCh := make(chan error, 1)
	go func() { errCh <- Register(app, config) }()
	select {
	case err := <-errCh:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("Register never returned — a runaway script wedged the load")
		return nil
	}
}

func TestSandboxBudget_RunawayHookFileFailsLoad(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hooksDir, "spin.pb.js"),
		[]byte(`while (true) {}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err = registerWithTimeout(t, app, Config{
		HooksDir:    hooksDir,
		Sandboxed:   true,
		ExecTimeout: 250 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("a runaway hook file loaded without error")
	}
}

func TestSandboxBudget_RunawayMigrationTopLevelFailsLoad(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	migDir := filepath.Join(t.TempDir(), "pb_migrations")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migDir, "1_spin.js"),
		[]byte(`while (true) {}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err = registerWithTimeout(t, app, Config{
		MigrationsDir: migDir,
		Sandboxed:     true,
		ExecTimeout:   250 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("a runaway migration file loaded without error")
	}
}

// A migration whose UP function spins must be interrupted when it RUNS, not
// only at load: registration evaluates the file's top level, but the up/down
// callbacks execute later, against the tenant's database.
func TestSandboxBudget_RunawayMigrationUpIsInterrupted(t *testing.T) {
	// Snapshot the global migrations list — same hygiene as sandbox_test.go —
	// so this test's registration doesn't leak into other tests.
	original := core.AppMigrations
	core.AppMigrations = core.MigrationsList{}
	t.Cleanup(func() { core.AppMigrations = original })

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	migDir := filepath.Join(t.TempDir(), "pb_migrations")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(migDir, "1_up_spins.js"),
		[]byte(`migrate(() => { while (true) {} })`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := registerWithTimeout(t, app, Config{
		MigrationsDir: migDir,
		Sandboxed:     true,
		ExecTimeout:   250 * time.Millisecond,
	}); err != nil {
		t.Fatalf("registration itself should succeed (the spin is inside up): %v", err)
	}

	items := core.AppMigrations.Items()
	if len(items) != 1 {
		t.Fatalf("registered %d migrations, want 1", len(items))
	}

	errCh := make(chan error, 1)
	go func() { errCh <- items[0].Up(app) }()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("the spinning up() returned nil — it must be interrupted with an error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the spinning up() never returned — tenant boot would hang here forever")
	}
}

// A runaway handler must not wedge its pooled executor: the request errors
// out and the executor is interruptible again for the next request.
func TestSandboxBudget_RunawayHandlerIsInterrupted(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	hooksDir := filepath.Join(t.TempDir(), "pb_hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatal(err)
	}
	src := `routerAdd("GET", "/spin", (e) => { while (true) {} })`
	if err := os.WriteFile(filepath.Join(hooksDir, "main.pb.js"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := registerWithTimeout(t, app, Config{
		HooksDir:      hooksDir,
		Sandboxed:     true,
		HooksPoolSize: 1,
		ExecTimeout:   250 * time.Millisecond,
	}); err != nil {
		t.Fatalf("register: %v", err)
	}

	done := make(chan struct{})
	go func() {
		rec := serveRoute(t, app, "GET", "/spin")
		if rec.Code < 400 {
			t.Errorf("spinning handler answered %d, want an error status", rec.Code)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the spinning handler never answered — one request wedged the executor pool")
	}
}
