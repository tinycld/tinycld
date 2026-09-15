package coreserver

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"
)

// PoolAssets serves files from <releasesDir>/_static/<prefix>/<path>.
//
// The pool is a flat store written by the entrypoint on each container
// start: every promoted release's _expo/static/ and assets/ trees get merged
// into the same shared directory, the newest release's copy winning on a
// name collision.
//
// Expo's chunk filenames are NOT content hashes. The hash covers the chunk's
// own module code with the paths of the async chunks it links to stripped
// out (@expo/metro-config's getStableChunkSource serializes with
// computedAsyncModulePaths: null). So when only a lazily-loaded route
// changes, the entry bundle keeps its filename while its chunk table now
// names different files. Served as `immutable`, a browser kept the old entry
// for a year and lazy-loaded chunks that no longer matched the bundles
// beside it — "Requiring unknown module" on every load, unfixable by the
// server because the URL never changed. Callers therefore pass a
// revalidating policy for _expo/static: FileFS answers a conditional GET via
// Last-Modified, so an unchanged file still costs only a 304.
//
// Stale tabs whose chunk filename is still present in the pool keep
// working across deploys; once the pool is pruned, the missing-chunk path
// 404s and the client reload picks up the active release.
//
// cacheControl is set on every successful response so callers can choose
// per-route policies.
func PoolAssets(releasesDir, prefix, cacheControl string) func(*core.RequestEvent) error {
	root := filepath.Join(releasesDir, "_static", prefix)
	fs := os.DirFS(root)

	return func(e *core.RequestEvent) error {
		if e.Request.Method != http.MethodGet && e.Request.Method != http.MethodHead {
			return e.Next()
		}

		path := e.Request.PathValue("path")
		f, err := fs.Open(path)
		if err != nil {
			return e.NotFoundError("", nil)
		}
		f.Close()

		if cacheControl != "" {
			e.Response.Header().Set("Cache-Control", cacheControl)
		}
		return e.FileFS(fs, path)
	}
}
