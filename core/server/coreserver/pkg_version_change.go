package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Per-package version change (update or downgrade).
//
// Unlike whole-image build revert (pkg_revert.go), a version change targets ONE
// package's schema by NAME, leaving every other package's data untouched. The
// keystone is pkg_migrate.go's named apply/revert; this file orchestrates the
// file swap + rebuild around it and records the result.
//
// The key enabler: the RUNNING process keeps every migration it registered at
// its own startup in core.AppMigrations, regardless of later on-disk file
// changes. So even after we swap a package's files down to an older version
// (removing the newer migration files from disk), the running binary can still
// execute the newer migrations' Down closures. That lets us use ONE ordering for
// both directions:
//
//  1. swap the package's files to the target version (npm pack / git fetch),
//  2. regenerate wiring — rewrites the migration->owner map so
//     migrationsForPackage(slug) reflects the TARGET version's file set,
//  3. diff against the current build's recorded set:
//     upgrade   -> applyNamedMigrations(target minus current)
//     downgrade -> revertNamedMigrations(current minus target)
//     both run against the still-running process's registered closures,
//  4. rebuild the binary so the post-restart process matches the new file set.
//
// The DB is backed up up front and everything rolls back on any failure.

// versionChange is one package's requested target version.
type versionChange struct {
	Slug          string `json:"slug"`
	TargetVersion string `json:"targetVersion"`
}

// ---------- handler ----------

func handleVersionChange(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		Changes []versionChange `json:"changes"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if len(body.Changes) == 0 {
		return re.BadRequestError("at least one change is required", nil)
	}
	for _, c := range body.Changes {
		if !slugPattern.MatchString(c.Slug) {
			return re.BadRequestError("invalid package slug: "+c.Slug, nil)
		}
		if c.TargetVersion == "" {
			return re.BadRequestError("targetVersion is required for "+c.Slug, nil)
		}
		// Constrain the version charset before it is concatenated into an npm/git
		// install spec. exec.Command uses no shell so this can't *execute*, but a
		// loose value could smuggle an extra arg or option into npm pack / git.
		if !versionTokenPattern.MatchString(c.TargetVersion) {
			return re.BadRequestError("invalid targetVersion for "+c.Slug+": "+c.TargetVersion, nil)
		}
	}

	installMu.Lock()
	if currentJob != nil {
		info := map[string]any{
			"jobId": currentJob.ID, "action": currentJob.Action,
			"slug": currentJob.Slug, "status": currentJob.Status,
		}
		installMu.Unlock()
		return re.JSON(http.StatusConflict, map[string]any{
			"error":      "Another operation is in progress",
			"currentJob": info,
		})
	}
	jobId := fmt.Sprintf("job_%d", time.Now().UnixMilli())
	job := &installJob{
		ID:      jobId,
		Action:  "version_change",
		Slug:    body.Changes[0].Slug,
		Changes: body.Changes,
		Status:  "running",
		Done:    make(chan struct{}),
	}
	currentJob = job
	installMu.Unlock()

	go runVersionChangeRebuild(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": jobId})
}

// handleDropReport previews the schema a downgrade of one package to a SPECIFIC
// target version would destroy, so the UI can warn before the operator confirms.
// Request body:
//
//	{ "slug": "<slug>", "targetVersion": "<version>" }
//
// It computes the exact set the downgrade reverts — the current build's package
// migrations minus the migrations the TARGET version ships — by read-only
// `npm pack`-ing the target and listing its pb-migrations dir (no workspace
// mutation). It then dry-reverts that set inside a transaction it rolls back
// (never commits). The response is {droppedCollections, droppedFields}; an empty
// report means no destructive schema change. If targetVersion is omitted (or the
// target can't be fetched), it falls back to the current build's full set — an
// upper bound — so the UI still gets a (conservative) warning.
func handleDropReport(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		Slug          string `json:"slug"`
		TargetVersion string `json:"targetVersion"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if body.Slug == "" {
		return re.BadRequestError("slug is required", nil)
	}
	if body.TargetVersion != "" && !versionTokenPattern.MatchString(body.TargetVersion) {
		return re.BadRequestError("invalid targetVersion", nil)
	}

	files := currentBuildMigrations(app, body.Slug)
	// Narrow to exactly what the downgrade reverts: current ∖ target. Best-effort
	// — if the target can't be fetched, fall back to the full current set.
	if body.TargetVersion != "" {
		if targetFiles, err := targetMigrationFiles(app, body.Slug, body.TargetVersion); err == nil {
			files = subtractStrings(files, targetFiles)
		} else {
			log.Printf("pkg_version_change: drop-report target fetch failed (%v); reporting full set", err)
		}
	}

	report, err := dryRevertNamedMigrations(app, files)
	if err != nil {
		return re.InternalServerError("Failed to compute drop report", err)
	}
	return re.JSON(http.StatusOK, report)
}

