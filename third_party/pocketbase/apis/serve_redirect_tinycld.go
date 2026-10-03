package apis

// Fork-only: an injected listener for Serve's HTTP->HTTPS redirect server.
// See third_party/pocketbase/FORK.md.

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

// SetRedirectListener makes Serve's HTTP->HTTPS redirect server use l
// instead of binding config.HttpAddr itself. Call it before Serve so the
// redirect server picks it up.
func SetRedirectListener(app core.App, l net.Listener) {
	app.Store().Set(redirectListenerStoreKey, l)
}

// serveHTTPRedirect serves h on the listener SetRedirectListener stored for
// app, or on addr via http.ListenAndServe when none was set. It registers an
// OnTerminate handler so the redirect server shuts down with the app, and
// logs a serve error through app.Logger() instead of dropping it.
func serveHTTPRedirect(app core.App, addr string, h http.Handler) {
	l, _ := app.Store().Get(redirectListenerStoreKey).(net.Listener)

	server := &http.Server{Addr: addr, Handler: h}

	app.OnTerminate().Bind(&hook.Handler[*core.TerminateEvent]{
		Id: "pbRedirectShutdown",
		Func: func(te *core.TerminateEvent) error {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			defer cancel()
			_ = server.Shutdown(ctx)
			return te.Next()
		},
	})

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
