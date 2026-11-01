package logging

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/appstate"
)

// appContextKey carries the app a record belongs to.
type appContextKey struct{}

// WithApp returns a copy of ctx that names app as the owner of every record
// logged with it. The installed handler writes such a record to app's _logs
// table; a record whose ctx names no app reaches stderr and Sentry only.
//
// Request middleware and each per-app background job put the app here, so a
// *Context log call made under them needs nothing more. This is the one place
// an app travels in a context: a log call site has no other way to name it.
func WithApp(ctx context.Context, app core.App) context.Context {
	if app == nil {
		return ctx
	}
	return context.WithValue(ctx, appContextKey{}, app)
}

// AppFrom reports the app WithApp put on ctx.
func AppFrom(ctx context.Context) (core.App, bool) {
	if ctx == nil {
		return nil, false
	}
	app, ok := ctx.Value(appContextKey{}).(core.App)
	return app, ok && app != nil
}

// defaultAppHolder wraps the app so an atomic.Pointer can hold an interface.
// id is the app's appstate.ID, which is the same for an app reopened on the
// same data dir.
type defaultAppHolder struct {
	app core.App
	id  string
}

var (
	defaultApp atomic.Pointer[defaultAppHolder]
	// defaultAppMu orders SetDefaultApp calls, so the check for a different
	// app and the store that follows it are one step. DefaultApp does not
	// take it.
	defaultAppMu sync.Mutex
)

var defaultAppLog = ForPackage("logging")

// SetDefaultApp names the app that owns records with no app in their context,
// and makes the process Sentry hub that app's hub. Only a process that runs
// exactly one app calls it: there, every record belongs to that app, which is
// how a standalone server kept writing startup and ticker records to its
// _logs table before apps could share a process. Call it before anything
// asks for the app's hub. Nil clears it.
//
// While one app is the default, a call with a different app (another data
// dir) logs an error and changes nothing: records with no app must not move
// to whichever app was set last, and a second app must not take the process
// hub. A call with the same app, or with an app reopened on the same data
// dir, replaces it.
func SetDefaultApp(app core.App) {
	if app == nil {
		defaultAppMu.Lock()
		defaultApp.Store(nil)
		defaultAppMu.Unlock()
		return
	}
	next := &defaultAppHolder{app: app, id: appstate.ID(app)}

	defaultAppMu.Lock()
	cur := defaultApp.Load()
	if cur != nil && cur.id != next.id {
		defaultAppMu.Unlock()
		defaultAppLog.Error("a second app cannot be the default log app; keeping the first",
			"defaultDataDir", cur.app.DataDir(), "refusedDataDir", app.DataDir())
		return
	}
	hubKey.Set(app, processHub())
	defaultApp.Store(next)
	defaultAppMu.Unlock()
}

// DefaultApp returns the app SetDefaultApp named, or nil.
func DefaultApp() core.App {
	if h := defaultApp.Load(); h != nil {
		return h.app
	}
	return nil
}

// appFor is the app a record logged with ctx belongs to, or nil.
func appFor(ctx context.Context) core.App {
	if app, ok := AppFrom(ctx); ok {
		return app
	}
	return DefaultApp()
}

// router is the process-wide handler: every record goes to stderr and Sentry,
// and to the _logs table of the app it belongs to. One router serves every
// app, so installing it once never ties the process's logs to one app.
type router struct {
	// base fans out to stderr and Sentry, with this logger's attrs and groups.
	base slog.Handler
	// ops are this logger's WithAttrs/WithGroup calls, replayed on the app's
	// _logs handler, which is only known when a record arrives.
	ops []handlerOp
}

func newRouter(stderr io.Writer) *router {
	return &router{base: NewFanout(
		slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: StderrLevel}),
		NewSentryHandler(SentryLevel),
	)}
}

func (r *router) Enabled(ctx context.Context, level slog.Level) bool {
	if r.base.Enabled(ctx, level) {
		return true
	}
	h := r.logsHandler(ctx)
	return h != nil && h.Enabled(ctx, level)
}

// Handle writes to every destination even when one fails: losing a _logs row
// must never also lose the stderr line or the Sentry event.
func (r *router) Handle(ctx context.Context, rec slog.Record) error {
	err := r.base.Handle(ctx, rec.Clone())
	if h := r.logsHandler(ctx); h != nil && h.Enabled(ctx, rec.Level) {
		if logsErr := h.Handle(ctx, rec.Clone()); logsErr != nil {
			err = logsErr
		}
	}
	return err
}

// logsHandler is the _logs handler of the app the record belongs to, or nil
// when it belongs to none.
func (r *router) logsHandler(ctx context.Context) slog.Handler {
	app := appFor(ctx)
	if app == nil {
		return nil
	}
	appLogger := app.Logger()
	// Until PocketBase opens the app's own logger during bootstrap,
	// app.Logger() is slog.Default(): this router. Writing there would
	// recurse forever, and there is no _logs table to write to yet anyway.
	if appLogger == slog.Default() {
		return nil
	}
	var h slog.Handler = newLogsTableHandler(appLogger.Handler())
	for _, op := range r.ops {
		if op.group != "" {
			h = h.WithGroup(op.group)
		} else {
			h = h.WithAttrs(op.attrs)
		}
	}
	return h
}

func (r *router) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return r
	}
	return r.with(r.base.WithAttrs(attrs), handlerOp{attrs: attrs})
}

func (r *router) WithGroup(name string) slog.Handler {
	if name == "" {
		return r
	}
	return r.with(r.base.WithGroup(name), handlerOp{group: name})
}

// with copies the op list so sibling loggers never share a backing array.
func (r *router) with(base slog.Handler, op handlerOp) *router {
	ops := make([]handlerOp, len(r.ops), len(r.ops)+1)
	copy(ops, r.ops)
	return &router{base: base, ops: append(ops, op)}
}
