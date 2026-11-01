package jsvm

// Fork: a migration file's runtime does not outlive the call that needs it.
//
// Upstream builds one runtime per migration file and registers the up and
// down functions the file passes to migrate(). Those functions belong to the
// runtime, so each registered migration kept a whole runtime alive for the
// life of the app: about 0.2 MiB each, which is most of an idle app's memory
// when the app carries a hundred or more migration files. The registered
// migrations must stay callable after the first run (a later named apply or
// revert calls Up or Down again), so each one now keeps the file's source and
// builds a new runtime when it is called. See FORK.md.

import (
	"fmt"
	"unique"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/buffer"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/console"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/process"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
	"github.com/pocketbase/pocketbase/tools/template"
)

// migrateBinding is the Go side of the JS migrate(up, down) global.
type migrateBinding = func(up, down func(txApp core.App) error)

// migrationRuntimes builds the runtimes migration files run in. It holds
// what those runtimes share, so a registered migration can build one again.
type migrationRuntimes struct {
	p           *plugin
	registry    *require.Registry
	templates   *template.Registry
	absHooksDir string
}

// newVM builds a runtime with the bindings upstream's registerMigrations
// loop installs, in the same order, with migrate bound to the given function.
func (r *migrationRuntimes) newVM(migrate migrateBinding) *sobek.Runtime {
	vm := sobek.New()

	r.registry.Enable(vm)
	console.Enable(vm)
	process.Enable(vm)
	buffer.Enable(vm)

	BindCore(vm)
	BindDbx(vm)
	BindSecurity(vm)
	r.p.bindHostAccess(vm)
	BindForms(vm)
	BindMails(vm)

	r.p.setTemplate(vm, r.templates)
	vm.Set("__hooks", r.absHooksDir)

	vm.Set("migrate", migrate)

	if r.p.config.OnInit != nil {
		r.p.config.OnInit(vm)
	}
	return vm
}

// register runs the top level of one migration file and registers one
// migration for each migrate() call it makes. The runtime is dropped when
// register returns. A function the file did not pass stays nil, as upstream
// registers it.
func (r *migrationRuntimes) register(file string, content []byte) error {
	// Interned: every app that loads the same file shares one copy.
	src := unique.Make(string(content))
	target := r.p.migrationsTarget()
	calls := 0
	vm := r.newVM(func(up, down func(txApp core.App) error) {
		call := migrationCall{runtimes: r, file: file, src: src, index: calls}
		calls++
		target.Register(call.fn(up != nil, false), call.fn(down != nil, true), file)
	})
	if _, err := r.p.runScript(vm, src.Value()); err != nil {
		return fmt.Errorf("failed to run migration %s: %w", file, err)
	}
	return nil
}

// migrationCall is one migrate() call of one migration file.
type migrationCall struct {
	runtimes *migrationRuntimes
	file     string
	src      unique.Handle[string]
	index    int
}

// fn returns the function to register for the up (down false) or down side
// of the call, or nil when the file passed none.
func (c migrationCall) fn(present, down bool) func(txApp core.App) error {
	if !present {
		return nil
	}
	return func(txApp core.App) error { return c.run(txApp, down) }
}

// run builds a new runtime, runs the file's top level again to get the
// function its migrate() call passes, and calls that function within the
// execution budget.
func (c migrationCall) run(txApp core.App, down bool) error {
	var fn func(txApp core.App) error
	calls := 0
	vm := c.runtimes.newVM(func(up, dn func(txApp core.App) error) {
		if calls == c.index {
			fn = up
			if down {
				fn = dn
			}
		}
		calls++
	})
	if _, err := c.runtimes.p.runScript(vm, c.src.Value()); err != nil {
		return fmt.Errorf("failed to run migration %s: %w", c.file, err)
	}
	if fn == nil {
		return fmt.Errorf("migration %s: migrate() call %d passed no function on a second run", c.file, c.index)
	}
	return runBudgeted(vm, c.runtimes.p.execTimeout(), func() error { return fn(txApp) })
}
