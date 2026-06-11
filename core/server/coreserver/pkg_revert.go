package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// ---------- revert handler ----------

func handleRevert(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		BuildID string `json:"buildId"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if body.BuildID == "" {
		return re.BadRequestError("buildId is required", nil)
	}

	installMu.Lock()
	if currentJob != nil {
		info := map[string]any{
			"jobId":  currentJob.ID,
			"action": currentJob.Action,
			"slug":   currentJob.Slug,
			"status": currentJob.Status,
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
		Action:  "revert",
		BuildID: body.BuildID,
		Status:  "running",
		Done:    make(chan struct{}),
	}
	currentJob = job
	installMu.Unlock()

	go runRevertPipeline(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": jobId})
}

// ---------- delete-build handler ----------

func handleDeleteBuild(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		BuildID string `json:"buildId"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if body.BuildID == "" {
		return re.BadRequestError("buildId is required", nil)
	}

	record, err := app.FindFirstRecordByFilter(
		"pkg_build",
		"build_id = {:id}",
		map[string]any{"id": body.BuildID},
	)
	if err != nil {
		return re.NotFoundError("Build not found", nil)
	}
	if record.GetString("status") == "current" {
		return re.BadRequestError("Cannot delete the current build", nil)
	}

	appDir := resolveServerDir()
	arch := buildArchiveFor(appDir, body.BuildID)
	if err := os.RemoveAll(arch.root); err != nil {
		return re.InternalServerError("Failed to remove build archive", err)
	}
	if err := app.Delete(record); err != nil {
		return re.InternalServerError("Failed to delete build record", err)
	}

	return re.JSON(http.StatusOK, map[string]any{"deleted": body.BuildID})
}

// ---------- revert pipeline ----------

