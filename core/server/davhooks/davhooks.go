// Package davhooks is the TS hook-point registration machinery shared by the
// DAV protocol packages (webdav, caldav). Each protocol declares its own
// hook-name list and JS binding name; everything else — the HostBindings
// surface, the validate-then-compile registration order, and the handler
// source normalization — is identical and lives here so the copies cannot
// drift. (They drifted once: webdav validated unknown names AFTER registering
// the valid ones, so a typo'd key in a webdavHook({...}) call left its valid
// siblings registered on a hook file that errored. Validation runs first here,
// unconditionally.)
package davhooks

import (
	"fmt"
	"strings"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
)

// TSHookPoint is the subset of coreserver.HookPoint the protocol packages
// need. Keeping it an interface here inverts the dependency — coreserver knows
// about the protocols, not the reverse.
type TSHookPoint interface {
	Enabled() bool
	Call(payload map[string]any) (value any, handled bool, err error)
}

// HostBindings is the minimal host surface a protocol package needs in order
// to register TS handlers. coreserver supplies it; the indirection keeps the
// protocol packages free of any dependency on the app composer.
type HostBindings struct {
	// Point returns (creating if needed) the named hook point. The returned
	// value must be safe to hold and cheap to test with Enabled().
	Point func(name string) TSHookPoint

	// AddHandler attaches a compiled JS handler to the named point.
	AddHandler func(name string, fn jsvm.Callable)

	// RegisterLoaderBinder installs a loader-only binding. It must run once,
	// on the hooks loader VM — not per pooled runtime.
	RegisterLoaderBinder func(func(loader *sobek.Runtime, compile jsvm.Compiler, app *pocketbase.PocketBase) error)
}

// PointName is the registry key for one source's hook point, e.g.
// "webdav.drive.beforeWrite". Namespacing by slug means two packages serving
// the same protocol never share a handler.
func PointName(protocol, slug, hook string) string {
	return protocol + "." + slug + "." + hook
}

// RegisterBinding installs the named JS global (e.g. "webdavHook") that takes
// an object of handlers keyed by names — one auditable function per protocol
// rather than one global per hook.
//
// Handlers arrive as source strings: the fork's transpile stringifies and
// recompiles each one standalone, so a handler closes over NOTHING from its
// defining module. Everything it needs must be inline.
func RegisterBinding(host HostBindings, binding, protocol, slug string, names []string) {
	host.RegisterLoaderBinder(func(loader *sobek.Runtime, compile jsvm.Compiler, _ *pocketbase.PocketBase) error {
		register := func(handlers map[string]string) error {
			// Reject unknown keys BEFORE compiling anything: a typo'd hook
			// name must fail the whole call, not leave its valid siblings
			// registered on a hook file that errored.
			for name := range handlers {
				if !isKnown(names, name) {
					return fmt.Errorf("%s: unknown hook %q (known: %v)", binding, name, names)
				}
			}
			for _, name := range names {
				source, ok := handlers[name]
				if !ok || source == "" {
					continue
				}
				fn, err := compile(NormalizeHandlerSource(name, source))
				if err != nil {
					return fmt.Errorf("%s: %s: %w", binding, name, err)
				}
				host.AddHandler(PointName(protocol, slug, name), fn)
			}
			return nil
		}
		return loader.Set(binding, register)
	})
}

// NormalizeHandlerSource makes a handler's stringified source a valid
// standalone expression.
//
// The three ways to write a handler stringify differently:
//
//	beforeWrite(e) {...}            → "beforeWrite(e) {...}"     (method shorthand)
//	beforeWrite: function (e) {...} → "function (e) {...}"       (already valid)
//	beforeWrite: (e) => {...}       → "(e) => {...}"             (already valid)
//
// Only the shorthand needs help: a method definition is not an expression, so
// compiling it verbatim is a syntax error. Prefixing "function " turns it into
// the named function expression "function beforeWrite(e) {...}".
//
// Shorthand is the form people naturally reach for, so accepting it matters
// more than the small amount of string handling it costs.
func NormalizeHandlerSource(name, source string) string {
	if strings.HasPrefix(source, name+"(") {
		return "function " + source
	}
	// Generator/async shorthand ("*name(...)", "async name(...)") would need
	// the same treatment, but a hook handler must be a plain synchronous
	// function — the runtime does not await a Promise — so those are left to
	// fail loudly at compile rather than being quietly rewritten.
	return source
}

func isKnown(names []string, name string) bool {
	for _, known := range names {
		if known == name {
			return true
		}
	}
	return false
}
