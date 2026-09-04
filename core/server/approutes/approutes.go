// Package approutes holds the URL shapes the server generates for the SPA.
//
// A leaf package with no dependencies, so both coreserver and notify can use it
// without an import cycle (coreserver already imports notify).
package approutes

// Prefix is the URL segment every app route lives under. It must match
// APP_PREFIX in core/lib/org-routes.ts.
//
// NOT an org slug: single-org deployments give each org its own host, so this
// is one fixed segment and nothing interpolates into it. Public share routes
// (/p/...), protocol mounts (/dav, /caldav, /carddav) and the API (/api) sit
// OUTSIDE this prefix and must never be rewritten with it.
const Prefix = "/a"

// Href returns an app path for a root-relative route, e.g. Href("boards") is
// "/a/boards". The path should not start with a slash.
func Href(path string) string {
	if path == "" {
		return Prefix
	}
	return Prefix + "/" + path
}
