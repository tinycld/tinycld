package webdav

import (
	"context"
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"golang.org/x/net/webdav"

	"tinycld.org/core/davauth"
)

const basicRealm = "TinyCld WebDAV"

// Register mounts the WebDAV routes for each source on the single-app router,
// and — when host is non-nil — installs the `webdavHook` binding so package TS
// can register handlers against the five interception points. A no-op when no
// sources are given. Core already installs the /drive CORS bypass, so this adds
// only the protocol handler, the Basic-Auth challenge, and the .well-known
// redirect.
//
// Pass coreserver.WebDAVHostBindings() as host to enable TS hooks; pass the
// zero value to run pure Go.
//
// Returns the built FileSystems in source order.
func Register(app *pocketbase.PocketBase, sources []Source, host HostBindings) ([]*FileSystem, error) {
	if len(sources) == 0 {
		return nil, nil
	}

	filesystems := make([]*FileSystem, 0, len(sources))
	handlers := make([]*webdav.Handler, 0, len(sources))

	for _, src := range sources {
		fs, err := NewFileSystem(app, src)
		if err != nil {
			return nil, err
		}
		if host.Point != nil {
			fs.SetTSHooks(RegisterTSHooks(host, src))
		}
		filesystems = append(filesystems, fs)
		handlers = append(handlers, newDAVHandler(app, fs))
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
				// Auth once per request; FileSystem methods read the user off
				// the context rather than re-verifying per call.
				ctx := context.WithValue(re.Request.Context(), userKey, user)
				handler.ServeHTTP(re.Response, re.Request.WithContext(ctx))
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

	return filesystems, nil
}

// HandlerFor builds a standalone WebDAV http.Handler for the given sources,
// backed by one app. The returned handler covers each source's prefix plus its
// .well-known alias, applying the Basic-Auth challenge itself. Returns nil when
// no sources are given.
//
// The app is taken as core.App — the minimal interface the filesystem actually
// needs — so any host can drive it without holding a concrete
// *pocketbase.PocketBase. Under per-process tenant isolation this runs INSIDE
// the org's own process, which mounts these routes on its own router from the
// source list the router materialized.
func HandlerFor(app core.App, sources []Source, host HostBindings) (http.Handler, []*FileSystem, error) {
	if len(sources) == 0 {
		return nil, nil, nil
	}

	mux := http.NewServeMux()
	filesystems := make([]*FileSystem, 0, len(sources))

	for _, src := range sources {
		fs, err := NewFileSystem(app, src)
		if err != nil {
			return nil, nil, err
		}
		if host.Point != nil {
			fs.SetTSHooks(RegisterTSHooks(host, src))
		}
		filesystems = append(filesystems, fs)

		handler := newDAVHandler(app, fs)
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
			ctx := context.WithValue(r.Context(), userKey, user)
			handler.ServeHTTP(w, r.WithContext(ctx))
		}

		mux.HandleFunc(src.Prefix, serve)
		mux.HandleFunc(src.Prefix+"/", serve)
	}

	// Once, not per source: http.ServeMux panics on a duplicate pattern.
	wellKnownTarget := sources[0].Prefix + "/"
	mux.HandleFunc(wellKnownPath, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, wellKnownTarget, http.StatusMovedPermanently)
	})

	return mux, filesystems, nil
}

// newDAVHandler wraps a FileSystem in the x/net/webdav handler.
//
// NewMemLS is load-bearing: an in-memory lock system is what advertises DAV
// class 2, and macOS Finder mounts a class-1-only share read-only.
func newDAVHandler(app core.App, fs *FileSystem) *webdav.Handler {
	return &webdav.Handler{
		FileSystem: fs,
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				app.Logger().Debug("WebDAV",
					"slug", fs.src.Slug,
					"method", r.Method,
					"path", r.URL.Path,
					"error", err)
			}
		},
	}
}

// wellKnownPath is the RFC 5785 alias clients probe for service discovery.
//
// The name is the PROTOCOL, not the mount prefix: a client looking for a WebDAV
// share requests /.well-known/webdav regardless of where the tree is mounted.
// Deriving it from the prefix (yielding /.well-known/drive) would leave the path
// every client actually asks for unserved.
//
// It follows that with multiple sources the alias can only point at one of them;
// the first registered wins, and the rest are reachable at their own prefixes.
const wellKnownPath = "/.well-known/webdav"

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
	for _, src := range sources {
		if reqPath == src.Prefix ||
			strings.HasPrefix(reqPath, src.Prefix+"/") ||
			reqPath == wellKnownPath {
			return true
		}
	}
	return false
}
