// Package davprefix is the per-process registry of DAV mount prefixes.
//
// Each DAV server (caldav, carddav, webdav) claims its sources' prefixes here
// before mounting them. The registry is what keeps a malformed or duplicated
// prefix from reaching the router: an empty or "/" prefix puts a Basic-Auth
// handler in front of the whole SPA, a prefix under /api or /_ shadows
// PocketBase's REST API or dashboard, and two packages claiming one prefix is
// a ServeMux panic at boot. Claiming fails with an error naming the packages
// instead.
//
// It is per-process, not global state a test must reset: a Registry is created
// by whoever composes the servers, so the single-org app and a hosting tenant
// each validate exactly the package set they run.
package davprefix

import (
	"fmt"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// Registry records which slug claimed each prefix. The zero value is not
// usable; call New.
type Registry struct {
	claimed map[string]string
}

func New() *Registry {
	return &Registry{claimed: map[string]string{}}
}

// reserved first segments. A DAV mount under one of these shadows
// infrastructure the app needs: /api is PocketBase's REST API, /_ its
// dashboard, /.well-known the protocol discovery DAV clients themselves use.
var reserved = map[string]struct{}{
	"api":         {},
	"_":           {},
	".well-known": {},
}

// Claim validates prefix and records slug as its owner.
//
// Cross-protocol collisions are caught too, unlike the per-protocol check this
// replaces: a webdav source claiming /caldav is rejected, because every server
// claims from the same Registry.
func (r *Registry) Claim(slug, prefix string) error {
	if prefix == "" || prefix == "/" || !strings.HasPrefix(prefix, "/") || strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("package %s: invalid dav prefix %q (must start with '/', not end with one, and not be the bare root)", slug, prefix)
	}

	first, _, _ := strings.Cut(strings.TrimPrefix(prefix, "/"), "/")
	if _, bad := reserved[first]; bad {
		return fmt.Errorf("package %s: dav prefix %q shadows the reserved /%s namespace", slug, prefix, first)
	}

	if other, dup := r.claimed[prefix]; dup {
		return fmt.Errorf("packages %s and %s both mount dav prefix %q", other, slug, prefix)
	}

	r.claimed[prefix] = slug
	return nil
}

// ForApp returns the app's registry, creating it on first use.
//
// App-scoped rather than a package global so the registry covers exactly the
// package set this process composes: each DAV server calls it independently
// (they are registered from three separate feature repos, each holding only an
// app), and they must all claim from the same one for a cross-protocol
// collision to be visible. A test app gets its own.
func ForApp(app core.App) *Registry {
	if r, ok := app.Store().Get(storeKey).(*Registry); ok {
		return r
	}
	r := New()
	app.Store().Set(storeKey, r)
	return r
}

const storeKey = "tinycldDAVPrefixes"
