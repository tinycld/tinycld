// Package jsvm implements pluggable utilities for binding a JS sobek runtime
// to the PocketBase instance (loading migrations, attaching to app hooks, etc.).
//
// The package also exports several reusable bindings so that users
// can utilize them as part of their own custom sobek runtime setup.
//
// Example:
//
//	jsvm.MustRegister(app, jsvm.Config{
//		HooksWatch: true,
//	})
package jsvm

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/fsnotify/fsnotify"
	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/buffer"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/console"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/process"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/types/generated"
	"github.com/pocketbase/pocketbase/tools/routine"
	"github.com/pocketbase/pocketbase/tools/template"
)

const typesFileName = "types.d.ts"

var defaultScriptPath = "pb.js"

func init() {
	// For backward compatibility and consistency with the Go exposed
	// methods that operate with relative paths (e.g. `$os.writeFile`),
	// we define the "current JS module" as if it is a file in the current working directory
	// (the filename itself doesn't really matter and in our case the hook handlers are executed as separate "programs").
	//
	// This is necessary for `require(module)` to properly traverse parents node_modules (goja_nodejs#95).
	cwd, err := os.Getwd()
	if err != nil {
		// truly rare case, log just for debug purposes
		color.Yellow("Failed to retrieve the current working directory: %v", err)
	} else {
		defaultScriptPath = filepath.Join(cwd, defaultScriptPath)
	}
}

// Config defines the config options of the jsvm plugin.
type Config struct {
	// OnInit is an optional function that will be called
	// after a JS runtime is initialized, allowing you to
	// attach custom Go variables and functions.
	//
	// Note this fires on EVERY runtime — the loader and every executor in
	// the hooks pool. Use it for stateless bindings (a namespace of native
	// funcs). For a binding whose job is to REGISTER something, use
	// OnLoaderInit instead, or the same handler is registered once per VM.
	OnInit func(vm *sobek.Runtime)

	// OnLoaderInit is an optional function called once, on the hooks LOADER
	// runtime only, with a Compiler bound to that plugin's executor pool.
	//
	// It is the Go→JS seam: install a binding here that takes a JS handler
	// and hands the resulting Callable to host code, so Go can invoke package
	// TS at a defined point and use what it returns. See callable.go.
	OnLoaderInit LoaderInit

	// ProgramSource is an optional hook to supply/share compiled sobek programs
	// across plugin instances. If nil, programs are compiled directly with sobek
	// (the default, single-app behavior).
	ProgramSource ProgramSource

	// Sandboxed, when true, installs only the capability-safe JS bindings and
	// omits the host-capability bindings ($os, $http, $filesystem, $filepath)
	// from BOTH the hook and migration runtimes, and neuters process.env /
	// process.argv. Intended for running untrusted (multi-tenant) code.
	//
	// Default false preserves the full stock single-app API (byte-for-byte).
	Sandboxed bool

	// ExecTimeout bounds every single JS execution: load-time evaluation of
	// hook and migration files, registered migration up/down runs, and each
	// pooled handler invocation. Withheld bindings do not bound COMPUTE — a
	// `while(true){}` at the top of a hostile package's hook file otherwise
	// spins the loading goroutine forever, which for a tenant is a boot that
	// never completes.
	//
	// Zero picks the default: 30s when Sandboxed, unlimited otherwise (stock
	// behavior). Negative disables the budget explicitly.
	ExecTimeout time.Duration

	// HooksWatch enables auto app restarts when a JS app hook file changes.
	//
	// Note that currently the application cannot be automatically restarted on Windows
	// because the restart process relies on execve.
	HooksWatch bool

	// HooksDir specifies the JS app hooks directory.
	//
	// If not set it fallbacks to a relative "pb_data/../pb_hooks" directory.
	HooksDir string

	// HooksFilesPattern specifies a regular expression pattern that
	// identify which file to load by the hook vm(s).
	//
	// If not set it fallbacks to `^.*(\.pb\.js|\.pb\.ts)$`, aka. any
	// HookdsDir file ending in ".pb.js" or ".pb.ts" (the last one is to enforce IDE linters).
	HooksFilesPattern string

	// HooksPoolSize specifies how many sobek.Runtime instances to prewarm
	// and keep for the JS app hooks gorotines execution.
	//
	// Zero or negative value means that it will create a new sobek.Runtime
	// on every fired goroutine.
	HooksPoolSize int

	// MigrationsDir specifies the JS migrations directory.
	//
	// If not set it fallbacks to a relative "pb_data/../pb_migrations" directory.
	MigrationsDir string

	// If not set it fallbacks to `^.*(\.js|\.ts)$`, aka. any MigrationDir file
	// ending in ".js" or ".ts" (the last one is to enforce IDE linters).
	MigrationsFilesPattern string

	// TypesDir specifies the directory where to store the embedded
	// TypeScript declarations file.
	//
	// If not set it fallbacks to "pb_data".
	//
	// Note: Avoid using the same directory as the HooksDir when HooksWatch is enabled
	// to prevent unnecessary app restarts when the types file is initially created.
	TypesDir string
}

