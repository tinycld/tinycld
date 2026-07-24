package carddav

import (
	"context"
	"net/http"
	"strings"

	"github.com/emersion/go-webdav/carddav"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

const basicRealm = `Basic realm="TinyCld CardDAV"`

// Register mounts the CardDAV routes on serve for the SINGLE-TENANT app, backed
// by the given sources. Uses sharedDBScope: one process holds every org's data,
// books are per the user's org membership, paths carry the org slug. A no-op when
// no sources are registered. Core already installs the /carddav CORS bypass, so
// this only adds the protocol handler + Basic-Auth challenge + .well-known redirect.
func Register(app *pocketbase.PocketBase, sources []Source) {
	if len(sources) == 0 {
		return
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		backend := &Backend{app: app, sources: sources, scope: sharedDBScope{}}
		handler := carddav.Handler{Backend: backend, Prefix: "/carddav"}

		serve := func(re *core.RequestEvent) error {
			if _, _, ok := re.Request.BasicAuth(); !ok {
				re.Response.Header().Set("WWW-Authenticate", basicRealm)
				http.Error(re.Response, "Authentication required", http.StatusUnauthorized)
				return nil
			}
			ctx := context.WithValue(re.Request.Context(), httpRequestKey, re.Request)
			handler.ServeHTTP(re.Response, re.Request.WithContext(ctx))
			return nil
		}

		e.Router.Any("/carddav/{path...}", serve)
		e.Router.Any("/carddav", serve)
		e.Router.Any("/.well-known/carddav", func(re *core.RequestEvent) error {
			http.Redirect(re.Response, re.Request, "/carddav/", http.StatusMovedPermanently)
			return nil
		})

		return e.Next()
	})
}

// HandlerFor builds a standalone CardDAV http.Handler for ONE org, backed by that
// org's app, using singleOrgScope (the whole DB is the org — the multi-org tenant
// model). The returned handler covers /carddav, /carddav/*, and
// /.well-known/carddav, applying the Basic-Auth challenge itself. Intended for the
// multi-org host to prefix-compose in front of a tenant's stock mux. Returns nil
// when no sources are given (nothing to serve).
//
// The app is passed as *pocketbase.PocketBase because the backend needs the
// concrete app for record Save/Delete; the router holds exactly this (inst.app).
func HandlerFor(app *pocketbase.PocketBase, sources []Source) http.Handler {
	if len(sources) == 0 {
		return nil
	}
	backend := &Backend{app: app, sources: sources, scope: singleOrgScope{bookSegment: "default"}}
	dav := carddav.Handler{Backend: backend, Prefix: "/carddav"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/carddav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/carddav/", http.StatusMovedPermanently)
	})
	serve := func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := r.BasicAuth(); !ok {
			w.Header().Set("WWW-Authenticate", basicRealm)
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), httpRequestKey, r)
		dav.ServeHTTP(w, r.WithContext(ctx))
	}
	mux.HandleFunc("/carddav", serve)
	mux.HandleFunc("/carddav/", serve)
	return mux
}

// Prefixes returns the URL path prefixes HandlerFor serves, so a composing router
// can route them to the CardDAV handler and everything else to the stock mux.
func Prefixes() []string {
	return []string{"/carddav", "/.well-known/carddav"}
}

// HasPrefix reports whether reqPath belongs to the CardDAV handler.
func HasPrefix(reqPath string) bool {
	return reqPath == "/carddav" ||
		strings.HasPrefix(reqPath, "/carddav/") ||
		reqPath == "/.well-known/carddav"
}
