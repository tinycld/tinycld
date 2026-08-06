package coreserver

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgbuild"
)

// cliDownloadEntry is one downloadable CLI binary in the /api/cli/downloads
// listing.
type cliDownloadEntry struct {
	// Platform is the URL key ("darwin-arm64", GOOS-GOARCH).
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	// Filename is the suggested save name ("tinycld" / "tinycld.exe").
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}

// RegisterCliDownloadEndpoints wires the public CLI download endpoints for the
// HOST composition, serving from the activated build dir's cli-dist/ — the
// pipeline writes the cross-compiled binaries next to the server binary, and
// `current` points at builds/<id>/tinycld, so resolveServerDir() finds them.
// A tenant serves its artifact's own copy via
// RegisterTenantCliDownloadEndpoints.
//
// Public like /api/app/update and /api/release (see registerCliDownloadEndpoints).
func RegisterCliDownloadEndpoints(app *pocketbase.PocketBase) {
	registerCliDownloadEndpoints(app, func() string {
		return filepath.Join(resolveServerDir(), pkgbuild.CLIDistDirName)
	})
}

// RegisterTenantCliDownloadEndpoints serves a tenant's CLI binaries from its
// build artifact (the multi-org builder stages cli-dist into the artifact).
func RegisterTenantCliDownloadEndpoints(app core.App, artifactDir string) {
	registerCliDownloadEndpoints(app, func() string {
		return filepath.Join(artifactDir, pkgbuild.CLIDistDirName)
	})
}

// registerCliDownloadEndpoints binds the shared handlers against a dist-dir
// resolver. Takes core.App so the HTTP tests drive the real registration.
//
// Both routes are PUBLIC: a browser download link cannot carry PocketBase's
// Authorization header, and the precedent is established — /api/app/update
// serves the org's full JS bundles and /api/release its package inventory,
// both unauthenticated. A CLI binary reveals nothing more. The OAuth grant
// middleware exempts /api/cli/ for the same reason (see oauth.exemptPaths).
func registerCliDownloadEndpoints(app core.App, distDir func() string) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/cli")

		g.GET("/downloads", func(re *core.RequestEvent) error {
			// [] not null when nothing is built (dev server, fresh image
			// before the first rebuild) — the UI hides the section on empty.
			entries := []cliDownloadEntry{}
			dir := distDir()
			for _, t := range pkgbuild.CLITargets {
				info, err := os.Stat(filepath.Join(dir, t.FileName()))
				if err != nil {
					continue
				}
				entries = append(entries, cliDownloadEntry{
					Platform: t.Platform(),
					OS:       t.OS,
					Arch:     t.Arch,
					Filename: t.DownloadName(),
					Size:     info.Size(),
					URL:      "/api/cli/download/" + t.Platform(),
				})
			}
			return re.JSON(http.StatusOK, map[string]any{"downloads": entries})
		})

		g.GET("/download/{platform}", func(re *core.RequestEvent) error {
			// Resolve ONLY by lookup in the fixed target list — the path value
			// never touches the filesystem, so it cannot traverse (same
			// discipline as serveBuildFile's platform allowlist).
			requested := re.Request.PathValue("platform")
			for _, t := range pkgbuild.CLITargets {
				if t.Platform() != requested {
					continue
				}
				fsys := os.DirFS(distDir())
				if f, err := fsys.Open(t.FileName()); err != nil {
					return re.NotFoundError("No binary built for this platform", nil)
				} else {
					f.Close()
				}
				re.Response.Header().Set("Content-Disposition",
					fmt.Sprintf("attachment; filename=%q", t.DownloadName()))
				re.Response.Header().Set("Content-Type", "application/octet-stream")
				return re.FileFS(fsys, t.FileName())
			}
			return re.NotFoundError("Unknown platform", nil)
		})

		return e.Next()
	})
}
