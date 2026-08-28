package coreserver

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"tinycld.org/core/approutes"

	"github.com/pocketbase/pocketbase/core"
)

// publicConfigScript builds the inline <script> that publishes NON-secret system
// config to the web client on window.__TINYCLD_PUBLIC_CONFIG__, read at module
// init by lib/app-config.ts (e.g. the Sentry DSN). Secret values never appear
// here (see the two gates below). Storage keys are mapped to client-facing field
// names so the browser contract is decoupled from the collection's key namespace.
// Returns "" when there's nothing to publish.
//
// The JSON is marshaled (not string-built) so values are safely escaped; we then
// guard against "</script>" appearing in a value, which would otherwise close the
// tag early — JSON-encodes "<" as-is, so a malicious DSN could break out.
func publicConfigScript() string {
	// Two independent gates keep secrets out of the HTML: (1) PublicValues()
	// returns only non-secret rows, and (2) the explicit whitelist below names the
	// exact keys the client may consume, under their client-facing names. Adding a
	// public client value is a deliberate edit here, not an automatic passthrough.
	// The whitelist is also asserted non-secret at injection time (the one place a
	// leak is unrecoverable) so a row mis-flagged is_secret=false elsewhere still
	// can't slip a credential into the page.
	const sentryDSNKey = "sentry.dsn"
	out := map[string]string{}
	if v := systemConfig.publicValue(sentryDSNKey); v != "" {
		out["sentryDsn"] = v
	}
	if len(out) == 0 {
		return ""
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	// Defense-in-depth against early tag termination from a "</script>" substring.
	safe := bytes.ReplaceAll(payload, []byte("</"), []byte("<\\/"))
	return "<script>window.__TINYCLD_PUBLIC_CONFIG__=" + string(safe) + "</script>"
}

// injectPublicConfig inserts the public-config <script> just before </head> in
// the SPA shell HTML. The script runs before the deferred bundle JS, so the
// window global is set by the time lib/configure-core reads it at module init.
// Falls back to returning the HTML unchanged when there's nothing to inject or
// no </head> is present.
func injectPublicConfig(html []byte) []byte {
	script := publicConfigScript()
	if script == "" {
		return html
	}
	idx := bytes.Index(bytes.ToLower(html), []byte("</head>"))
	if idx < 0 {
		return html
	}
	out := make([]byte, 0, len(html)+len(script))
	out = append(out, html[:idx]...)
	out = append(out, script...)
	out = append(out, html[idx:]...)
	return out
}

// binaryDir returns the directory containing the running executable, or
// the empty string for `go run` invocations (where os.Args[0] is a temp
// binary and the caller's cwd is the source tree). Used by the Default*
// helpers below to anchor relative paths next to the installed binary
// regardless of the cwd it was launched from.
func binaryDir() string {
	if strings.HasPrefix(os.Args[0], os.TempDir()) {
		return ""
	}
	return filepath.Dir(os.Args[0])
}

// DefaultPublicDir returns the default public dir relative to the running
// executable — "./public" when running via `go run` (tempdir binary) or
// next to the installed binary otherwise.
func DefaultPublicDir() string {
	if dir := binaryDir(); dir != "" {
		return filepath.Join(dir, "public")
	}
	return "./public"
}

// DefaultWebsiteDir returns the marketing-website root on the persistent state
// volume: <TINYCLD_STATE_DIR>/website (e.g. /workspace/website in production),
// or "./website" when TINYCLD_STATE_DIR is unset (`go run` / dev). The site lives
// on the state volume, NOT baked into the image, so a website-only deploy
// (utils/deploy.sh web) can rsync a new build onto the volume without an image
// rebuild. It stays SEPARATE from DefaultPublicDir() so the app's `expo export`
// (which sweeps public/ into its web bundle) never absorbs the site.
// StaticWithDynamicFallback treats a missing/empty websiteDir as "no website".
func DefaultWebsiteDir() string {
	if d := os.Getenv("TINYCLD_STATE_DIR"); d != "" {
		return filepath.Join(d, "website")
	}
	return "./website"
}

// DefaultReleasesDir returns the releases dir under the STATE root
// (resolveStateDir()), which persists across the per-build symlink swap. The
// runtime entrypoint promotes per-deploy bundles into this directory; a
// `current` symlink there points at the active release. When TINYCLD_STATE_DIR
// is unset, stateReleasesDir() falls back to the binary dir (the `go run` /
// pre-relocation case), preserving the previous "./releases" behavior.
func DefaultReleasesDir() string {
	return stateReleasesDir()
}

// DefaultTypesDir returns the default location the server writes generated
// pbSchema.ts / pbZodSchema.ts to. Post-merge, core is nested INSIDE the app
// member ( <appDir>/core ) rather than a top-level sibling, and the installed
// binary lives at <appDir>/tinycld. So core's types/ sits directly under the
// binary's dir: <appDir>/core/types.
//
// For `go run` (tempdir binary) the cwd is the Go server dir (<appDir>/server),
// whose parent is <appDir>; core/types is then ../core/types.
//
// TINYCLD_TYPES_DIR overrides this (CI/tests scanning a non-standard tree).
func DefaultTypesDir() string {
	if env := os.Getenv("TINYCLD_TYPES_DIR"); env != "" {
		return env
	}
	dir := binaryDir()
	if dir == "" {
		// `go run` / temp-built binary: cwd is the Go server dir (<appDir>/server),
		// and core is nested at <appDir>/core. Resolve relative to cwd's parent.
		return filepath.Join("..", "core", "types")
	}
	// Installed binary at <appDir>/tinycld; core nested at <appDir>/core.
	return filepath.Join(dir, "core", "types")
}

// legacyAppSegments are the first path segments that used to be app routes
// before everything moved under approutes.Prefix.
//
// An explicit allowlist, not a catch-all rewrite: a blanket "prepend /a to any
// miss" would also rewrite asset 404s, /.well-known probes and future marketing
// paths, and would fight the website lookup in StaticWithDynamicFallback.
var legacyAppSegments = map[string]bool{
	// Pre-auth. These are the ones that actually matter: invite and reset links
	// are EMAILED, so URLs minted before the move keep arriving from inboxes
	// for days. A 404 there is a dead invite.
	"accept-invite":  true,
	"reset-password": true,
	"connect":        true,
	"pick-org":       true,
	"setup":          true,
	// Workspace areas a user may have bookmarked.
	"settings": true,
	"help":     true,
}

// legacyAppRedirect maps a pre-prefix app URL to its current location, or ""
// when the path is not a legacy app route.
//
// Public (/p/...), protocol (/dav, /caldav, /carddav) and API paths are NOT app
// routes and must never be rewritten.
func legacyAppRedirect(path string) string {
	if path == "" || strings.HasPrefix(path, approutes.Prefix+"/") || path == approutes.Prefix {
		return ""
	}
	seg, _, _ := strings.Cut(path, "/")
	if !legacyAppSegments[seg] {
		return ""
	}
	return approutes.Prefix + "/" + path
}

// redirectLegacyAppPath issues a 302 to the prefixed location when path is a
// legacy app route. Reports whether it handled the request.
//
// 302, not 301: a permanent redirect is cached indefinitely by browsers and
// cannot be walked back if an entry here turns out to shadow a real path.
func redirectLegacyAppPath(e *core.RequestEvent, path string) bool {
	target := legacyAppRedirect(path)
	if target == "" {
		return false
	}
	if q := e.Request.URL.RawQuery; q != "" {
		target += "?" + q
	}
	e.Redirect(http.StatusFound, target)
	return true
}

// StaticWithFallback serves static files from dir, falling back to
// fallbackFile for missing paths (so SPA routing works).
func StaticWithFallback(dir string, fallbackFile string) func(*core.RequestEvent) error {
	fs := os.DirFS(dir)

	return func(e *core.RequestEvent) error {
		// Only serve static files for GET/HEAD — let WebDAV methods pass through
		if e.Request.Method != http.MethodGet && e.Request.Method != http.MethodHead {
			return e.Next()
		}

		path := e.Request.PathValue("path")
		if path == "" {
			path = "index.html"
		}

		f, err := fs.Open(path)
		if err == nil {
			f.Close()
			return e.FileFS(fs, path)
		}

		indexPath := path + "/index.html"
		f, err = fs.Open(indexPath)
		if err == nil {
			f.Close()
			return e.FileFS(fs, indexPath)
		}

		// Never serve the SPA HTML fallback for API paths. This catch-all is
		// registered after PocketBase's own /api/* routes, so it only sees an
		// /api/ request when that route isn't (yet) registered — e.g. during the
		// post-restart boot window after a package swap, before the collection
		// routes are wired. Returning app.html there hands API clients
		// "<!DOCTYPE …" with a 200, which a JSON parse then chokes on
		// ("Unexpected token '<'"). A JSON 404 lets clients see "not ready" and
		// retry instead of mis-parsing HTML as JSON.
		if redirectLegacyAppPath(e, path) {
			return nil
		}

		if fallbackFile != "" && !strings.HasPrefix("/"+path, "/api/") {
			return e.FileFS(fs, fallbackFile)
		}

		return e.NotFoundError("", nil)
	}
}

// serveStaticFrom tries to serve `path` (then `path/index.html`) out of fs.
// Returns true (and writes the response) on a hit, false on a miss. Shared by
// the website + public lookups so both apply the same path → path/index.html
// resolution. A nil fs (dir not configured) is always a miss.
func serveStaticFrom(e *core.RequestEvent, fs fs.FS, path string) (bool, error) {
	if fs == nil {
		return false, nil
	}
	if f, err := fs.Open(path); err == nil {
		f.Close()
		// apple-app-site-association has no extension, so Go's mime sniffing
		// serves it as text/plain — but iOS Universal Links require
		// application/json or it silently fails domain verification.
		if path == ".well-known/apple-app-site-association" {
			e.Response.Header().Set("Content-Type", "application/json")
		}
		setHTMLRevalidate(e, path)
		return true, e.FileFS(fs, path)
	}
	indexPath := path + "/index.html"
	if f, err := fs.Open(indexPath); err == nil {
		f.Close()
		setHTMLRevalidate(e, indexPath)
		return true, e.FileFS(fs, indexPath)
	}
	return false, nil
}

// setHTMLRevalidate marks an HTML document as always-revalidate. FileFS serves
// via http.ServeContent, which emits Last-Modified and answers a conditional
// GET with 304, so `no-cache` keeps the entry document current without the
// full-refetch cost of `no-store` — an unchanged page costs one header
// round-trip. Non-HTML static assets (favicon, images, workers) keep FileFS's
// default (no directive) and are unaffected. This closes the stale-shell bug:
// an HTML entry that pins content-hashed asset URLs must never be served from
// cache without revalidation, or it references asset hashes the client dropped.
func setHTMLRevalidate(e *core.RequestEvent, path string) {
	if strings.HasSuffix(path, ".html") {
		e.Response.Header().Set("Cache-Control", "no-cache")
	}
}

// StaticWithDynamicFallback serves static files, then falls back to the active
// release's SPA shell (<releasesDir>/current/app.html). Lookups are tried in
// order:
//
//  1. websiteDir — the marketing website (org-mode deploys only; "" otherwise).
//     It lives in its OWN directory, NOT in publicDir, so the app's `expo
//     export` (which sweeps publicDir into its web bundle) can never pull the
//     website into the SPA bundle — the bug where every app route rendered the
//     marketing homepage after an in-app package install.
//  2. publicDir — the app's own web static files (favicon, sw.js, workers/),
//     i.e. exactly what Expo emits from the app member's public/ dir.
//  3. SPA fallback — <releasesDir>/current/app.html for any non-/api/ miss.
//
// When releasesDir is empty or its `current` symlink doesn't resolve to a
// readable app.html, the handler falls back to publicDir/app.html — the
// legacy behavior used in dev where the volume isn't mounted. Any path
// not present in any location returns 404.
func StaticWithDynamicFallback(publicDir, websiteDir, releasesDir string) func(*core.RequestEvent) error {
	publicFs := os.DirFS(publicDir)
	var websiteFs fs.FS
	if websiteDir != "" {
		websiteFs = os.DirFS(websiteDir)
	}

	return func(e *core.RequestEvent) error {
		if e.Request.Method != http.MethodGet && e.Request.Method != http.MethodHead {
			return e.Next()
		}

		path := e.Request.PathValue("path")
		if path == "" {
			path = "index.html"
		}

		// Website first (org mode), then the app's own public/ files.
		if hit, err := serveStaticFrom(e, websiteFs, path); hit {
			return err
		}
		if hit, err := serveStaticFrom(e, publicFs, path); hit {
			return err
		}

		// Never serve the SPA HTML fallback for API paths. This catch-all is
		// registered after PocketBase's own /api/* routes, so it only sees an
		// /api/ request when that route isn't (yet) registered — e.g. during the
		// post-restart boot window after a package swap, before the collection
		// routes are wired. Returning app.html there hands API clients
		// "<!DOCTYPE …" with a 200, which a JSON parse then chokes on
		// ("Unexpected token '<'"). A JSON 404 lets clients see "not ready" and
		// retry instead of mis-parsing HTML as JSON.
		if strings.HasPrefix("/"+path, "/api/") {
			return e.NotFoundError("", nil)
		}

		if redirectLegacyAppPath(e, path) {
			return nil
		}

		// SPA fallback: the active release's shell. writeAppShell revalidates
		// via ETag (see its doc) so a tab reload always confirms the shell is
		// current — a stale shell would reference asset hashes the client has
		// since dropped — but an unchanged shell 304s instead of re-sending.
		if releasesDir != "" {
			currentApp := filepath.Join(releasesDir, "current", "app.html")
			if data, err := os.ReadFile(currentApp); err == nil {
				return writeAppShell(e, injectPublicConfig(data))
			}
		}

		// Dev fallback: publicDir/app.html. Read into memory (rather than
		// streaming via FileFS) so the same public-config injection applies.
		if data, err := fs.ReadFile(publicFs, "app.html"); err == nil {
			return writeAppShell(e, injectPublicConfig(data))
		}

		return e.NotFoundError("", nil)
	}
}

// writeAppShell sends the injected SPA shell HTML with revalidation caching.
//
// The shell body changes only per release (a new app.html) or when injected
// public config changes, so a content-hash ETag identifies it exactly. We set
// `no-cache` (always revalidate, never serve from cache blindly) plus that
// ETag: an unchanged shell answers a conditional GET with a 304 and no body,
// while a changed shell sends the new bytes. This keeps the shell always-fresh
// — a cached shell would pin content-hashed asset URLs the client has dropped,
// the stale-shell bug — without `no-store`'s full-refetch-every-load cost.
//
// The ETag is computed over the FINAL injected bytes (what the client receives),
// so two deploys with identical shells + config collide correctly (same ETag →
// 304), and any change to either busts it.
func writeAppShell(e *core.RequestEvent, html []byte) error {
	sum := sha256.Sum256(html)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`

	e.Response.Header().Set("Cache-Control", "no-cache")
	e.Response.Header().Set("ETag", etag)
	e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")

	// http.ServeContent isn't usable here (the body is built in memory, not a
	// ReadSeeker file), so match If-None-Match ourselves. A comma-split covers
	// the multi-value form; weak validators ("W/…") never match our strong tag.
	if match := e.Request.Header.Get("If-None-Match"); match != "" {
		for _, candidate := range strings.Split(match, ",") {
			if strings.TrimSpace(candidate) == etag {
				e.Response.WriteHeader(http.StatusNotModified)
				return nil
			}
		}
	}

	_, _ = e.Response.Write(html)
	return nil
}