// targetMigrationFiles read-only fetches a package version and returns the
// migration filenames it ships (the basenames in its manifest migrations dir).
// It does NOT touch the workspace — it packs into a temp dir and lists files.
func targetMigrationFiles(app core.App, slug, targetVersion string) ([]string, error) {
	reg, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:s}",
		map[string]any{"s": slug})
	if err != nil {
		return nil, fmt.Errorf("package %q not in registry", slug)
	}
	spec, err := specForVersion(reg.GetString("npm_package"), targetVersion)
	if err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "tinycld-droprep-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	if out, err := runCmd(tmpDir, "npm", "pack", spec, "--pack-destination", tmpDir); err != nil {
		return nil, errFromCmd("npm pack", out, err)
	}
	tgz, _ := filepath.Glob(filepath.Join(tmpDir, "*.tgz"))
	if len(tgz) == 0 {
		return nil, fmt.Errorf("no .tgz produced by npm pack %s", spec)
	}
	if _, err := runCmd(tmpDir, "tar", "xzf", tgz[0], "-C", tmpDir); err != nil {
		return nil, fmt.Errorf("untar: %w", err)
	}
	extractDir := filepath.Join(tmpDir, "package")
	manifest, err := parseManifestViaNode(extractDir)
	if err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	migDir := migrationsDirFromManifest(manifest)
	if migDir == "" {
		return []string{}, nil // target ships no migrations
	}
	entries, err := os.ReadDir(filepath.Join(extractDir, migDir))
	if err != nil {
		return []string{}, nil // no migrations dir in the tarball
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// migrationsDirFromManifest reads manifest.migrations.directory out of RawJSON
// (parsedManifest doesn't surface it as a typed field).
func migrationsDirFromManifest(m *parsedManifest) string {
	mig, ok := m.RawJSON["migrations"].(map[string]any)
	if !ok {
		return ""
	}
	dir, _ := mig["directory"].(string)
	return dir
}

// package's current build, falling back to the global delta narrowed to this
// package for builds predating the pkg_migration_files field.
func currentBuildMigrations(app core.App, slug string) []string {
	build, err := app.FindFirstRecordByFilter(
		"pkg_build",
		"pkg_slug = {:s} && status = 'current'",
		map[string]any{"s": slug},
	)
	if err != nil {
		return nil
	}
	var files []string
	if err := build.UnmarshalJSONField("pkg_migration_files", &files); err == nil && len(files) > 0 {
		return files
	}
	var delta []string
	_ = build.UnmarshalJSONField("migration_files", &delta)
	return intersectStrings(delta, migrationsForPackage(slug))
}

// subtractStrings returns the members of a not present in b.
func subtractStrings(a, b []string) []string {
	set := make(map[string]bool, len(b))
	for _, x := range b {
		set[x] = true
	}
	out := make([]string, 0)
	for _, x := range a {
		if !set[x] {
			out = append(out, x)
		}
	}
	return out
}

// ---------- file swap / regen / rebuild helpers ----------