// MustRegister registers the jsvm plugin in the provided app instance
// and panics if it fails.
//
// Example usage:
//
//	jsvm.MustRegister(app, jsvm.Config{
//		OnInit: func(vm *sobek.Runtime) {
//			// register custom bindings
//			vm.Set("myCustomVar", 123)
//		},
//	})
func MustRegister(app core.App, config Config) {
	if err := Register(app, config); err != nil {
		panic(err)
	}
}

// Register registers the jsvm plugin in the provided app instance.
func Register(app core.App, config Config) error {
	p := &plugin{app: app, config: config}

	if p.config.HooksDir == "" {
		p.config.HooksDir = filepath.Join(app.DataDir(), "../pb_hooks")
	}

	if p.config.MigrationsDir == "" {
		p.config.MigrationsDir = filepath.Join(app.DataDir(), "../pb_migrations")
	}

	if p.config.HooksFilesPattern == "" {
		p.config.HooksFilesPattern = `^.*(\.pb\.js|\.pb\.ts)$`
	}

	if p.config.MigrationsFilesPattern == "" {
		p.config.MigrationsFilesPattern = `^.*(\.js|\.ts)$`
	}

	if p.config.TypesDir == "" {
		p.config.TypesDir = app.DataDir()
	}

	p.app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		err := e.Next()
		if err != nil {
			return err
		}

		// ensure that the user has the latest types declaration
		err = p.refreshTypesFile()
		if err != nil {
			color.Yellow("Unable to refresh app types file: %v", err)
		}

		return nil
	})

	if err := p.registerMigrations(); err != nil {
		return fmt.Errorf("registerMigrations: %w", err)
	}

	if err := p.registerHooks(); err != nil {
		return fmt.Errorf("registerHooks: %w", err)
	}

	return nil
}

type plugin struct {
	app    core.App
	config Config
}

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

// newRequireRegistry builds the require registry for a plugin's VMs. When
// sandboxed it installs a loader that refuses every file-based require (native
// modules like process/console/buffer bypass the loader and still work), so
// untrusted code cannot require an arbitrary host path to read/execute a file.
func newRequireRegistry(sandboxed bool) *require.Registry {
	if sandboxed {
		return require.NewRegistryWithLoader(func(string) ([]byte, error) {
			return nil, require.ModuleFileDoesNotExistError
		})
	}
	return new(require.Registry)
}

// setTemplateBinding installs $template. When sandboxed, only loadString (pure,
// in-memory) is exposed — loadFiles/loadFS read host files and are withheld.
func setTemplateBinding(vm *sobek.Runtime, reg *template.Registry, sandboxed bool) {
	if !sandboxed {
		vm.Set("$template", reg)
		return
	}
	obj := vm.NewObject()
	obj.Set("loadString", reg.LoadString)
	vm.Set("$template", obj)
}

