package coreserver

import (
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

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

// DefaultWebsiteDir returns the default marketing-website dir, resolved next to
// the running binary as <binaryDir>/website (or "./website" for `go run`). It is
// deliberately SEPARATE from DefaultPublicDir(): org-mode deploys copy the built
// Astro site here, NOT into public/, so the app's `expo export` (which sweeps
// public/ into its web bundle) never absorbs the website. Empty in dev / com mode
// where the dir doesn't exist — StaticWithDynamicFallback treats a missing
// websiteDir as "no website" and serves the app shell as before.
func DefaultWebsiteDir() string {
	if dir := binaryDir(); dir != "" {
		return filepath.Join(dir, "website")
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
		return true, e.FileFS(fs, path)
	}
	indexPath := path + "/index.html"
	if f, err := fs.Open(indexPath); err == nil {
		f.Close()
		return true, e.FileFS(fs, indexPath)
	}
	return false, nil
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

		// SPA fallback. Set no-store on app.html so a tab reload always
		// pulls the active release's shell rather than a cached copy that
		// may reference asset hashes the client has since dropped.
		if releasesDir != "" {
			currentApp := filepath.Join(releasesDir, "current", "app.html")
			if data, err := os.ReadFile(currentApp); err == nil {
				e.Response.Header().Set("Cache-Control", "no-store")
				e.Response.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = e.Response.Write(data)
				return nil
			}
		}

		// Dev fallback: publicDir/app.html.
		if f, err := publicFs.Open("app.html"); err == nil {
			f.Close()
			e.Response.Header().Set("Cache-Control", "no-store")
			return e.FileFS(publicFs, "app.html")
		}

		return e.NotFoundError("", nil)
	}
}
