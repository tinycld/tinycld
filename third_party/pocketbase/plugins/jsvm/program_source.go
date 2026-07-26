package jsvm

import "github.com/grafana/sobek"

// ProgramSource is an optional hook that lets an embedder supply (and typically
// cache/share) compiled sobek programs across multiple plugin instances.
//
// It is intentionally generic: it has no knowledge of multi-tenancy. When a
// Config leaves ProgramSource nil (the default), the plugin compiles each
// program directly with sobek, preserving the original single-app behavior.
//
// An implementation typically keys its cache on (src, strict) so that identical
// program sources compiled with the same strictness across plugin instances
// resolve to a single compiled *sobek.Program.
type ProgramSource interface {
	Compile(name, src string, strict bool) (*sobek.Program, error)
}

// compile returns a compiled program for the given JS source, routing through
// p.config.ProgramSource when set and falling back to direct sobek compilation
// otherwise. strict selects ECMAScript strict mode: hook FILES compile sloppy
// (strict=false, matching sobek's RunScript) while wrapped callback programs
// compile strict (strict=true, matching the existing MustCompile(..., true) sites).
func (p *plugin) compile(src string, strict bool) (*sobek.Program, error) {
	if p.config.ProgramSource != nil {
		return p.config.ProgramSource.Compile(defaultScriptPath, src, strict)
	}
	return sobek.Compile(defaultScriptPath, src, strict)
}
