package jsvm

// Fork-only behavior of the jsvm plugin. The upstream files (jsvm.go,
// binds.go, pool.go) only call into this file, so that an upstream merge
// touches as few of our lines as possible. See third_party/pocketbase/FORK.md.

import (
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"time"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
	"github.com/pocketbase/pocketbase/tools/template"
)

// defaultSandboxExecTimeout is generous for real package code — hook farms
// compile in milliseconds and migrations run in well under a second — while
// still turning a deliberate wedge into a classifiable error.
const defaultSandboxExecTimeout = 30 * time.Second

// execTimeout resolves the effective per-execution budget from the config.
func (p *plugin) execTimeout() time.Duration {
	switch {
	case p.config.ExecTimeout > 0:
		return p.config.ExecTimeout
	case p.config.ExecTimeout < 0:
		return 0
	case p.config.Sandboxed:
		return defaultSandboxExecTimeout
	default:
		return 0 // stock single-app behavior: trusted code, no budget
	}
}

// runBudgeted executes fn with a wall-clock budget on vm. The interrupt is
// cleared afterwards either way, so a pooled VM that overran once is usable
// for the next invocation.
func runBudgeted(vm *sobek.Runtime, budget time.Duration, fn func() error) error {
	if budget <= 0 {
		return fn()
	}
	timer := time.AfterFunc(budget, func() {
		vm.Interrupt(fmt.Sprintf("JS execution exceeded the %s budget", budget))
	})
	defer func() {
		timer.Stop()
		vm.ClearInterrupt()
	}()
	return fn()
}

// poolOptions carries the fork's settings for an executor pool.
type poolOptions struct {
	// budget bounds each run's execution (see Config.ExecTimeout); 0 = none.
	// Without it a single runaway handler occupies its executor forever, and
	// pool-size runaways wedge every hook in the app.
	budget time.Duration

	// programs is the optional shared compiler (see Config.ProgramSource).
	programs ProgramSource
}

func (p *plugin) poolOptions() poolOptions {
	return poolOptions{budget: p.execTimeout(), programs: p.config.ProgramSource}
}

// run executes call on vm within the pool's budget.
func (o poolOptions) run(vm *sobek.Runtime, call func(vm *sobek.Runtime) error) error {
	return runBudgeted(vm, o.budget, func() error { return call(vm) })
}

// mustCompile compiles a wrapped callback program in strict mode, through the
// pool's ProgramSource when one is set. It panics on error, like the
// sobek.MustCompile call it replaces. A nil pool compiles directly.
func (p *vmsPool) mustCompile(src string) *sobek.Program {
	var programs ProgramSource
	if p != nil {
		programs = p.tinycld.programs
	}
	pr, err := compileProgram(programs, src, true)
	if err != nil {
		panic(err)
	}
	return pr
}

// loaderInit runs the OnLoaderInit callback. Registration must happen exactly
// once, so it runs on the loader rather than in sharedBinds/OnInit (which fire
// per VM).
func (p *plugin) loaderInit(loader *sobek.Runtime, executors *vmsPool) {
	if p.config.OnLoaderInit != nil {
		p.config.OnLoaderInit(loader, p.newCompiler(executors))
	}
}

// runHookFile executes a hook file's top-level code on the loader. It compiles
// through the optional ProgramSource, in sloppy mode (strict=false) to match
// RunScript semantics, and within the execution budget so that a hostile hook
// file's top-level spin fails THIS load rather than wedging the app's boot.
func (p *plugin) runHookFile(loader *sobek.Runtime, content string) (sobek.Value, error) {
	prog, err := p.compile(content, false)
	if err != nil {
		return nil, err
	}
	var res sobek.Value
	err = runBudgeted(loader, p.execTimeout(), func() error {
		var rerr error
		res, rerr = loader.RunProgram(prog)
		return rerr
	})
	return res, err
}

// runScript executes a migration file's top-level code within the budget.
func (p *plugin) runScript(vm *sobek.Runtime, content string) (sobek.Value, error) {
	var res sobek.Value
	err := runBudgeted(vm, p.execTimeout(), func() error {
		var rerr error
		res, rerr = vm.RunScript(defaultScriptPath, content)
		return rerr
	})
	return res, err
}

// migrateFunc returns the JS `migrate(up, down)` binding for one migration
// file. It registers into Config.MigrationsList when set (core.AppMigrations
// otherwise). The up/down callbacks execute LATER (RunAllMigrations, against
// the org's DB) on this same vm, so they carry the budget with them —
// bounding only the file's top level would leave the actual migration run
// free to spin.
func (p *plugin) migrateFunc(vm *sobek.Runtime, file string) func(up, down func(txApp core.App) error) {
	budget := p.execTimeout()
	wrap := func(fn func(txApp core.App) error) func(core.App) error {
		if fn == nil || budget <= 0 {
			return fn
		}
		return func(txApp core.App) error {
			return runBudgeted(vm, budget, func() error { return fn(txApp) })
		}
	}

	target := p.config.MigrationsList
	if target == nil {
		target = &core.AppMigrations
	}

	return func(up, down func(txApp core.App) error) {
		target.Register(wrap(up), wrap(down), file)
	}
}