// registerMigrations registers the JS migrations loader.
func (p *plugin) registerMigrations() error {
	// fetch all js migrations sorted by their filename
	files, err := filesContent(p.config.MigrationsDir, p.config.MigrationsFilesPattern)
	if err != nil {
		return err
	}

	absHooksDir, err := filepath.Abs(p.config.HooksDir)
	if err != nil {
		return err
	}

	registry := newRequireRegistry(p.config.Sandboxed) // this can be shared by multiple runtimes
	templateRegistry := template.NewRegistry()

	for file, content := range files {
		vm := sobek.New()

		registry.Enable(vm)
		console.Enable(vm)
		process.Enable(vm)
		buffer.Enable(vm)

		if p.config.Sandboxed {
			scrubProcess(vm)
		}

		BindCore(vm)
		BindDbx(vm)
		BindSecurity(vm)
		if !p.config.Sandboxed {
			BindOS(vm)
			BindFilepath(vm)
			BindHTTP(vm)
			BindFilesystem(vm)
		}
		BindForms(vm)
		BindMails(vm)

		setTemplateBinding(vm, templateRegistry, p.config.Sandboxed)
		vm.Set("__hooks", absHooksDir)

		// The up/down callbacks execute LATER (RunAllMigrations, against the
		// tenant's DB) on this same vm, so they carry the budget with them —
		// bounding only the file's top level would leave the actual migration
		// run free to spin.
		budget := p.execTimeout()
		wrapMigration := func(fn func(txApp core.App) error) func(core.App) error {
			if fn == nil || budget <= 0 {
				return fn
			}
			return func(txApp core.App) error {
				return runBudgeted(vm, budget, func() error { return fn(txApp) })
			}
		}
		vm.Set("migrate", func(up, down func(txApp core.App) error) {
			core.AppMigrations.Register(wrapMigration(up), wrapMigration(down), file)
		})

		if p.config.OnInit != nil {
			p.config.OnInit(vm)
		}

		err := runBudgeted(vm, budget, func() error {
			_, rerr := vm.RunScript(defaultScriptPath, string(content))
			return rerr
		})
		if err != nil {
			return fmt.Errorf("failed to run migration %s: %w", file, err)
		}
	}

	return nil
}

// registerHooks registers the JS app hooks loader.
func (p *plugin) registerHooks() error {
	// fetch all js hooks sorted by their filename
	files, err := filesContent(p.config.HooksDir, p.config.HooksFilesPattern)
	if err != nil {
		return err
	}

	// prepend the types reference directive
	//
	// note: it is loaded during startup to handle conveniently also
	// the case when the HooksWatch option is enabled and the application
	// restart on newly created file
	for name, content := range files {
		if len(content) != 0 {
			// skip non-empty files for now to prevent accidental overwrite
			continue
		}
		path := filepath.Join(p.config.HooksDir, name)
		directive := `/// <reference path="` + p.relativeTypesPath(p.config.HooksDir) + `" />`
		if err := prependToEmptyFile(path, directive+"\n\n"); err != nil {
			color.Yellow("Unable to prepend the types reference: %v", err)
		}
	}

	// initialize the hooks dir watcher
	if p.config.HooksWatch {
		if err := p.watchHooks(); err != nil {
			color.Yellow("Unable to init hooks watcher: %v", err)
		}
	}

	if len(files) == 0 {
		// no need to register the vms since there are no entrypoint files anyway
		return nil
	}

	absHooksDir, err := filepath.Abs(p.config.HooksDir)
	if err != nil {
		return err
	}

	p.app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.BindFunc(p.normalizeServeExceptions)

		return e.Next()
	})

	// safe to be shared across multiple vms
	requireRegistry := newRequireRegistry(p.config.Sandboxed)
	templateRegistry := template.NewRegistry()

	sharedBinds := func(vm *sobek.Runtime) {
		requireRegistry.Enable(vm)
		console.Enable(vm)
		process.Enable(vm)
		buffer.Enable(vm)

		if p.config.Sandboxed {
			scrubProcess(vm)
		}

		BindCore(vm)
		BindDbx(vm)
		BindSecurity(vm)
		if !p.config.Sandboxed {
			BindOS(vm)
			BindFilepath(vm)
			BindHTTP(vm)
			BindFilesystem(vm)
		}
		BindForms(vm)
		BindMails(vm)
		if p.config.Sandboxed {
			BindApisSandboxed(vm)
		} else {
			BindApis(vm)
		}

		vm.Set("$app", p.app)
		setTemplateBinding(vm, templateRegistry, p.config.Sandboxed)
		vm.Set("__hooks", absHooksDir)

		if p.config.OnInit != nil {
			p.config.OnInit(vm)
		}
	}

	// initiliaze the executor vms
	executors := newPool(p.config.HooksPoolSize, p.execTimeout(), func() *sobek.Runtime {
		executor := sobek.New()
		sharedBinds(executor)
		return executor
	})

	// initialize the loader vm
	loader := sobek.New()
	sharedBinds(loader)
	p.hooksBinds(loader, executors)
	p.cronBinds(loader, executors)
	p.routerBinds(loader, executors)

	// Loader-only bindings. Registration must happen exactly once, so this
	// runs here rather than in sharedBinds/OnInit (which fire per VM).
	if p.config.OnLoaderInit != nil {
		p.config.OnLoaderInit(loader, p.newCompiler(executors))
	}

	if err := p.compileHookFiles(loader, files); err != nil {
		return err
	}

	return nil
}

