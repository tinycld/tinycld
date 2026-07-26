package jsvm

import (
	"fmt"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/fatih/color"
)

// isTypeScript reports whether a hook/migration filename is TypeScript source
// that must be transpiled before it reaches the JS engine. Matches the ".ts" and
// ".pb.ts" files already accepted by the Hooks/Migrations file patterns.
func isTypeScript(name string) bool {
	return strings.HasSuffix(name, ".ts")
}

// transformSource returns runnable JS for a hook/migration file. TypeScript files
// are transpiled via esbuild (in-process, no CGO); .js files pass through
// byte-for-byte. Called from filesContent so BOTH the hooks and the migrations
// load paths get it (migrations bypass p.compile — see jsvm.go).
func transformSource(name string, content []byte) ([]byte, error) {
	// Keep empty input empty: esbuild would emit a ~175-byte inline-sourcemap stub
	// for "", but registerHooks (jsvm.go ~L255) relies on an empty .pb.ts staying
	// 0 bytes to fire its types.d.ts-directive bootstrap for freshly-created dev
	// hook files. The router's publish-time transpileForStore intentionally has NO
	// such guard (the store is production; that dev bootstrap doesn't apply there).
	if len(content) == 0 {
		return content, nil
	}
	if !isTypeScript(name) {
		return content, nil
	}
	res := api.Transform(string(content), api.TransformOptions{
		Loader:    api.LoaderTS,
		Target:    api.ES2020,
		Sourcemap: api.SourceMapInline,
	})
	if len(res.Errors) > 0 {
		msgs := make([]string, 0, len(res.Errors))
		for _, e := range res.Errors {
			msgs = append(msgs, e.Text)
		}
		return nil, fmt.Errorf("transpile %s: %s", name, strings.Join(msgs, "; "))
	}
	for _, w := range res.Warnings {
		color.Yellow("transpile warning %s: %s", name, w.Text)
	}
	return res.Code, nil
}
