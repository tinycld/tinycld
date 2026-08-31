package carddav

import (
	"context"
	"net/http"
	"strings"

	"github.com/emersion/go-webdav/carddav"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/davauth"
)

const basicRealm = "TinyCld CardDAV"

// Register mounts the CardDAV routes on serve for the single-org app, backed by
// the given sources. Uses singleOrgScope: the process is one org, each user sees
// one book of the contacts they own. A no-op when no sources are registered. Core
// already installs the /carddav CORS bypass, so this only adds the protocol
// handler + Basic-Auth challenge + .well-known redirect.
func Register(app *pocketbase.PocketBase, sources []Source) {
	if len(sources) == 0 {
		return
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		backend := &Backend{app: app, sources: sources, scope: singleOrgScope{bookSegment: "default"}}
		handler := carddav.Handler{Backend: backend, Prefix: "/carddav"}

		serve := func(re *core.RequestEvent) error {
			if _, _, ok := re.Request.BasicAuth(); !ok {
				davauth.Challenge(re.Response, basicRealm)
				return nil
			}
			if davauth.TooManyFailures(app, re.Request) {
				http.Error(re.Response, "Too many failed attempts", http.StatusTooManyRequests)
				return nil
			}
			// Authenticate at the route, like CalDAV and WebDAV. Left to the
			// backend, a bad credential surfaces as a bare error that go-webdav
			// turns into a 500 with no WWW-Authenticate — clients read that as
			// a server fault and never re-prompt. The request cache also means
			// one bcrypt verification per request, not per backend call (a
			// single PROPFIND drives several).
			r := davauth.WithRequestCache(re.Request)
			if _, err := davauth.Authenticate(app, r); err != nil {
				davauth.NoteFailure(app, r)
				davauth.Challenge(re.Response, basicRealm)
				return nil
			}
			davauth.NoteSuccess(app, r)
			ctx := context.WithValue(r.Context(), httpRequestKey, r)
			handler.ServeHTTP(re.Response, r.WithContext(ctx))
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
// org's app, using singleOrgScope (the whole DB is the org — the hosting tenant
// model). The returned handler covers /carddav, /carddav/*, and
// /.well-known/carddav, applying the Basic-Auth challenge itself. Returns nil
// when no sources are given (nothing to serve).
//
// The app is taken as core.App — the minimal interface the backend actually
// needs (record Save/Delete/find are all core.App methods). That lets any host
// drive it without holding a concrete *pocketbase.PocketBase.
//
// Under per-process tenant isolation this runs INSIDE the org's own process
// (hosting's cmd/serve-org), which mounts these routes on its own router from
// the source list the router materialized. The hosting host has no tenant app
// object to compose against — it only reverse-proxies to the tenant socket.
func HandlerFor(app core.App, sources []Source) http.Handler {
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
			davauth.Challenge(w, basicRealm)
			return
		}
		if davauth.TooManyFailures(app, r) {
			http.Error(w, "Too many failed attempts", http.StatusTooManyRequests)
			return
		}
		// See Register: authenticate at the route so a bad credential is a 401
		// with a challenge, not a backend error go-webdav reports as 500; the
		// cache keeps it to one verification per request.
		r = davauth.WithRequestCache(r)
		if _, err := davauth.Authenticate(app, r); err != nil {
			davauth.NoteFailure(app, r)
			davauth.Challenge(w, basicRealm)
			return
		}
		davauth.NoteSuccess(app, r)
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
