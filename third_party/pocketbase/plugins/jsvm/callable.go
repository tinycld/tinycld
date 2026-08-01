package jsvm

import (
	"fmt"

	"github.com/grafana/sobek"
)

// The Go→JS callout seam.
//
// The stock jsvm plugin only runs JS that JS itself registered against a hook,
// route or cron — a Go caller can trigger those, but it cannot ask "run this
// package's handler and give me back what it returned". Host code that owns a
// request path (a protocol server, say) needs exactly that: an opt-in point
// where package TS can veto or reshape a decision the Go made.
//
// Callable closes that gap with the smallest possible surface. A registration
// function installed via Config.OnLoaderInit receives a Compiler; package TS
// calls it with a handler function; the Compiler returns a Go closure that runs
// that handler on a pooled executor VM and returns its exported value.
//
// Two properties this deliberately preserves from hooksBinds:
//
//   - The handler is compiled ONCE, at hook-load time. Per-invocation cost is a
//     pool borrow plus RunProgram — no parse, no transpile.
//   - The handler string is wrapped and recompiled standalone, so it closes over
//     NOTHING from its defining module. A handler must be self-contained. (Same
//     constraint the record-hook callbacks carry.)
//
// Registration runs on the loader VM only. That matters: Config.OnInit fires on
// every VM in the pool, so a registration binding installed there would register
// the same handler once per executor.

// Callable runs a registered JS handler and returns its exported result.
//
// The args are marshalled into the VM as `__args`, so a handler declared as
// `function (e) { … }` receives args[0] as `e`. A returned Go error value, or a
// thrown exception, surfaces as the error result.
//
// Safe for concurrent use: each call borrows its own executor from the pool.
type Callable func(args ...any) (any, error)

// Compiler turns a JS handler source string into a Go-callable closure.
//
// This is what a registration binding hands to package TS. The typical shape is
// a namespace function that takes the handler and stores the returned Callable
// somewhere the host can reach:
//
//	obj.Set("onRequest", func(handler string) error {
//	    fn, err := compile(handler)
//	    if err != nil { return err }
//	    myRegistry.Add(fn)
//	    return nil
//	})
type Compiler func(handlerSource string) (Callable, error)

// LoaderInit is called once, on the loader VM only, with a Compiler bound to
// that plugin's executor pool. Install registration bindings here rather than in
// OnInit when the binding's job is to REGISTER a handler — OnInit runs per VM and
// would register the same handler once per pool executor.
type LoaderInit func(loader *sobek.Runtime, compile Compiler)

// newCompiler builds the Compiler handed to LoaderInit. The wrapping mirrors
// hooksBinds: the handler source is embedded in a self-contained program that
// applies it to __args, so it is compiled once and closes over nothing.
func (p *plugin) newCompiler(executors *vmsPool) Compiler {
	return func(handlerSource string) (Callable, error) {
		if handlerSource == "" {
			return nil, fmt.Errorf("jsvm: empty handler source")
		}

		pr, err := p.compile("{__result = ("+handlerSource+").apply(undefined, __args)}", true)
		if err != nil {
			return nil, fmt.Errorf("jsvm: compile handler: %w", err)
		}

		return func(args ...any) (any, error) {
			var out any

			runErr := executors.run(func(executor *sobek.Runtime) error {
				oldApp := executor.Get("$app")
				executor.Set("__args", args)
				executor.Set("__result", sobek.Undefined())

				_, err := executor.RunProgram(pr)

				// Read the result back before clearing, so a handler that
				// returns a value (filterList, canRead) can hand it to Go.
				if res := executor.Get("__result"); res != nil {
					if resErr := checkGojaValueForError(p.app, res); resErr != nil {
						err = resErr
					} else if err == nil {
						out = res.Export()
					}
				}

				executor.Set("__args", sobek.Undefined())
				executor.Set("__result", sobek.Undefined())
				executor.Set("$app", oldApp)

				return normalizeException(err)
			})
			if runErr != nil {
				return nil, runErr
			}
			return out, nil
		}, nil
	}
}