// runRevertPipeline restores a previously-archived build: swaps in its server
// binary, steps the database schema back down past every newer build's
// migrations, re-stages its web bundle, marks newer builds superseded, and
// relaunches via the exit-75 supervisor loop. Reverting is one-way — the
// superseded builds become permanently unreachable (their migrations are torn
// down and their binaries assume schema that no longer exists).
func runRevertPipeline(app *pocketbase.PocketBase, job *installJob) {
	defer func() {
		installMu.Lock()
		currentJob = nil
		installMu.Unlock()
		close(job.Done)
	}()

	appDir := resolveServerDir()

	var rollbackStack []func()
	rollback := func() {
		for i := len(rollbackStack) - 1; i >= 0; i-- {
			rollbackStack[i]()
		}
	}

	// Resolve the target build first so the install-log record carries its slug.
	target, err := app.FindFirstRecordByFilter(
		"pkg_build",
		"build_id = {:id}",
		map[string]any{"id": job.BuildID},
	)
	if err != nil {
		job.Slug = job.BuildID
		logRecord := createInstallLog(app, job, "revert")
		job.Status = "failed"
		job.Error = "Build not found: " + job.BuildID
		emitComplete(job, "failed", job.Error)
		finalizeInstallLog(app, logRecord, "failed", job.Error, job.LogLines)
		return
	}
	job.Slug = target.GetString("pkg_slug")
	logRecord := createInstallLog(app, job, "revert")

	fail := func(step string, err error) {
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed at %s: %v", step, err)
		emitProgress(job, step, job.Progress, "FAILED: "+err.Error())
		emitComplete(job, "failed", job.Error)
		rollback()
		finalizeInstallLog(app, logRecord, "failed", job.Error, job.LogLines)
	}

	// Step 1: Validate target (5%)
	emitProgress(job, "Validating build", 5, "Checking "+job.BuildID)
	if target.GetString("status") == "current" {
		fail("validate", fmt.Errorf("build %s is already current", job.BuildID))
		return
	}
	if target.GetString("status") == "superseded" {
		fail("validate", fmt.Errorf("build %s was reverted past and is no longer reachable", job.BuildID))
		return
	}
	arch := buildArchiveFor(appDir, job.BuildID)
	if _, statErr := os.Stat(arch.release); statErr != nil {
		fail("validate", fmt.Errorf("build archive missing or incomplete (no release bundle)"))
		return
	}
	hasServer := target.GetBool("binary_archived")
	if hasServer {
		if _, statErr := os.Stat(arch.binary); statErr != nil {
			fail("validate", fmt.Errorf("build archive missing server binary"))
			return
		}
	}

	// Step 2: Migration safety gate + compute down-count (15%). Sum the
	// migrations every build newer than the target applied; that's the number of
	// migrations `migrate down` must reverse. Confirm the live _migrations tail
	// still matches the recorded chain before stepping down.
	emitProgress(job, "Checking migrations", 15, "Computing schema rollback")
	newer, newerErr := buildsNewerThan(app, target)
	if newerErr != nil {
		fail("migration check", newerErr)
		return
	}
	downCount, expectedTail := plannedMigrationDown(newer)
	if gateErr := verifyMigrationTail(app, expectedTail); gateErr != nil {
		fail("migration check", gateErr)
		return
	}

	// Forward case: when the target's schema is AHEAD of the live state (a newer
	// build downgraded the package, tearing the target's migrations down), there's
	// nothing to step down (downCount == 0) but the target's own migrations are
	// missing and must be re-applied. Compute the migrations the target build
	// expects (its full set) that are NOT currently applied — after the Step 4
	// binary swap those are exactly the ones a forward `migrate` will re-apply.
	var targetFiles []string
	if uErr := target.UnmarshalJSONField("pkg_migration_files", &targetFiles); uErr != nil || len(targetFiles) == 0 {
		_ = target.UnmarshalJSONField("migration_files", &targetFiles)
	}
	applied, appliedErr := appliedMigrationFiles(app)
	if appliedErr != nil {
		fail("migration check", appliedErr)
		return
	}
	forwardSet := subtractStrings(targetFiles, applied)

	// Step 3: Backup the current DB (the revert operation's own safety net) (25%)
	emitProgress(job, "Backing up database", 25, "Creating SQLite backup")
	dbRollback, dbErr := backupDatabase(appDir)
	if dbErr != nil {
		fail("database backup", dbErr)
		return
	}
	rollbackStack = append(rollbackStack, func() {
		if err := dbRollback(); err != nil {
			log.Printf("pkg_revert: rollback — database restore failed: %v", err)
		}
	})

	// Step 4: Swap in the archived binary (40%). migrate down (next step) runs
	// with this older binary, which understands the older schema.
	migrateBin := resolveServerBinary()
	if hasServer {
		emitProgress(job, "Swapping binary", 40, "Installing archived server binary")
		binRollback, binErr := swapToArchivedBinary(appDir, arch.binary)
		if binErr != nil {
			fail("binary swap", binErr)
			return
		}
		rollbackStack = append(rollbackStack, func() {
			if err := binRollback(); err != nil {
				log.Printf("pkg_revert: rollback — binary restore failed: %v", err)
			}
		})
		migrateBin = filepath.Join(appDir, binaryName)
	}

	// Step 5: Reconcile schema to the target (55%). Either step newer builds'
	// migrations back down, or (the forward case) re-apply the target's own
	// migrations that a later downgrade tore off.
	if downCount > 0 {
		emitProgress(job, "Reversing migrations", 55,
			fmt.Sprintf("Running migrate down %d", downCount))
		migrateOut, mErr := runCmd(appDir, migrateBin, "migrate", "down", strconv.Itoa(downCount))
		if mErr != nil {
			fail("migrate down", fmt.Errorf("%v: %s", mErr, migrateOut))
			return
		}
	} else if len(forwardSet) > 0 {
		// Forward revert: the target's schema is AHEAD of live (a later build
		// downgraded this package, tearing the target's migrations off disk). The
		// on-disk server/pb_migrations dir reflects that downgraded state, so a bare
		// forward `migrate` would scan it, see no new files, and report "no new
		// migrations" — the target's collections would never be recreated. Restore
		// the target build's archived migration files into server/pb_migrations as
		// real files first, so the swapped-in binary's jsvm re-scan finds them as
		// pending (they were torn down, so they're absent from the _migrations
		// history) and applies them forward. The files are non-canonical (the next
		// generator regen rewrites the dir from member sources), but they are exactly
		// what `migrate` needs to reproduce THIS build's schema right now.
		restored, restoreErr := restoreArchivedMigrations(appDir, arch, forwardSet)
		if restoreErr != nil {
			fail("restore migrations", restoreErr)
			return
		}
		rollbackStack = append(rollbackStack, func() {
			for _, p := range restored {
				os.Remove(p)
			}
		})
		emitProgress(job, "Restoring migrations", 55,
			fmt.Sprintf("Re-applying %d migration(s)", len(forwardSet)))
		migrateOut, mErr := runCmd(appDir, migrateBin, "migrate")
		if mErr != nil {
			fail("migrate up", fmt.Errorf("%v: %s", mErr, migrateOut))
			return
		}
	} else {
		emitProgress(job, "Reversing migrations", 55, "No schema changes to reverse")
	}

	// The `migrate down` subprocess (and the VACUUM INTO backup in Step 3) opened
	// pb_data/data.db as a separate process and reset its WAL, leaving the live
	// server's mmap'd WAL index stale — so the Step 7 app.RunInTransaction write
	// would fail with "database disk image is malformed (11)" on a bind-mounted
	// pb_data. Reconnect the live DB pools first. See recoverLiveDBAfterExternalWrite.
	if rErr := recoverLiveDBAfterExternalWrite(app); rErr != nil {
		fail("db reconnect", rErr)
		return
	}

	// Step 6: Re-stage the archived web bundle (70%). Copy it into
	// release-staging/<release_id> so the entrypoint's promote_release picks it
	// up on the post-restart boot.
	emitProgress(job, "Staging release", 70, "Restoring archived web bundle")
	releaseID := target.GetString("release_id")
	if releaseID == "" {
		releaseID = "revert-" + job.BuildID
	}
	stageDest := filepath.Join(appDir, "release-staging", releaseID)
	os.RemoveAll(stageDest)
	if err := copyDir(arch.release, stageDest); err != nil {
		fail("stage release", err)
		return
	}
	rollbackStack = append(rollbackStack, func() {
		os.RemoveAll(stageDest)
	})

	// Step 7: Update build + registry records (85%). Mark the target current,
	// supersede every newer build, reconcile the package registry to the reverted
	// state, and point the registry at the target's package version — all in one
	// transaction so an interrupted run can't leave the build set with zero or
	// multiple `current` rows.
	emitProgress(job, "Updating records", 85, "Recording revert")
	updateErr := app.RunInTransaction(func(txApp core.App) error {
		for _, b := range newer {
			b.Set("status", "superseded")
			if err := txApp.Save(b); err != nil {
				return fmt.Errorf("supersede build %s: %w", b.GetString("build_id"), err)
			}
		}
		target.Set("status", "current")
		if err := txApp.Save(target); err != nil {
			return err
		}
		if err := disableRevertedPackages(txApp, target, newer); err != nil {
			return err
		}
		return syncRegistryToBuild(txApp, target)
	})
	if updateErr != nil {
		fail("update records", updateErr)
		return
	}
	// The pkg_install_log row (action=revert, target slug, timestamps) is the
	// history trail; no separate pkg_build marker is written, so the build list
	// stays a clean one-record-per-installed-state view.

	emitProgress(job, "Requesting restart", 95, "Signaling server restart")
	job.Status = "success"
	finalizeInstallLog(app, logRecord, "success", "", job.LogLines)
	emitComplete(job, "success", "")

	time.Sleep(2 * time.Second)
	requestRestart(appDir)
}

