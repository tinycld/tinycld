package coreserver

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/pocketbase/pocketbase/core"
)

// emptyReleaseManifest is returned (with 200) when no manifest.json is baked
// into the current release — e.g. local/dev images or any build that wasn't
// cut by the release pipeline. The client treats an empty members list as
// "no version inventory available" and hides the section, so a 200 with this
// shape is friendlier than a 404 the client would have to special-case.
var emptyReleaseManifest = map[string]any{"members": []any{}}

// ReleaseHandler returns the pinned-release manifest baked into the current
// release as JSON. The manifest is written by the release pipeline
// (utils/lib/pin-release.ts) and copied into <releasesDir>/current/manifest.json
// by the Docker build; it lists the app tag plus every bundled package's
// tag/sha so the in-app About panel can show exactly what this image ships.
//
// Reads the file on every request (~1KB, OS-page-cached). When the file is
// absent or unreadable it returns the empty-manifest shape with 200 rather
// than erroring, so non-release builds degrade gracefully.
//
// `Cache-Control: no-store` mirrors VersionHandler — the manifest changes
// across deploys and clients should always see the fresh value.
func ReleaseHandler(releasesDir string) func(*core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		e.Response.Header().Set("Cache-Control", "no-store")

		path := filepath.Join(releasesDir, "current", "manifest.json")
		b, err := os.ReadFile(path)
		if err != nil {
			return e.JSON(http.StatusOK, emptyReleaseManifest)
		}

		// Validate it parses so a corrupt file can't poison the client with
		// a non-JSON body; on failure fall back to the empty shape.
		var parsed map[string]any
		if jsonErr := json.Unmarshal(b, &parsed); jsonErr != nil {
			return e.JSON(http.StatusOK, emptyReleaseManifest)
		}
		return e.JSON(http.StatusOK, parsed)
	}
}
