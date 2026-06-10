package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
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

	go runVersionChangePipeline(app, job)

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

// ---------- pipeline ----------

func runVersionChangePipeline(app *pocketbase.PocketBase, job *installJob) {
	defer func() {
		installMu.Lock()
		currentJob = nil
		installMu.Unlock()
		close(job.Done)
	}()

	appDir := resolveServerDir()
	logRecord := createInstallLog(app, job, "version_change")

	var rollbackStack []func()
	rollback := func() {
		for i := len(rollbackStack) - 1; i >= 0; i-- {
			rollbackStack[i]()
		}
	}
	// Tracks whether a destructive, in-process schema mutation has run. Unlike the
	// install/revert pipelines (which run `migrate` in a SUBPROCESS, leaving this
	// process's DB connection untouched), a version change applies/reverts
	// migrations in-process against the live connection. So a rollback that
	// restores the DB by file-copy (backupDatabase's rollback) leaves the running
	// process's open handle inconsistent with the file. When that happens we MUST
	// restart so the supervisor relaunches against the restored database.
	dbMutated := false
	fail := func(step string, err error) {
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed at %s: %v", step, err)
		emitProgress(job, step, job.Progress, "FAILED: "+err.Error())
		emitComplete(job, "failed", job.Error)
		rollback()
		finalizeInstallLog(app, logRecord, "failed", job.Error, job.LogLines)
		if dbMutated {
			// The DB was restored from backup over the live, open file — the
			// running process can't be trusted to keep serving. Relaunch.
			log.Printf("pkg_version_change: restarting after DB-restoring rollback")
			time.Sleep(2 * time.Second)
			requestRestart(appDir)
		}
	}

	// Step 1: Re-validate compatibility authoritatively (5%). The UI checks before
	// enabling Apply, but state can change between check and apply; re-run the same
	// solver against the live registry + proposed targets before mutating anything.
	emitProgress(job, "Checking compatibility", 5, "Validating version set")
	if violations := checkChangeCompat(app, job.Changes); len(violations) > 0 {
		fail("compatibility", compatError(violations))
		return
	}

	// Step 2: Back up the DB once up front (15%) — the downgrade safety net.
	emitProgress(job, "Backing up database", 15, "Creating SQLite backup")
	dbRollback, dbErr := backupDatabase(appDir)
	if dbErr != nil {
		fail("database backup", dbErr)
		return
	}
	rollbackStack = append(rollbackStack, func() {
		if err := dbRollback(); err != nil {
			log.Printf("pkg_version_change: rollback — database restore failed: %v", err)
		}
	})

	// Step 3: Process each package change. Each call records its own per-package
	// rollback steps onto the shared stack, so a late failure unwinds everything.
	total := len(job.Changes)
	for i, change := range job.Changes {
		base := 20 + (i*70)/total
		emitProgress(job, "Changing version", base,
			fmt.Sprintf("%s -> %s (%d/%d)", change.Slug, change.TargetVersion, i+1, total))
		if err := applyOneVersionChange(app, job, change, base, &rollbackStack, &dbMutated); err != nil {
			fail(fmt.Sprintf("change %s", change.Slug), err)
			return
		}
	}

	// All changes succeeded — drop the member backups swapPackageFiles left under
	// wsRoot/backups/ (they're only needed for the rollback path). Leaving them
	// would leak a full copy of each member dir on every successful change. Remove
	// the whole backups/ dir so it never lingers in the workspace root.
	wsRoot := filepath.Dir(appDir)
	os.RemoveAll(filepath.Join(wsRoot, backupsDirName))

	emitProgress(job, "Requesting restart", 95, "Signaling server restart")
	job.Status = "success"
	finalizeInstallLog(app, logRecord, "success", "", job.LogLines)
	emitComplete(job, "success", "")

	time.Sleep(2 * time.Second)
	requestRestart(appDir)
}

