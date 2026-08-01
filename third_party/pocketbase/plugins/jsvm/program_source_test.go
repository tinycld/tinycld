package jsvm

import (
	"testing"

	"github.com/grafana/sobek"
)

// countingSource records how many times Compile was invoked per source string,
// mimicking a shared cache that compiles once per unique source.
type countingSource struct {
	calls map[string]int
	cache map[string]*sobek.Program
}

func newCountingSource() *countingSource {
	return &countingSource{calls: map[string]int{}, cache: map[string]*sobek.Program{}}
}

func (c *countingSource) Compile(name, src string, strict bool) (*sobek.Program, error) {
	c.calls[src]++
	if p, ok := c.cache[src]; ok {
		return p, nil
	}
	p, err := sobek.Compile(name, src, strict)
	if err != nil {
		return nil, err
	}
	c.cache[src] = p
	return p, nil
}

// TestRegisterHooks_UsesProgramSourceForFiles verifies that when a ProgramSource
// is configured, the loader compiles hook FILE content through it (not directly).
func TestRegisterHooks_UsesProgramSourceForFiles(t *testing.T) {
	src := newCountingSource()
	p := &plugin{config: Config{ProgramSource: src}}
	loader := sobek.New()

	if err := p.compileHookFiles(loader, map[string][]byte{"main.pb.js": []byte("var noop = 1")}); err != nil {
		t.Fatalf("compileHookFiles error: %v", err)
	}
	if src.calls["var noop = 1"] == 0 {
		t.Fatal("expected the hook file source to be compiled via ProgramSource")
	}
}

// TestCompileHookFiles_PreservesSloppyMode ensures hook files compile in sloppy
// mode (like goja's RunScript), so pre-existing sloppy-mode hook files (implicit
// globals, octal literals) still load. A strict-mode compile would reject these.
func TestCompileHookFiles_PreservesSloppyMode(t *testing.T) {
	p := &plugin{config: Config{}} // nil ProgramSource -> direct sobek.Compile
	loader := sobek.New()
	// Octal literal is a SyntaxError in strict mode, legal in sloppy mode.
	err := p.compileHookFiles(loader, map[string][]byte{"legacy.pb.js": []byte("var x = 0777")})
	if err != nil {
		t.Fatalf("expected sloppy-mode hook file to compile, got: %v", err)
	}
}

func TestCompileHelper_NilProgramSourceFallsBackToDirectCompile(t *testing.T) {
	p := &plugin{config: Config{}} // ProgramSource nil
	prog, err := p.compile("1 + 1", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prog == nil {
		t.Fatal("expected a compiled program, got nil")
	}
}

func TestCompileHelper_RoutesThroughProgramSourceAndDedupes(t *testing.T) {
	src := newCountingSource()
	p := &plugin{config: Config{ProgramSource: src}}

	if _, err := p.compile("2 + 2", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := p.compile("2 + 2", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := src.calls["2 + 2"]; got != 2 {
		t.Fatalf("expected ProgramSource.Compile invoked 2 times for the source, got %d", got)
	}
	if src.cache["2 + 2"] == nil {
		t.Fatal("expected the source string to be cached by the ProgramSource")
	}
}

func TestCompileHelper_ProducesRunnableProgram(t *testing.T) {
	p := &plugin{config: Config{ProgramSource: newCountingSource()}}
	prog, err := p.compile("40 + 2", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vm := sobek.New()
	v, err := vm.RunProgram(prog)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if v.ToInteger() != 42 {
		t.Fatalf("expected 42, got %v", v.Export())
	}
}

// TestBinds_CompileCallbacksViaProgramSourceStrict verifies the wrapped callback
// program shape used by the bind sites routes through ProgramSource and compiles
// in strict mode (matching the original sobek.MustCompile(..., true) behavior).
func TestBinds_CompileCallbacksViaProgramSourceStrict(t *testing.T) {
	src := newCountingSource()
	p := &plugin{config: Config{ProgramSource: src}}

	// This is the exact wrapped shape the bind sites build.
	wrapped := "{(" + "function(e){return e}" + ").apply(undefined, __args)}"
	if _, err := p.compile(wrapped, true); err != nil {
		t.Fatalf("compile error: %v", err)
	}
	if src.calls[wrapped] != 1 {
		t.Fatalf("expected wrapped callback compiled via ProgramSource once, got %d", src.calls[wrapped])
	}

	// A strict-mode-only rejection must occur when strict=true is honored:
	// duplicate parameter names are a SyntaxError in strict mode, legal in sloppy.
	if _, err := p.compile("{(function(a, a){}).apply(undefined, __args)}", true); err == nil {
		t.Fatal("expected strict-mode compile of duplicate params to fail")
	}
}

// TestSharedProgram_IsolatesRuntimeGlobals proves a single compiled program run
// on two runtimes observes each runtime's own $app, not shared state — the
// invariant that makes cross-instance program sharing (via ProgramSource) safe.
func TestSharedProgram_IsolatesRuntimeGlobals(t *testing.T) {
	src := newCountingSource()
	p := &plugin{config: Config{ProgramSource: src}}

	prog, err := p.compile("$app", true)
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	vmA := sobek.New()
	vmA.Set("$app", "APP_A")
	vmB := sobek.New()
	vmB.Set("$app", "APP_B")

	rA, err := vmA.RunProgram(prog)
	if err != nil {
		t.Fatalf("run A: %v", err)
	}
	rB, err := vmB.RunProgram(prog)
	if err != nil {
		t.Fatalf("run B: %v", err)
	}

	if rA.String() != "APP_A" {
		t.Fatalf("runtime A saw %q, expected APP_A", rA.String())
	}
	if rB.String() != "APP_B" {
		t.Fatalf("runtime B saw %q, expected APP_B", rB.String())
	}
	// Same source compiled once by the shared source (proves the *Program is shared).
	if src.calls["$app"] != 1 {
		t.Fatalf("expected 1 compile for shared source, got %d", src.calls["$app"])
	}
}
