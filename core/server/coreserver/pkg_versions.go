package coreserver

import (
	"net/http"
	"sync"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Package version discovery. The mechanics (spec classification, npm/git
// listing, the discovery cache, semver sorting) moved to pkgbuild/versions.go so
// the hosting router can serve the same discovery over the per-org control
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

// versionsFanoutLimit caps how many discovery calls run at once. Each one
// shells out to `git ls-remote` or `npm view`, so this bounds the subprocesses
// and outbound connections a single request can spawn — a registry with dozens
// of rows must not fork dozens of gits simultaneously.
const versionsFanoutLimit = 8

// handleVersions returns version info for every package in pkg_registry that has
// a resolvable source spec. A per-package fetch failure is reported in that
// row's Error field rather than failing the whole response.
func handleVersions(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return re.InternalServerError("Failed to load package registry", err)
	}

	return re.JSON(http.StatusOK, map[string]any{
		"packages": versionInfosForRows(records, versionsForSpec),
	})
}

// versionInfosForRows discovers every row CONCURRENTLY, bounded by
// versionsFanoutLimit. Each row is an independent network round-trip, and
// serially they summed to seconds of wall-clock for a normal package set — the
// Packages screen's whole load time. The shared discovery cache is
// mutex-guarded and each goroutine writes only its own slot, so the fan-out
// needs no further synchronization.
//
// Results are written BY INDEX, so the caller's ordering (the registry's `slug`
// sort) survives regardless of which discovery finishes first.
func versionInfosForRows(
	records []*core.Record,
	discover func(spec string) (pkgSource, []string, string),
) []versionInfo {
	infos := make([]versionInfo, len(records))
	sem := make(chan struct{}, versionsFanoutLimit)
	var wg sync.WaitGroup
	for i, rec := range records {
		wg.Add(1)
		go func(i int, rec *core.Record) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			infos[i] = versionInfoForRegistryRow(rec, discover)
		}(i, rec)
	}
	wg.Wait()
	return infos
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
