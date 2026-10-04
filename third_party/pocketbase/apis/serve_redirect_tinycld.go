package apis

// Fork-only: an injected listener and a listener hook for Serve's
// HTTP->HTTPS redirect server. See third_party/pocketbase/FORK.md.

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// redirectListenerStoreKey is the app.Store() key for the listener SetRedirectListener
// injects. It is unexported so only this file can read or write it.
const redirectListenerStoreKey = "@tinycldRedirectListener"

// redirectServerStoreKey is the app.Store() key for the *http.Server
// serveHTTPRedirect last started, so a later call can shut the previous one
// down instead of leaking it (see serveHTTPRedirect).
const redirectServerStoreKey = "@tinycldRedirectServer"

// redirectServerHookStoreKey is the app.Store() key for the hook
// SetRedirectServerHook installs.
const redirectServerHookStoreKey = "@tinycldRedirectServerHook"

// redirectListenerReadStoreKey marks that serveHTTPRedirect has already read
// redirectListenerStoreKey, so SetRedirectListener can warn on a call that
// arrives too late to take effect (see SetRedirectListener).
const redirectListenerReadStoreKey = "@tinycldRedirectListenerRead"

// SetRedirectListener makes Serve's HTTP->HTTPS redirect server use l
// instead of binding config.HttpAddr itself. Call it before Serve — once
// serveHTTPRedirect has read the store (Serve is already running), a call
// here is too late to take effect and only warns.
func SetRedirectListener(app core.App, l net.Listener) {
	if app.Store().Has(redirectListenerReadStoreKey) {
		app.Logger().Warn("SetRedirectListener called after the redirect server already started; it has no effect")
	}
	app.Store().Set(redirectListenerStoreKey, l)
}

// RedirectServerHook gets the redirect server before it serves and the
// listener it would serve on, and returns the listener to serve on instead.
type RedirectServerHook func(srv *http.Server, l net.Listener) net.Listener

// SetRedirectServerHook makes Serve's HTTP->HTTPS redirect server serve on
// the listener hook returns. The hook runs once per redirect server, before
// it serves, so a caller can follow every connection from the first one
// (for example, to let requests in flight finish before a shutdown). When
// no listener was injected, the redirect server binds config.HttpAddr
// itself and passes that listener to the hook. Like SetRedirectListener, it
// must be called before Serve.
func SetRedirectServerHook(app core.App, hook RedirectServerHook) {
	if app.Store().Has(redirectListenerReadStoreKey) {
		app.Logger().Warn("SetRedirectServerHook called after the redirect server already started; it has no effect")
	}
	app.Store().Set(redirectServerHookStoreKey, hook)
}

// serveHTTPRedirect serves h on the listener SetRedirectListener stored for
// app, or on addr via http.ListenAndServe when none was set. A hook set with
// SetRedirectServerHook replaces that listener before the server serves. It
// registers an OnTerminate handler so the redirect server shuts down with
// the app, and logs a serve error through app.Logger() instead of dropping
// it.
//
// A previous server started for app (from an earlier call on the same app)
// is shut down first, so repeated calls — e.g. across a restart that calls
// Serve again — don't leak the old server's goroutine.
func serveHTTPRedirect(app core.App, addr string, h http.Handler) {
	app.Store().Set(redirectListenerReadStoreKey, true)
	l, _ := app.Store().Get(redirectListenerStoreKey).(net.Listener)
	hookFn, _ := app.Store().Get(redirectServerHookStoreKey).(RedirectServerHook)

	if prev, ok := app.Store().Get(redirectServerStoreKey).(*http.Server); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		_ = prev.Shutdown(ctx)
		cancel()
	}

	server := &http.Server{Addr: addr, Handler: h}
	app.Store().Set(redirectServerStoreKey, server)

	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id: "pbRedirectShutdown",
		Func: func(te *core.TerminateEvent) error {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
			return te.Next()
		},
	})

	if hookFn != nil {
		if l == nil {
			bound, err := net.Listen("tcp", addr)
			if err != nil {
				app.Logger().Error("redirect server error", "error", err)
				return
			}
			l = bound
		}
		l = hookFn(server, l)
	}

	var err error
	if l != nil {
		err = server.Serve(l)
	} else {
		err = server.ListenAndServe()
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		app.Logger().Error("redirect server error", "error", err)
	}
}