// restoreArchivedMigrations copies the named migration files from the target
// build's archive (arch.migrations, populated by archiveBuild) into the live
// server/pb_migrations dir as real files, so the swapped-in binary's `migrate`
// scan finds them and applies them forward. Returns the absolute paths it wrote
// (for the rollback stack to remove on failure). A file absent from the archive
// (an older build archived before migration-file capture, or a defensive skip at
// archive time) is reported as an error: without it the forward apply can't
// recreate the target schema, so failing here is the safe choice — the rollback
// then restores the pre-revert DB + binary.
func restoreArchivedMigrations(appDir string, arch buildArchive, names []string) ([]string, error) {
	dstDir := filepath.Join(appDir, "server", "pb_migrations")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir pb_migrations: %w", err)
	}
	written := make([]string, 0, len(names))
	for _, name := range names {
		src := filepath.Join(arch.migrations, name)
		if _, err := os.Stat(src); err != nil {
			return written, fmt.Errorf("archived migration %s missing; cannot recreate target schema", name)
		}
		dst := filepath.Join(dstDir, name)
		// The existing entry (if any) is a generator symlink pointing at the
		// downgraded member's files; replace it with the archived real file so jsvm
		// applies the target's version.
		os.Remove(dst)
		if _, err := runCmd(".", "cp", "-aL", src, dst); err != nil {
			return written, fmt.Errorf("restore migration %s: %w", name, err)
		}
		written = append(written, dst)
	}
	return written, nil
}

