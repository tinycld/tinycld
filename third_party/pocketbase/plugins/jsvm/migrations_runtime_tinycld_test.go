package jsvm

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"weak"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// liveRuntimes records a weak pointer to every runtime the plugin builds,
// through OnInit, and counts the ones still reachable after a GC.
type liveRuntimes struct {
	mu   sync.Mutex
	refs []weak.Pointer[sobek.Runtime]
}

func (l *liveRuntimes) onInit(vm *sobek.Runtime) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.refs = append(l.refs, weak.Make(vm))
}

func (l *liveRuntimes) built() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.refs)
}

func (l *liveRuntimes) live() int {
	runtime.GC()
	runtime.GC()
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, r := range l.refs {
		if r.Value() != nil {
			n++
		}
	}
	return n
}

func writeMigration(t *testing.T, dir, name, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An app keeps its registered migrations for its whole life, so a runtime a
// migration holds is memory the app never gets back. After Register, after
// the migrations ran, and after a revert, no migration runtime is reachable,
// and up and down still do their work.
func TestMigrationRuntimes_NoneOutlivesItsCall(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		writeMigration(t, dir, "1700000020_kept_"+name+".js", `
migrate((app) => {
	app.save(new Collection({ type: "base", name: "kept_`+name+`" }))
}, (app) => {
	app.delete(app.findCollectionByNameOrId("kept_`+name+`"))
})`)
	}

	var vms liveRuntimes
	if err := Register(app, Config{
		MigrationsDir: dir,
		HooksDir:      filepath.Join(dir, ".no-hooks"),
		OnInit:        vms.onInit,
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if vms.built() == 0 {
		t.Fatal("OnInit saw no runtime: the test does not observe the migration runtimes")
	}
	if n := vms.live(); n != 0 {
		t.Fatalf("after Register, %d of %d migration runtimes are still reachable", n, vms.built())
	}

	if err := app.RunAppMigrations(); err != nil {
		t.Fatalf("RunAppMigrations: %v", err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, err := app.FindCollectionByNameOrId("kept_" + name); err != nil {
			t.Fatalf("up of kept_%s did not run: %v", name, err)
		}
	}
	if n := vms.live(); n != 0 {
		t.Fatalf("after the migrations ran, %d of %d migration runtimes are still reachable", n, vms.built())
	}

	reverted, err := core.NewMigrationsRunner(app, core.AppMigrationsOf(app)).Down(1)
	if err != nil {
		t.Fatalf("Down(1): %v", err)
	}
	if len(reverted) != 1 || reverted[0] != "1700000020_kept_c.js" {
		t.Fatalf("reverted %v, want [1700000020_kept_c.js]", reverted)
	}
	if _, err := app.FindCollectionByNameOrId("kept_c"); err == nil {
		t.Fatal("down of kept_c did not run: the collection is still there")
	}
	if n := vms.live(); n != 0 {
		t.Fatalf("after the revert, %d of %d migration runtimes are still reachable", n, vms.built())
	}
}

// A file registers one migration per migrate() call, each runs its own
// function, and a side the file did not pass stays nil (a revert refuses a
// nil Down rather than recording a revert that never ran).
func TestMigrationRuntimes_EachCallKeepsItsOwnFunctions(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)

	dir := t.TempDir()
	writeMigration(t, dir, "1700000030_two_calls.js", `
migrate((app) => { app.save(new Collection({ type: "base", name: "first_call" })) })
migrate((app) => { app.save(new Collection({ type: "base", name: "second_call" })) }, (app) => {
	app.delete(app.findCollectionByNameOrId("second_call"))
})`)

	if err := Register(app, Config{MigrationsDir: dir, HooksDir: filepath.Join(dir, ".no-hooks")}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	items := core.OwnAppMigrations(app).Items()
	if len(items) != 2 {
		t.Fatalf("registered %d migrations, want 2", len(items))
	}

	var withDown, withoutDown *core.Migration
	for _, m := range items {
		if m.Down == nil {
			withoutDown = m
		} else {
			withDown = m
		}
	}
	if withDown == nil || withoutDown == nil {
		t.Fatalf("want one migration with a down and one without, got %+v", items)
	}

	if err := withoutDown.Up(app); err != nil {
		t.Fatalf("first call's up: %v", err)
	}
	if _, err := app.FindCollectionByNameOrId("first_call"); err != nil {
		t.Fatalf("first call's up did not run its own function: %v", err)
	}
	if _, err := app.FindCollectionByNameOrId("second_call"); err == nil {
		t.Fatal("first call's up ran the second call's function")
	}

	if err := withDown.Up(app); err != nil {
		t.Fatalf("second call's up: %v", err)
	}
	if err := withDown.Down(app); err != nil {
		t.Fatalf("second call's down: %v", err)
	}
	if _, err := app.FindCollectionByNameOrId("second_call"); err == nil {
		t.Fatal("second call's down did not run")
	}
	if _, err := app.FindCollectionByNameOrId("first_call"); err != nil {
		t.Fatalf("second call's down removed the first call's collection: %v", err)
	}
}
