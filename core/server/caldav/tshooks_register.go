package caldav

import (
	"fmt"
	"strings"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
)

// Registering the TS hook points.
//
// The four points a package can hook are declared here, named per source slug
// so two packages serving calendars never share a handler. The JS-facing
// binding is a single `caldavHook({...})` call that takes an object of
// handlers, which keeps the surface one auditable function rather than four
// globals.
//
// The wiring is deliberately indirect. core/caldav must not import coreserver
// (coreserver composes the whole app, so that would be a cycle), and coreserver
// must not know each protocol's hook names. So the host injects two small
// function values — a registrar and a point factory — and this file does the
// rest. HostBindings below is what coreserver passes in.

// hookPointNames are the JS keys a handler object may carry, in the order the
// request path encounters them.
var hookPointNames = []string{
	"beforeWrite",
	"beforeDelete",
	"canRead",
	"filterList",
}

// HostBindings is the minimal host surface this package needs in order to
// register TS handlers. coreserver supplies it; the indirection keeps
// core/caldav free of any dependency on the app composer.
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

// pointName is the registry key for one source's hook point, e.g.
// "caldav.calendar.beforeWrite". Namespacing by slug means two packages serving
// calendars cannot collide.
func pointName(slug, hook string) string {
	return "caldav." + slug + "." + hook
}

// RegisterTSHooks installs the `caldavHook` JS binding and returns the TSHooks
// wired to this source's points, ready to hand to Backend.SetTSHooks.
//
// The returned TSHooks reference the points whether or not any handler is ever
// registered; each point reports Enabled() == false until one is, which is what
// keeps the fast path free.
func RegisterTSHooks(host HostBindings, src Source) TSHooks {
	hooks := TSHooks{
		BeforeWrite:  host.Point(pointName(src.Slug, "beforeWrite")),
		BeforeDelete: host.Point(pointName(src.Slug, "beforeDelete")),
		CanRead:      host.Point(pointName(src.Slug, "canRead")),
		FilterList:   host.Point(pointName(src.Slug, "filterList")),
	}

	slug := src.Slug
	host.RegisterLoaderBinder(func(loader *sobek.Runtime, compile jsvm.Compiler, _ *pocketbase.PocketBase) error {
		// caldavHook({ beforeWrite(e) {...}, filterList(e) {...} })
		//
		// Handlers arrive as source strings: the fork's transpile stringifies
		// and recompiles each one standalone, so a handler closes over NOTHING
		// from its defining module. Everything it needs must be inline.
		register := func(handlers map[string]string) error {
			// Reject unknown keys BEFORE compiling anything, so a typo'd hook
			// name fails loudly rather than silently registering nothing.
			for name := range handlers {
				if !isKnownHookName(name) {
					return fmt.Errorf("caldavHook: unknown hook %q (known: %v)", name, hookPointNames)
				}
			}
			for _, name := range hookPointNames {
				source, ok := handlers[name]
				if !ok || source == "" {
					continue
				}
				fn, err := compile(normalizeHandlerSource(name, source))
				if err != nil {
					return fmt.Errorf("caldavHook: %s: %w", name, err)
				}
				host.AddHandler(pointName(slug, name), fn)
			}
			return nil
		}
		return loader.Set("caldavHook", register)
	})

	return hooks
}

// normalizeHandlerSource makes a handler's stringified source a valid
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
func normalizeHandlerSource(name, source string) string {
	if strings.HasPrefix(source, name+"(") {
		return "function " + source
	}
	// Generator/async shorthand ("*name(...)", "async name(...)") would need
	// the same treatment, but a hook handler must be a plain synchronous
	// function — the runtime does not await a Promise — so those are left to
	// fail loudly at compile rather than being quietly rewritten.
	return source
}

func isKnownHookName(name string) bool {
	for _, known := range hookPointNames {
		if known == name {
			return true
		}
	}
	return false
}