// plannedMigrationDown sums the migration counts of the newer builds and returns
// the expected _migrations tail (the filenames that must currently sit on top of
// the history, newest first) so the gate can confirm the chain is intact.
func plannedMigrationDown(newer []*core.Record) (downCount int, expectedTail []string) {
	for _, b := range newer {
		downCount += int(b.GetInt("migrations_applied"))
		var files []string
		b.UnmarshalJSONField("migration_files", &files)
		expectedTail = append(expectedTail, files...)
	}
	return downCount, expectedTail
}

// verifyMigrationTail confirms the live _migrations history begins (newest first)
// with exactly the expected filenames. A mismatch means the history was changed
// outside our control (manual edits, history-sync, an out-of-band install), so a
// blind `migrate down N` could reverse the wrong migrations — block instead.
func verifyMigrationTail(app core.App, expected []string) error {
	applied, err := appliedMigrationFiles(app)
	if err != nil {
		return err
	}
	return tailMatches(applied, expected)
}

// tailMatches reports whether the newest-first applied history begins with
// exactly the expected filenames (the pure core of verifyMigrationTail).
func tailMatches(applied, expected []string) error {
	if len(expected) == 0 {
		return nil
	}
	if len(applied) < len(expected) {
		return fmt.Errorf("migration history shorter than expected (%d < %d); cannot safely revert",
			len(applied), len(expected))
	}
	for i, f := range expected {
		if applied[i] != f {
			return fmt.Errorf("migration history diverged at %q (expected %q); "+
				"the schema was changed outside the installer — resolve manually before reverting",
				applied[i], f)
		}
	}
	return nil
}

// disableRevertedPackages marks the pkg_registry rows for packages whose install
// was reverted past as `disabled`, so the package list reflects the reverted
// state (their collections were just torn down by `migrate down` and the
// reverted binary no longer registers them). A slug is disabled only if it was
// introduced by one of the now-superseded builds AND no surviving build (the
// target or anything older still `current`/`available`) re-introduces it — that
// guard keeps a package installed if an earlier build of it survives the revert.
// Bundled packages are never disabled (they ship in the binary regardless).
func disableRevertedPackages(txApp core.App, target *core.Record, superseded []*core.Record) error {
	// Slugs that survive the revert: any non-superseded build at or before the
	// target. (After this revert the surviving builds are exactly target +
	// everything already available/superseded-from-a-prior-revert; we query for
	// the ones that still represent an installed state.)
	survivors, err := txApp.FindRecordsByFilter(
		"pkg_build",
		"created <= {:ts} && status != 'superseded'",
		"",
		0,
		0,
		dbx.Params{"ts": target.GetString("created")},
	)
	if err != nil {
		return err
	}
	surviving := make(map[string]bool, len(survivors))
	for _, b := range survivors {
		surviving[b.GetString("pkg_slug")] = true
	}

	seen := make(map[string]bool)
	for _, b := range superseded {
		slug := b.GetString("pkg_slug")
		// Skip the synthetic base-image slug and anything an earlier surviving
		// build still keeps installed; dedupe so we touch each slug once.
		if slug == "" || slug == baseImageSlug || surviving[slug] || seen[slug] {
			continue
		}
		seen[slug] = true
		reg, findErr := txApp.FindFirstRecordByFilter(
			"pkg_registry",
			"slug = {:slug}",
			dbx.Params{"slug": slug},
		)
		if findErr != nil {
			continue // no registry row (e.g. predates registry) — nothing to disable
		}
		if reg.GetString("status") == "bundled" {
			continue
		}
		reg.Set("status", "disabled")
		if saveErr := txApp.Save(reg); saveErr != nil {
			return fmt.Errorf("disable package %s: %w", slug, saveErr)
		}
	}
	return nil
}

// syncRegistryToBuild points the pkg_registry record for the build's package at
// the build's archived version, so the package list reflects the reverted state.
func syncRegistryToBuild(app core.App, build *core.Record) error {
	record, err := app.FindFirstRecordByFilter(
		"pkg_registry",
		"slug = {:slug}",
		map[string]any{"slug": build.GetString("pkg_slug")},
	)
	if err != nil {
		// No registry row (e.g. build predates registry); nothing to sync.
		return nil
	}
	record.Set("version", build.GetString("version"))
	record.Set("npm_package", build.GetString("npm_package"))
	return app.Save(record)
}