// migrationFiles loads the migration sources from MigrationsFS or
// MigrationsDir, transformed for the JS engine.
func (p *plugin) migrationFiles() (map[string][]byte, error) {
	if p.config.MigrationsFS != nil {
		return transformFiles(filesContentFS(p.config.MigrationsFS, p.config.MigrationsFilesPattern))
	}
	return transformFiles(filesContent(p.config.MigrationsDir, p.config.MigrationsFilesPattern))
}

// hookFiles loads the hook sources from HooksFS or HooksDir, transformed for
// the JS engine.
func (p *plugin) hookFiles() (map[string][]byte, error) {
	if p.config.HooksFS != nil {
		return transformFiles(filesContentFS(p.config.HooksFS, p.config.HooksFilesPattern))
	}
	return transformFiles(filesContent(p.config.HooksDir, p.config.HooksFilesPattern))
}

// typesDirectiveTargets returns the hook files that may get the types
// reference directive prepended. The prepend writes to HooksDir, so with
// HooksFS set there are none: HooksDir is then empty and the names would
// resolve against the process working directory.
func (p *plugin) typesDirectiveTargets(files map[string][]byte) map[string][]byte {
	if p.config.HooksFS != nil {
		return nil
	}
	return files
}

// transformFiles runs transformSource over every loaded file.
func transformFiles(files map[string][]byte, err error) (map[string][]byte, error) {
	if err != nil {
		return nil, err
	}
	for name, raw := range files {
		transformed, err := transformSource(name, raw)
		if err != nil {
			return nil, err
		}
		files[name] = transformed
	}
	return files, nil
}

// filesContentFS is filesContent over an fs.FS. It deliberately mirrors that
// function's contract — non-recursive, pattern-filtered, keyed by base
// filename — so a caller can swap the source without any behavioral
// difference. A missing or empty FS yields an empty map, matching
// filesContent's ErrNotExist handling.
func filesContentFS(fsys fs.FS, pattern string) (map[string][]byte, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}

	var exp *regexp.Regexp
	if pattern != "" {
		if exp, err = regexp.Compile(pattern); err != nil {
			return nil, err
		}
	}

	result := map[string][]byte{}

	for _, f := range entries {
		if f.IsDir() || (exp != nil && !exp.MatchString(f.Name())) {
			continue
		}

		raw, err := fs.ReadFile(fsys, f.Name())
		if err != nil {
			return nil, err
		}
		result[f.Name()] = raw
	}

	return result, nil
}

// newRequireRegistry builds the require registry for a plugin's VMs. When
// sandboxed it installs a loader that refuses every file-based require (native
// modules like process/console/buffer bypass the loader and still work), so
// untrusted code cannot require an arbitrary host path to read/execute a file.
func (p *plugin) newRequireRegistry() *require.Registry {
	if p.config.Sandboxed {
		return require.NewRegistryWithLoader(func(string) ([]byte, error) {
			return nil, require.ModuleFileDoesNotExistError
		})
	}
	return new(require.Registry)
}

// bindHostAccess installs the host-capability bindings ($os, $filepath,
// $http, $filesystem). When sandboxed it withholds them and scrubs the
// process shim instead.
func (p *plugin) bindHostAccess(vm *sobek.Runtime) {
	if p.config.Sandboxed {
		scrubProcess(vm)
		return
	}
	BindOS(vm)
	BindFilepath(vm)
	BindHTTP(vm)
	BindFilesystem(vm)
}

// bindApis installs $apis. When sandboxed $apis.static is removed: it serves
// an author-chosen host directory (os.DirFS on an arbitrary path), which is a
// raw filesystem read.
func (p *plugin) bindApis(vm *sobek.Runtime) {
	if p.config.Sandboxed {
		BindApisSandboxed(vm)
		return
	}
	BindApis(vm)
}

// BindApisSandboxed registers the $apis helpers safe for untrusted code —
// everything BindApis provides EXCEPT $apis.static (a raw filesystem read).
func BindApisSandboxed(vm *sobek.Runtime) {
	BindApis(vm)
	if obj, ok := vm.Get("$apis").(*sobek.Object); ok {
		if err := obj.Delete("static"); err != nil {
			panic(err)
		}
	}
}

// setTemplate installs $template. When sandboxed, only loadString (pure,
// in-memory) is exposed — loadFiles/loadFS read host files and are withheld.
func (p *plugin) setTemplate(vm *sobek.Runtime, reg *template.Registry) {
	if !p.config.Sandboxed {
		vm.Set("$template", reg)
		return
	}
	obj := vm.NewObject()
	obj.Set("loadString", reg.LoadString)
	vm.Set("$template", obj)
}

// scrubProcess replaces the node-compat process.env / process.argv on a
// sandboxed VM with an empty object / empty array, so untrusted code cannot read
// host environment variables or argv through the process shim after
// $os.getenv has been withheld.
func scrubProcess(vm *sobek.Runtime) {
	proc := vm.Get("process")
	obj, ok := proc.(*sobek.Object)
	if !ok || obj == nil {
		return // process shim absent; nothing to scrub
	}
	_ = obj.Set("env", vm.NewObject())
	_ = obj.Set("argv", vm.NewArray())
}
