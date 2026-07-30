package caldav

import (
	"errors"
	"net/http"
	"strings"

	"github.com/emersion/go-webdav/caldav"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/davauth"
)

const basicRealm = "TinyCld CalDAV"

// wellKnownPath is the RFC 5785 alias clients probe for service discovery.
//
// The name is the PROTOCOL, not the mount prefix: a client looking for a
// calendar service requests /.well-known/caldav regardless of where the tree is
// mounted. Deriving it from the prefix would leave the path every client
// actually asks for unserved.
const wellKnownPath = "/.well-known/caldav"

// Register mounts the CalDAV routes for each source on the single-app router,
// and — when host is non-nil — installs the `caldavHook` binding so package TS
// can register handlers against the four interception points. A no-op when no
// sources are given.
//
// Pass coreserver.CalDAVHostBindings() as host to enable TS hooks; pass the
// zero value to run pure Go.
//
// Returns the built Backends in source order.
func Register(app *pocketbase.PocketBase, sources []Source, host HostBindings) []*Backend {
	if len(sources) == 0 {
		return nil
	}

	backends := make([]*Backend, 0, len(sources))
	handlers := make([]*caldav.Handler, 0, len(sources))

	for _, src := range sources {
		b := NewBackend(app, src)
		if host.Point != nil {
			b.SetTSHooks(RegisterTSHooks(host, src))
		}
		backends = append(backends, b)
		handlers = append(handlers, &caldav.Handler{
			Backend: withErrorReporting(b, src.OnError),
			Prefix:  src.Prefix,
		})
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		for i, src := range sources {
			handler := handlers[i]

			serve := func(re *core.RequestEvent) error {
				// Refuse before spending bcrypt: the point of the limit is to
				// stop an attacker making the server do the expensive part.
				if davauth.TooManyFailures(app, re.Request) {
					http.Error(re.Response, "Too many failed attempts", http.StatusTooManyRequests)
					return nil
				}
				user, err := davauth.Authenticate(app, re.Request)
				if err != nil {
					davauth.NoteFailure(app, re.Request)
					davauth.Challenge(re.Response, basicRealm)
					return nil
				}
				davauth.NoteSuccess(app, re.Request)
				// Auth once per request; backend methods read the user off the
				// context rather than re-verifying per call.
				handler.ServeHTTP(re.Response, re.Request.WithContext(withUser(re.Request.Context(), user)))
				return nil
			}

			e.Router.Any(src.Prefix+"/{path...}", serve)
			e.Router.Any(src.Prefix, serve)
		}

		// One alias for the protocol, pointing at the first source. Registering
		// it per source would bind the same route repeatedly.
		wellKnownTarget := sources[0].Prefix + "/"
		e.Router.Any(wellKnownPath, func(re *core.RequestEvent) error {
			http.Redirect(re.Response, re.Request, wellKnownTarget, http.StatusMovedPermanently)
			return nil
		})

		return e.Next()
	})

	return backends
}

// HandlerFor builds a standalone CalDAV http.Handler for the given sources,
// backed by one app. The returned handler covers each source's prefix plus the
// .well-known alias, applying the Basic-Auth challenge itself. Returns nil when
// no sources are given.
//
// The app is taken as core.App — the minimal interface the backend actually
// needs — so any host can drive it without holding a concrete
// *pocketbase.PocketBase. Under per-process tenant isolation this runs INSIDE
// the org's own process, which mounts these routes on its own router from the
// source list the router materialized.
func HandlerFor(app core.App, sources []Source, host HostBindings) (http.Handler, []*Backend) {
	if len(sources) == 0 {
		return nil, nil
	}

	mux := http.NewServeMux()
	backends := make([]*Backend, 0, len(sources))

	for _, src := range sources {
		b := NewBackend(app, src)
		if host.Point != nil {
			b.SetTSHooks(RegisterTSHooks(host, src))
		}
		backends = append(backends, b)

		handler := &caldav.Handler{
			Backend: withErrorReporting(b, src.OnError),
			Prefix:  src.Prefix,
		}
		serve := func(w http.ResponseWriter, r *http.Request) {
			// See Register: refuse before spending bcrypt on an attacker.
			if davauth.TooManyFailures(app, r) {
				http.Error(w, "Too many failed attempts", http.StatusTooManyRequests)
				return
			}
			user, err := davauth.Authenticate(app, r)
			if err != nil {
				davauth.NoteFailure(app, r)
				davauth.Challenge(w, basicRealm)
				return
			}
			davauth.NoteSuccess(app, r)
			handler.ServeHTTP(w, r.WithContext(withUser(r.Context(), user)))
		}

		mux.HandleFunc(src.Prefix, serve)
		mux.HandleFunc(src.Prefix+"/", serve)
	}

	// Once, not per source: http.ServeMux panics on a duplicate pattern.
	wellKnownTarget := sources[0].Prefix + "/"
	mux.HandleFunc(wellKnownPath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, wellKnownTarget, http.StatusMovedPermanently)
	})

	return mux, backends
}

// Prefixes returns the URL path prefixes the sources serve, so a composing
// router can split them out from the stock mux.
func Prefixes(sources []Source) []string {
	out := make([]string, 0, len(sources)+1)
	if len(sources) > 0 {
		out = append(out, wellKnownPath)
	}
	for _, src := range sources {
		out = append(out, src.Prefix)
	}
	return out
}

// HasPrefix reports whether reqPath belongs to one of the sources' handlers.
func HasPrefix(sources []Source, reqPath string) bool {
	if len(sources) > 0 && reqPath == wellKnownPath {
		return true
	}
	for _, src := range sources {
		if reqPath == src.Prefix || strings.HasPrefix(reqPath, src.Prefix+"/") {
			return true
		}
	}
	return false
}

// IsNotFound reports whether err is this package's not-found sentinel. Both
// missing and unauthorized resources resolve to it, so callers cannot
// accidentally distinguish the two in a response.
func IsNotFound(err error) bool {
	return errors.Is(err, errNotFoundCause)
}