// compileHookFiles executes each hook file's top-level code on the provided
// loader vm, compiling via p.compile so an optional ProgramSource can share the
// programs. Hook files compile in sloppy mode (strict=false) to match sobek's
// RunScript semantics. The panic/recover behavior mirrors the original inline
// loop (HooksWatch => log, else => panic).
func (p *plugin) compileHookFiles(loader *sobek.Runtime, files map[string][]byte) error {
	var loadErr error
	for file, content := range files {
		func() {
			defer func() {
				if r := recover(); r != nil {
					fmtErr := fmt.Errorf("failed to execute %s:\n - %v", file, r)
					switch {
					case p.config.Sandboxed:
						// Untrusted code: a load-time throw must fail this app's
						// registration (returned to the caller), never panic the
						// shared multi-tenant process.
						if loadErr == nil {
							loadErr = fmtErr
						}
					case p.config.HooksWatch:
						color.Red("%v", fmtErr)
					default:
						panic(fmtErr)
					}
				}
			}()

			prog, cerr := p.compile(string(content), false)
			if cerr != nil {
				panic(cerr)
			}
			// Budgeted per file: a hostile hook file's top-level spin must
			// fail THIS load, not wedge the whole app's boot.
			rerr := runBudgeted(loader, p.execTimeout(), func() error {
				_, err := loader.RunProgram(prog)
				return err
			})
			if rerr != nil {
				panic(rerr)
			}
		}()
		if loadErr != nil {
			return loadErr
		}
	}
	return loadErr
}

// normalizeExceptions registers a global error handler that
// wraps the extracted sobek exception error value for consistency
// when throwing or returning errors.
func (p *plugin) normalizeServeExceptions(e *core.RequestEvent) error {
	err := e.Next()

	if err == nil || e.Written() {
		return err // no error or already committed
	}

	return normalizeException(err)
}

// watchHooks initializes a hooks file watcher that will restart the
// application (*if possible) in case of a change in the hooks directory.
//
// This method does nothing if the hooks directory is missing.
func (p *plugin) watchHooks() error {
	watchDir := p.config.HooksDir

	hooksDirInfo, err := os.Lstat(p.config.HooksDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil // no hooks dir to watch
		}
		return err
	}

	if hooksDirInfo.Mode()&os.ModeSymlink == os.ModeSymlink {
		watchDir, err = filepath.EvalSymlinks(p.config.HooksDir)
		if err != nil {
			return fmt.Errorf("failed to resolve hooksDir symlink: %w", err)
		}
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	var debounceTimer *time.Timer

	stopDebounceTimer := func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
			debounceTimer = nil
		}
	}

	p.app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		watcher.Close()

		stopDebounceTimer()

		return e.Next()
	})

	// start listening for events.
	routine.FireAndForget(func() {
		defer stopDebounceTimer()

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				stopDebounceTimer()

				debounceTimer = time.AfterFunc(50*time.Millisecond, func() {
					// app restart is currently not supported on Windows
					if runtime.GOOS == "windows" {
						color.Yellow("File %s changed, please restart the app manually", event.Name)
					} else {
						color.Yellow("File %s changed, restarting...", event.Name)
						if err := p.app.Restart(); err != nil {
							color.Red("Failed to restart the app:", err)
						}
					}
				})
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				color.Red("Watch error:", err)
			}
		}
	})

	// add directories to watch
	//
	// @todo replace once recursive watcher is added (https://github.com/fsnotify/fsnotify/issues/18)
	dirsErr := filepath.WalkDir(watchDir, func(path string, entry fs.DirEntry, err error) error {
		// ignore access failures and non-dir entries
		if err != nil || !entry.IsDir() {
			return nil
		}

		// skip traversing hidden directories and node_modules
		if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules" {
			return filepath.SkipDir
		}

		return watcher.Add(path)
	})
	if dirsErr != nil {
		watcher.Close()
	}

	return dirsErr
}