// applyOneVersionChange performs a single package's update or downgrade following
// the swap -> regen -> migrate -> rebuild order described in the file header.
// Per-package rollbacks are appended to rollbackStack.
func applyOneVersionChange(
	app *pocketbase.PocketBase,
	job *installJob,
	change versionChange,
	baseProgress int,
	rollbackStack *[]func(),
	dbMutated *bool,
) error {
	appDir := resolveServerDir()
	wsRoot := filepath.Dir(appDir)

	reg, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:s}",
		map[string]any{"s": change.Slug})
	if err != nil {
		return fmt.Errorf("package %q not in registry", change.Slug)
	}
	current := reg.GetString("version")
	dir, dirErr := versionDirection(change.TargetVersion, current)
	if dirErr != nil {
		return dirErr
	}
	if dir == directionSame {
		emitProgress(job, "Changing version", baseProgress,
			fmt.Sprintf("%s already at %s — skipping", change.Slug, current))
		return nil
	}
	downgrade := dir == directionDown

	// Migrations the current build added for this package — the set a downgrade
	// reverts down from / an upgrade diffs against.
	currentPkgMigrations := currentBuildMigrations(app, change.Slug)

	targetSpec, specErr := specForVersion(reg.GetString("npm_package"), change.TargetVersion)
	if specErr != nil {
		return specErr
	}

	// 1. Swap files to the target version. The base (`core`) is the whole
	// app-shell repo, fetched by git clone of its source repo and swapped
	// source-only (preserving runtime state); a feature is an npm-pack of one
	// sibling dir. Downstream (regen -> migrate -> rebuild -> stage -> archive ->
	// restart) is identical — swapBaseFiles returns HasServer:true so the existing
	// rebuild gate fires for core with no slug-based branching there.
	emitProgress(job, "Installing files", baseProgress+4, "Fetching "+targetSpec)
	var manifest *parsedManifest
	var swapErr error
	if change.Slug == "core" {
		// The base clones its whole repo from the registry's source spec at the
		// bare target ref (the version tag), not a feature npm-pack spec.
		manifest, swapErr = swapBaseFiles(app, job, reg.GetString("npm_package"), change.TargetVersion, wsRoot, appDir, rollbackStack)
	} else {
		manifest, swapErr = swapPackageFiles(app, job, targetSpec, change.Slug, wsRoot, appDir, rollbackStack)
	}
	if swapErr != nil {
		return swapErr
	}

	// 1b. Authoritative compat re-check against the FETCHED target manifest: the
	// pre-flight gate only saw installed manifests, so a target that tightens its
	// own peerVersions is caught here, before its migrations run.
	manifestJSON, _ := json.Marshal(manifest.RawJSON)
	if v := verifyTargetPeerVersions(change.Slug, string(manifestJSON), resolvedWithChanges(app, job.Changes)); len(v) > 0 {
		return compatError(v)
	}

	// 2. Regenerate wiring so migrationsForPackage(slug) reflects the target set.
	emitProgress(job, "Regenerating", baseProgress+8, "Rewiring packages")
	if genErr := regenerateWiring(appDir, rollbackStack); genErr != nil {
		return genErr
	}
	targetPkgMigrations := migrationsForPackage(change.Slug)

	// 3. Step the schema to the target. appliedDelta is the migrations this change
	// actually applied (upgrade) — empty for a downgrade, which only reverts.
	// From here on the live DB schema is mutated: signal the caller so a later
	// failure restarts after restoring the DB backup.
	//
	// DIRECTION ASYMMETRY (important): a DOWNGRADE reverts in-process via the
	// running binary's still-registered Down closures — the migration being
	// reverted WAS registered at this process's startup (we booted on the newer
	// version), so revertNamedMigrations can run it even after the file is swapped
	// off disk. An UPGRADE cannot apply in-process: the NEW migration only exists
	// in the just-swapped files and was NEVER registered in this (older) process's
	// core.AppMigrations, so applyNamedMigrations would fail with "not registered".
	// Instead we apply the upgrade the same way the install pipeline does — shell
	// out to the current binary's `migrate` subcommand, which boots fresh, has jsvm
	// re-scan the (now-updated) pb_migrations/ dir, registers the new .js file, and
	// applies all pending. Only this package's migration is pending at this point
	// (we just regenerated for a single-package change), so "apply pending" == the
	// package's new migrations. We snapshot _migrations before/after to record the
	// exact applied delta for the build record.
	*dbMutated = true
	var appliedDelta []string
	if downgrade {
		toRevert := subtractStrings(currentPkgMigrations, targetPkgMigrations)
		emitProgress(job, "Reverting migrations", baseProgress+12,
			fmt.Sprintf("Rolling back %d migration(s)", len(toRevert)))
		if _, rErr := revertNamedMigrations(app, toRevert); rErr != nil {
			return fmt.Errorf("revert migrations: %w", rErr)
		}
	} else {
		toApply := subtractStrings(targetPkgMigrations, currentPkgMigrations)
		emitProgress(job, "Applying migrations", baseProgress+12,
			fmt.Sprintf("Applying %d migration(s)", len(toApply)))
		before, beforeErr := appliedMigrationFiles(app)
		if beforeErr != nil {
			log.Printf("pkg_version_change: snapshot before apply failed: %v", beforeErr)
		}
		// Use the CURRENT on-disk binary's migrate subcommand: jsvm scans the
		// migrations DIR (independent of the binary's compiled-in Go migration set),
		// so the just-swapped .js file is picked up and applied even though the new
		// binary isn't built yet (that happens in finalizeVersionChange).
		migrateBin := resolveServerBinary()
		if out, mErr := runCmd(appDir, migrateBin, "migrate"); mErr != nil {
			return fmt.Errorf("apply migrations: %w: %s", mErr, out)
		}
		after, afterErr := appliedMigrationFiles(app)
		if afterErr != nil {
			log.Printf("pkg_version_change: snapshot after apply failed: %v", afterErr)
		}
		// Record exactly what the apply added, narrowed to this package's owned
		// files (the same source-of-truth intersection the install pipeline uses).
		appliedDelta = intersectStrings(newMigrationFiles(before, after), targetPkgMigrations)
		// Defensive: if snapshots were unavailable, fall back to the intended set so
		// the build record isn't silently empty.
		if len(appliedDelta) == 0 && len(toApply) > 0 && (beforeErr != nil || afterErr != nil) {
			appliedDelta = toApply
		}
	}

	// 4. Rebuild the binary + web bundle so the post-restart process matches the
	// new file set, then record the build and update the registry.
	emitProgress(job, "Rebuilding", baseProgress+16, "Building "+change.Slug)
	return finalizeVersionChange(app, job, reg, change, targetSpec, manifest,
		targetPkgMigrations, appliedDelta, appDir, rollbackStack)
}

