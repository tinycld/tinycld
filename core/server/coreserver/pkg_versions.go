package coreserver

import (
	"net/http"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Package version discovery. The mechanics (spec classification, npm/git
// listing, the 60s cache, semver sorting) moved to pkgbuild/versions.go so the
// multi-org router can serve the same discovery over the per-org control
// socket; this file keeps only the DB-backed endpoint.

// versionInfo is the per-package discovery result returned to the UI.
type versionInfo struct {
	Slug      string    `json:"slug"`
	Source    pkgSource `json:"source"`
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	Available []string  `json:"available"` // descending (newest first), semver-sorted
	HasUpdate bool      `json:"hasUpdate"`
	Error     string    `json:"error,omitempty"` // per-package fetch failure; others still returned
}

// ---------- discovery endpoint ----------

// handleVersions returns version info for every package in pkg_registry that has
// a resolvable source spec. A per-package fetch failure is reported in that
// row's Error field rather than failing the whole response.
func handleVersions(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return re.InternalServerError("Failed to load package registry", err)
	}

	infos := make([]versionInfo, 0, len(records))
	for _, rec := range records {
		infos = append(infos, versionInfoForRegistryRow(rec, versionsForSpec))
	}

	return re.JSON(http.StatusOK, map[string]any{"packages": infos})
}

// versionInfoForRegistryRow builds one registry row's discovery result.
// discover is the versions source — versionsForSpec on the host, the
// control-socket call in a hosted tenant — so both paths share the row shape.
func versionInfoForRegistryRow(rec *core.Record, discover func(spec string) (pkgSource, []string, string)) versionInfo {
	spec := rec.GetString("npm_package")
	current := rec.GetString("version")
	info := versionInfo{
		Slug:    rec.GetString("slug"),
		Current: current,
		// Always a non-nil slice so it marshals as `[]`, never `null`: the
		// client types `available` as string[] and calls `.length`/`.indexOf`
		// on it (the Packages version controls, detectDowngrade), which throw
		// on null. A nil Go slice JSON-encodes to `null`, so initialize it
		// here — the unknown-source path below relies on this default.
		Available: []string{},
	}
	if spec == "" {
		// Bundled packages with no install spec have no external source.
		info.Source = sourceUnknown
		return info
	}
	src, versions, fetchErr := discover(spec)
	info.Source = src
	if versions != nil {
		info.Available = versions
	}
	info.Error = fetchErr
	if len(versions) > 0 {
		info.Latest = versions[0]
		info.HasUpdate = isNewer(versions[0], current)
	}
	return info
}