// fullTypesPathReturns returns the full path to the generated TS file.
func (p *plugin) fullTypesPath() string {
	return filepath.Join(p.config.TypesDir, typesFileName)
}

// relativeTypesPath returns a path to the generated TS file relative
// to the specified basepath.
//
// It fallbacks to the full path if generating the relative path fails.
func (p *plugin) relativeTypesPath(basepath string) string {
	fullPath := p.fullTypesPath()

	rel, err := filepath.Rel(basepath, fullPath)
	if err != nil {
		// fallback to the full path
		rel = fullPath
	}

	return rel
}

// refreshTypesFile saves the embedded TS declarations as a file on the disk.
func (p *plugin) refreshTypesFile() error {
	fullPath := p.fullTypesPath()

	// ensure that the types directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}

	// retrieve the types data to write
	data, err := generated.Types.ReadFile(typesFileName)
	if err != nil {
		return err
	}

	// read the first timestamp line of the old file (if exists) and compare it to the embedded one
	// (note: ignore errors to allow always overwriting the file if it is invalid)
	existingFile, err := os.Open(fullPath)
	if err == nil {
		timestamp := make([]byte, 13)
		io.ReadFull(existingFile, timestamp)
		existingFile.Close()

		if len(data) >= len(timestamp) && bytes.Equal(data[:13], timestamp) {
			return nil // nothing new to save
		}
	}

	return os.WriteFile(fullPath, data, 0644)
}

// prependToEmptyFile prepends the specified text to an empty file.
//
// If the file is not empty this method does nothing.
func prependToEmptyFile(path, text string) error {
	info, err := os.Stat(path)

	if err == nil && info.Size() == 0 {
		return os.WriteFile(path, []byte(text), 0644)
	}

	return err
}

// filesContent returns a map with all direct files within the specified dir and their content.
//
// If directory with dirPath is missing or no files matching the pattern were found,
// it returns an empty map and no error.
//
// If pattern is empty string it matches all root files.
func filesContent(dirPath string, pattern string) (map[string][]byte, error) {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string][]byte{}, nil
		}
		return nil, err
	}

	var exp *regexp.Regexp
	if pattern != "" {
		var err error
		if exp, err = regexp.Compile(pattern); err != nil {
			return nil, err
		}
	}

	result := map[string][]byte{}

	for _, f := range files {
		if f.IsDir() || (exp != nil && !exp.MatchString(f.Name())) {
			continue
		}

		raw, err := os.ReadFile(filepath.Join(dirPath, f.Name()))
		if err != nil {
			return nil, err
		}

		transformed, err := transformSource(f.Name(), raw)
		if err != nil {
			return nil, err
		}
		result[f.Name()] = transformed
	}

	return result, nil
}

// scrubProcess replaces the node-compat process.env / process.argv on a
// sandboxed VM with an empty object / empty array, so untrusted code cannot read
// host environment variables (e.g. MT_SUPERUSER_PASSWORD) or argv through the
// process shim after $os.getenv has been withheld.
func scrubProcess(vm *sobek.Runtime) {
	proc := vm.Get("process")
	obj, ok := proc.(*sobek.Object)
	if !ok || obj == nil {
		return // process shim absent; nothing to scrub
	}
	_ = obj.Set("env", vm.NewObject())
	_ = obj.Set("argv", vm.NewArray())
}