// versionDirection compares a target version to the current one with semver
// semantics (so 1.0 == 1.0.0). It returns directionSame / directionUp /
// directionDown, or an error if either version is unparsable — we never guess a
// direction, because guessing "downgrade" would silently revert migrations.
func versionDirection(target, current string) (versionDir, error) {
	t, err := semver.NewVersion(target)
	if err != nil {
		return directionSame, fmt.Errorf("unparsable target version %q: %w", target, err)
	}
	c, err := semver.NewVersion(current)
	if err != nil {
		return directionSame, fmt.Errorf("unparsable current version %q: %w", current, err)
	}
	switch {
	case t.Equal(c):
		return directionSame, nil
	case t.GreaterThan(c):
		return directionUp, nil
	default:
		return directionDown, nil
	}
}

type versionDir int

const (
	directionSame versionDir = iota
	directionUp
	directionDown
)

// resolvedWithChanges builds the post-change version map (current versions with
// proposed targets overlaid, plus @tinycld/core) for the compat solver.
func resolvedWithChanges(app core.App, changes []versionChange) map[string]string {
	resolved := map[string]string{}
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return resolved
	}
	changeMap := map[string]string{}
	for _, c := range changes {
		changeMap[c.Slug] = c.TargetVersion
	}
	for _, rec := range records {
		slug := rec.GetString("slug")
		if t, ok := changeMap[slug]; ok {
			resolved[slug] = t
		} else {
			resolved[slug] = rec.GetString("version")
		}
	}
	if coreVer, ok := resolved["core"]; ok {
		resolved[corePackageKey] = coreVer
	}
	return resolved
}

// checkChangeCompat re-runs the compatibility solver against the live registry
// with the proposed changes overlaid, returning any violations.
func checkChangeCompat(app core.App, changes []versionChange) []compatViolation {
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return []compatViolation{{Package: "(registry)", Found: err.Error()}}
	}
	changeMap := map[string]string{}
	for _, c := range changes {
		changeMap[c.Slug] = c.TargetVersion
	}
	resolved := map[string]string{}
	peers := map[string]map[string]string{}
	for _, rec := range records {
		slug := rec.GetString("slug")
		if t, ok := changeMap[slug]; ok {
			resolved[slug] = t
		} else {
			resolved[slug] = rec.GetString("version")
		}
		if p := peerVersionsFromManifest(rec.GetString("manifest_json")); len(p) > 0 {
			peers[slug] = p
		}
	}
	if coreVer, ok := resolved["core"]; ok {
		resolved[corePackageKey] = coreVer
	}
	return solveCompat(resolved, peers)
}

// currentBuildMigrations returns the per-package migration files recorded on the
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

// swapPackageFiles fetches the target version's tarball, validates its manifest,
// and copies it over the package's workspace member dir. Returns the parsed
// target manifest. Records a rollback that restores the prior member contents.
func swapPackageFiles(
	app *pocketbase.PocketBase,
	job *installJob,
	spec, slug, wsRoot, appDir string,
	rollbackStack *[]func(),
) (*parsedManifest, error) {
	tmpDir, err := os.MkdirTemp("", "tinycld-ver-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	packOut, err := runCmd(wsRoot, "npm", "pack", spec, "--pack-destination", tmpDir)
	if err != nil {
		return nil, errFromCmd("npm pack", packOut, err)
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
	if manifest.Slug != slug {
		return nil, fmt.Errorf("fetched package slug %q does not match %q", manifest.Slug, slug)
	}

	// Back up the existing member dir so a failure restores it intact. The backup
	// goes under wsRoot/backups/<slug>, NOT a wsRoot/<slug>.bak sibling: getPackages()
	// (tinycld.packages.ts) and the generator enumerate members by scanning wsRoot
	// ONE level deep for dirs with a package.json, so a sibling .bak — a full cp -a
	// of the member, package.json + manifest.ts and all — gets counted as a SECOND
	// copy of the package, producing a duplicate `go.work` use entry and failing the
	// rebuild with "go work sync: path <member>/server appears multiple times". The
	// backups/ dir has no package.json of its own and its contents sit two levels
	// deep, so the scan never sees them. It lives in wsRoot (not os.TempDir) so the
	// rollback `mv` is a same-filesystem rename, not a cross-device copy.
	pkgDest := filepath.Join(wsRoot, slug)
	backupDir := filepath.Join(wsRoot, backupsDirName, slug)
	os.RemoveAll(backupDir)
	if err := os.MkdirAll(filepath.Dir(backupDir), 0o755); err != nil {
		return nil, fmt.Errorf("create backups dir: %w", err)
	}
	if _, err := runCmd(".", "cp", "-a", pkgDest, backupDir); err != nil {
		return nil, fmt.Errorf("backup member dir: %w", err)
	}
	*rollbackStack = append(*rollbackStack, func() {
		os.RemoveAll(pkgDest)
		if _, err := runCmd(".", "mv", backupDir, pkgDest); err != nil {
			log.Printf("pkg_version_change: rollback — restore %s failed: %v", slug, err)
		}
	})

	os.RemoveAll(pkgDest)
	if err := copyDir(extractDir, pkgDest); err != nil {
		return nil, fmt.Errorf("copy package files: %w", err)
	}
	// Dropping the backup on success is deferred to job completion (see the
	// backups/ cleanup in runVersionChangePipeline); the rollback above handles the
	// failure path. The backup stays under wsRoot/backups/ for the run's duration.
	return manifest, nil
}

// backupsDirName is the per-run member-backup root under the workspace root. It
// deliberately has no package.json/manifest.ts so member enumeration (which scans
// the workspace root one level deep) never treats a backed-up member as a second
// installed copy of its package.
const backupsDirName = "backups"

// regenerateWiring re-runs the generator (rewrites routes, owner map, go wiring)
// and records a rollback that regenerates again after a member restore.
func regenerateWiring(appDir string, rollbackStack *[]func()) error {
	out, err := runCmd(appDir, "npx", "tsx", "scripts/generate.ts")
	if err != nil {
		return errFromCmd("generate", out, err)
	}
	// Reset the owner-map cache so post-regen lookups read the rewritten file.
	resetMigrationOwnersCache()
	*rollbackStack = append(*rollbackStack, func() {
		runCmd(appDir, "npx", "tsx", "scripts/generate.ts")
		resetMigrationOwnersCache()
	})
	return nil
}

// finalizeVersionChange rebuilds the binary (if the package has a server),
// rebuilds + stages the web bundle, archives a new build snapshot, and updates
// the registry to the target version. pnpm install relinks the swapped member.
func finalizeVersionChange(
	app *pocketbase.PocketBase,
	job *installJob,
	reg *core.Record,
	change versionChange,
	targetSpec string,
	manifest *parsedManifest,
	targetPkgMigrations []string,
	appliedDelta []string,
	appDir string,
	rollbackStack *[]func(),
) error {
	wsRoot := filepath.Dir(appDir)
	goSrcDir := filepath.Join(appDir, "server")

	// Relink the workspace so the swapped member's deps resolve.
	if out, err := runCmdEnv(wsRoot, []string{"CI=true"}, "pnpm", "install", "--no-frozen-lockfile"); err != nil {
		return errFromCmd("pnpm install", out, err)
	}

	if manifest.HasServer {
		if out, err := runCmd(goSrcDir, "go", "work", "sync"); err != nil {
			return errFromCmd("go work sync", out, err)
		}
		if err := buildNewBinary(goSrcDir, appDir); err != nil {
			return fmt.Errorf("go build: %w", err)
		}
		*rollbackStack = append(*rollbackStack, func() {
			os.Remove(filepath.Join(appDir, "tinycld.new"))
		})
		if err := validateBinary(filepath.Join(appDir, "tinycld.new")); err != nil {
			return fmt.Errorf("validate binary: %w", err)
		}
		binRollback, err := swapBinary(appDir)
		if err != nil {
			return fmt.Errorf("binary swap: %w", err)
		}
		*rollbackStack = append(*rollbackStack, func() {
			if rbErr := binRollback(); rbErr != nil {
				log.Printf("pkg_version_change: rollback — binary restore failed: %v", rbErr)
			}
		})
	}

	// Rebuild + stage the web bundle.
	if out, err := runCmd(appDir, "npx", "expo", "export", "--platform", "web"); err != nil {
		return errFromCmd("expo export", out, err)
	}
	stageDest, err := stageRelease(appDir)
	if err != nil {
		return fmt.Errorf("stage release: %w", err)
	}
	*rollbackStack = append(*rollbackStack, func() { os.RemoveAll(stageDest) })

	// Update the registry version + spec, and archive a new build snapshot.
	manifestJSON, _ := json.Marshal(manifest.RawJSON)
	if err := upsertPkgRegistry(app, manifest, targetSpec, manifestJSON); err != nil {
		return fmt.Errorf("registry update: %w", err)
	}

	buildID := fmt.Sprintf("build-%d", time.Now().UnixMilli())
	releaseID := filepath.Base(stageDest)
	// migration_files / migrations_applied feed whole-image build revert's
	// count-based `migrate down N` and tail check, so they must record only what
	// this build ADDED — appliedDelta (empty for a downgrade, which reverts and
	// adds nothing). pkg_migration_files records the package's FULL target set, so
	// a future per-package version change can diff against it.
	if appliedDelta == nil {
		appliedDelta = []string{}
	}
	buildFields := map[string]any{
		"build_id":            buildID,
		"pkg_slug":            change.Slug,
		"npm_package":         targetSpec,
		"version":             change.TargetVersion,
		"action":              "install",
		"binary_archived":     manifest.HasServer,
		"release_id":          releaseID,
		"migrations_applied":  len(appliedDelta),
		"migration_files":     appliedDelta,
		"pkg_migration_files": targetPkgMigrations,
		"notes":               fmt.Sprintf("Version change to %s", change.TargetVersion),
	}
	_, archiveCleanup, archErr := archiveBuild(appDir, buildID, stageDest, manifest.HasServer, buildFields)
	if archErr != nil {
		return fmt.Errorf("archive build: %w", archErr)
	}
	*rollbackStack = append(*rollbackStack, archiveCleanup)
	if _, err := recordBuild(app, buildFields); err != nil {
		return fmt.Errorf("record build: %w", err)
	}
	return nil
}
