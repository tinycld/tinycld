package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// MemberSpec describes one workspace member to assemble into a build.
// Spec is the fetch spec passed to `npm pack` — an npm name+range
// (@tinycld/mail@0.3.1), a git URL (git+https://…#tag), or a local
// git+file:// remote (used by the integration test).
//
// FromCurrent marks a member that should be COPIED from the currently-active
// build rather than re-fetched. Only the member(s) a delta actually changes are
// fetched fresh; everything else is copied from the live build so an install of
// one package can't silently re-resolve another member's spec to a drifted
// remote state (e.g. re-fetching the tinycld base from github HEAD, which may be
// behind the running base — and would drop migrations the running base ships).
type MemberSpec struct {
	Slug        string `json:"slug"`
	Version     string `json:"version"`
	Spec        string `json:"spec"`
	FromCurrent bool   `json:"fromCurrent,omitempty"`
}

// RebuildManifest is the complete desired package set for one build. It is
// written verbatim to builds/<id>/manifest.json before the build runs and
// serves as the build's input AND its rollback record.
type RebuildManifest struct {
	BuildID string       `json:"buildId"`
	Members []MemberSpec `json:"members"`
}

// MemberBySlug returns the member spec for slug, if present.
func (m RebuildManifest) MemberBySlug(slug string) (MemberSpec, bool) {
	for _, ms := range m.Members {
		if ms.Slug == slug {
			return ms, true
		}
	}
	return MemberSpec{}, false
}

// buildsToKeep is how many recent build dirs the prune step retains (beyond
// the current one). Each is mostly hardlinks into the shared pnpm store, so
// the real disk cost per retained build is small.
const buildsToKeep = 5

// Progress milestones for the /admin progress bar, in the order a rebuild hits
// them. The assemble band (members fetched/copied) runs from progAssembleStart
// to progAssembleEnd with per-member ticks spread across it; the build pipeline
// (pnpm/go/expo/native) owns the middle; the DB + activation phases own the tail.
// Keeping the whole scale here (rather than scattered magic numbers) makes the
// bar monotonic across all four rebuild paths.
const (
	progAssembleStart = 5
	progAssembleEnd   = 42
	progPnpmInstall   = 45
	progGoBuild       = 60
	progExpoWeb       = 72
	progStageRelease  = 76
	progNativeStart   = 80
	progNativeEnd     = 88
	progBackupDB      = 90
	progSyncMig       = 93
	progActivate      = 96
	progCommit        = 98
	progRestart       = 99
)

// rebuildDeps holds the orchestrator's steps as injectable functions so the
// control flow (ordering, failure handling, rollback) is unit-testable without
// running a real build.
type rebuildDeps struct {
	assemble  func(m RebuildManifest, buildDir string) error
	pipeline  func(job *installJob, buildDir string) (buildOutput, error)
	backupDB  func() error
	restoreDB func() error
	syncMig   func(buildDir string) (SyncResult, error)
	activate  func(buildID string) error
	// recoverDB re-bootstraps the live app's DB pools after the out-of-band DB
	// access of backupDB + syncMig, so the post-activate registry/build-record
	// writes see the real tables. Optional (nil-safe).
	recoverDB func() error
	// recordBuild persists the pkg_build row (release id + native OTA bundles)
	// the /api/app/update endpoint and the revert/rollback UI read. Runs after a
	// successful build, before restart. Optional (nil-safe).
	recordBuild    func(out buildOutput) error
	commitRegistry func() error
	prune          func(keep int) error
	// finalizeLog records the terminal state of the pkg_install_log row the UI
	// polls (status endpoint). It MUST run before restart() — restart os.Exit's
	// the process, so a deferred finalize would never fire. Optional (nil-safe).
	finalizeLog func(status, errMsg string)
	restart     func()
}

// fail finalizes the install log (if any) and marks the job failed. Used at
// every pre-activation failure exit so the status endpoint reports the failure
// instead of hanging the UI poller.
func (d rebuildDeps) fail(job *installJob, step string, err error) error {
	if d.finalizeLog != nil {
		d.finalizeLog("failed", err.Error())
	}
	return failJob(job, step, err)
}

// rebuildWith runs the full rebuild: assemble → build → backup → migrate →
// activate → commit registry → prune → restart. UP migrations are NOT applied
// here; the freshly-built binary applies them on its post-swap boot. On any
// failure before activation the build dir is discarded and (if the DB was
// already backed up) the DB is restored, leaving the live `current` unchanged.
func rebuildWith(job *installJob, m RebuildManifest, d rebuildDeps) error {
	buildDir := filepath.Join(stateBuildsDir(), m.BuildID)
	rebuildStart := monoNow()

	// Document exactly what this build is assembled from — the most useful single
	// line for reproducing / post-morteming a build later.
	jobLogf(job, "rebuild starting: %s", memberSetSummary(m))
	jobLogf(job, "build dir: %s  (state root: %s)", buildDir, resolveStateDir())

	emitProgress(job, "Assembling build", progAssembleStart, memberSetSummary(m))
	if err := timeStep(job, "assemble", func() error { return d.assemble(m, buildDir) }); err != nil {
		return d.fail(job, "assemble", err)
	}
	out, err := d.pipeline(job, buildDir)
	if err != nil {
		// Failure precedes the DB backup; live state untouched. Discard build.
		jobLogf(job, "build failed — discarding build dir %s (live state untouched)", buildDir)
		_ = os.RemoveAll(buildDir)
		return d.fail(job, "build", err)
	}
	// From here the DB may change — back it up so we can roll back.
	emitProgress(job, "Backing up database", progBackupDB, "Creating SQLite backup")
	if err := timeStep(job, "backup database", d.backupDB); err != nil {
		_ = os.RemoveAll(buildDir)
		return d.fail(job, "backup", err)
	}
	emitProgress(job, "Applying migrations", progSyncMig, "Reconciling schema to new build")
	if err := timeStep(job, "sync migrations", func() error {
		res, mErr := d.syncMig(buildDir)
		if mErr == nil {
			logSyncResult(job, res)
		}
		return mErr
	}); err != nil {
		jobLogf(job, "migration sync failed — restoring DB backup + discarding build")
		restore(d)
		_ = os.RemoveAll(buildDir)
		return d.fail(job, "migrate", err)
	}
	emitProgress(job, "Activating build", progActivate, "Flipping current symlink")
	if err := timeStep(job, "activate build", func() error { return d.activate(m.BuildID) }); err != nil {
		jobLogf(job, "activate failed — restoring DB backup")
		restore(d)
		return d.fail(job, "activate", err)
	}
	jobLogf(job, "current symlink now points at build %s", m.BuildID)
	// backupDatabase (VACUUM INTO) and the migration sync access the SQLite file
	// out-of-band, which leaves the live app's connection pools pointed at a stale
	// view (observed: recordBuild/commitRegistry hit "no such table: pkg_registry"
	// against a detached DB). Re-bootstrap the pools so the post-activate registry
	// + build-record writes land in the real DB. (The old in-place pipeline did the
	// same via recoverLiveDBAfterExternalWrite after its migrate subprocess.)
	if d.recoverDB != nil {
		if err := timeStep(job, "reconnect DB pools", d.recoverDB); err != nil {
			jobLogf(job, "WARNING: recoverDB failed (post-activate writes may not land): %v", err)
		}
	}
	if d.recordBuild != nil {
		if err := d.recordBuild(out); err != nil {
			// The build is already live; failing to record the pkg_build row only
			// affects OTA-update advertisement + the rollback UI, not correctness.
			// Log and continue rather than abort an already-activated build.
			jobLogf(job, "WARNING: recordBuild failed (OTA + rollback-target metadata missing): %v", err)
		} else {
			jobLogf(job, "recorded pkg_build %s (release %s, %d native bundle(s))",
				m.BuildID, out.releaseID, len(out.bundles))
		}
	}
	emitProgress(job, "Finalizing", progCommit, "Recording build + registry")
	if d.commitRegistry != nil {
		if err := d.commitRegistry(); err != nil {
			// The build is already live; a registry mirror failure is logged but
			// not fatal — the next boot reconciles from the live tree. Don't
			// restore the DB (the new schema is correct) or unflip the symlink.
			jobLogf(job, "WARNING: commitRegistry failed (admin inventory may lag; reconciles on next boot): %v", err)
		} else {
			jobLogf(job, "registry mirrored to the live build")
		}
	}
	if d.prune != nil {
		if err := d.prune(buildsToKeep); err != nil {
			jobLogf(job, "WARNING: build prune failed (old builds retained): %v", err)
		}
	}
	job.Status = "success"
	jobLogf(job, "rebuild succeeded in %s — restarting onto build %s", monoSince(rebuildStart), m.BuildID)
	// Finalize the install log BEFORE restart — restart os.Exit's the process.
	if d.finalizeLog != nil {
		d.finalizeLog("success", "")
	}
	emitProgress(job, "Restarting", progRestart, "Activating new build")
	emitComplete(job, "success", "")
	d.restart()
	return nil
}

func restore(d rebuildDeps) {
	if d.restoreDB != nil {
		if err := d.restoreDB(); err != nil {
			log.Printf("rebuild: DB restore failed: %v", err)
			return
		}
		// The restore overwrote data.db (and cleared its WAL) underneath the still-
		// running live app, whose connection pool holds a now-stale mmap of the old
		// WAL index — its next write would fail "disk image is malformed". Re-open
		// the pools so the live process (which keeps serving after a pre-activation
		// failure) sees the restored DB cleanly. Post-activation failures restore in
		// the entrypoint instead (different process), so this only matters here.
		if d.recoverDB != nil {
			if err := d.recoverDB(); err != nil {
				log.Printf("rebuild: DB reconnect after restore failed: %v", err)
			}
		}
	}
}

func failJob(job *installJob, step string, err error) error {
	job.Status = "failed"
	job.Error = err.Error()
	emitProgress(job, step, job.Progress, "FAILED: "+err.Error())
	emitComplete(job, "failed", job.Error)
	// Surface the failure to Sentry (background jobs bypass the request-scoped
	// middleware). No-op when Sentry isn't configured.
	captureRebuildFailure(job, job.Action, step, err)
	return err
}

// rebuild wires the production dependencies and runs the full rebuild for the
// given desired manifest. It is the single entry point every mutating package
// operation (install / uninstall / version change / core upgrade) funnels into.
// logRecord is the pkg_install_log row the status endpoint polls; it is
// finalized (success/failed) before the restart so the UI poller terminates.
func rebuild(app *pocketbase.PocketBase, job *installJob, m RebuildManifest, logRecord *core.Record) error {
	buildDir := filepath.Join(stateBuildsDir(), m.BuildID)

	// backupDatabase returns the restore closure; capture it across steps.
	var restoreClosure func() error

	deps := rebuildDeps{
		assemble: func(mm RebuildManifest, bd string) error { return assembleBuild(job, mm, bd) },
		pipeline: func(j *installJob, bd string) (buildOutput, error) {
			return runBuildPipeline(j, bd, m.BuildID)
		},
		backupDB: func() error {
			r, e := backupDatabase(filepath.Join(buildDir, "tinycld"))
			restoreClosure = r
			return e
		},
		restoreDB: func() error {
			if restoreClosure != nil {
				return restoreClosure()
			}
			return nil
		},
		syncMig: func(bd string) (SyncResult, error) {
			newSet, err := buildMigrationFiles(bd)
			if err != nil {
				return SyncResult{}, err
			}
			applied, err := appliedMigrationFiles(app)
			if err != nil {
				return SyncResult{}, err
			}
			return syncMigrations(app, applied, newSet)
		},
		activate:  activateBuild,
		recoverDB: func() error { return recoverLiveDBAfterExternalWrite(app) },
		recordBuild: func(out buildOutput) error {
			return recordRebuildBuild(app, m, buildDir, out)
		},
		commitRegistry: func() error { return commitRegistry(app, m, buildDir) },
		prune:          pruneBuilds,
		finalizeLog: func(status, errMsg string) {
			finalizeInstallLog(app, logRecord, status, errMsg, job.LogLines)
		},
		restart: func() {
			// Arm the surviving data.db.backup as a rollback snapshot BEFORE the
			// restart. This is the post-activation success path: DOWN migrations
			// already ran against the live DB and the symlink already flipped, so if
			// the new binary fails its health probe the entrypoint must restore the
			// DB (not just the symlink). Arming leaves the backup file in place +
			// drops a marker the entrypoint commits (deletes) on a healthy boot.
			armDatabaseBackup(m.BuildID)
			// Flush all pre-restart writes (install-log finalize, registry mirror)
			// from the WAL into data.db before the hard os.Exit, or the new binary
			// reads a data.db missing them.
			checkpointWAL(app)
			requestRestart("")
		},
	}
	return rebuildWith(job, m, deps)
}

// recordRebuildBuild persists the pkg_build row for a freshly-activated build:
// its release id and the native OTA bundle metadata /api/app/update serves, plus
// enough identity for the rollback UI to offer this build as a revert target.
// recordBuild() demotes the prior `current` row to `available` in the same
// transaction. binary_archived is always true — every rebuild compiles a server
// binary that travels with its build dir.
func recordRebuildBuild(app core.App, m RebuildManifest, buildDir string, out buildOutput) error {
	// The build is labeled by the member the operation CHANGED — the one member
	// that isn't FromCurrent. The rollback UI finds revert targets by
	// (pkg_slug, version), so this must be the changed package (e.g. "todo"), not
	// an arbitrary unchanged member. version comes from the built manifest (semver),
	// not the delta's git tag.
	slug, version := changedMember(m, buildDir)
	fields := map[string]any{
		"build_id":        m.BuildID,
		"pkg_slug":        slug,
		"version":         version,
		"action":          "install",
		"binary_archived": true,
		"release_id":      out.releaseID,
		"bundles":         serializeBundles(out.bundles),
	}
	_, err := recordBuild(app, fields)
	return err
}

// changedMember returns the registry slug + semver of the member the rebuild
// changed (the single non-FromCurrent member). Falls back to the base when every
// member is FromCurrent (a pure rebuild). The version is read from the built
// manifest so it's the package semver, not a git-tag delta string.
func changedMember(m RebuildManifest, buildDir string) (slug, version string) {
	for _, ms := range m.Members {
		if !ms.FromCurrent {
			s := memberSlugToRegistry(ms.Slug)
			v := changedMemberVersion(buildDir, s, ms.Slug)
			if v == "" {
				v = ms.Version
			}
			return s, v
		}
	}
	// Every member unchanged — label by the base.
	if ms, ok := m.MemberBySlug(baseMemberSlug); ok {
		return baseRegistrySlug, ms.Version
	}
	return baseRegistrySlug, ""
}

// The base platform is the `tinycld` workspace member, but its pkg_registry row
// (and the /admin UI delta) uses the historical slug "core". These map between
// the two namespaces so the desired set always speaks member slugs while the
// registry keeps its slug.
const baseRegistrySlug = "core"
const baseMemberSlug = "tinycld"

func registrySlugToMember(slug string) string {
	if slug == baseRegistrySlug {
		return baseMemberSlug
	}
	return slug
}

func memberSlugToRegistry(slug string) string {
	if slug == baseMemberSlug {
		return baseRegistrySlug
	}
	return slug
}

// buildCurrentMemberSet reads installed/bundled pkg_registry rows into the
// member set the current live build represents. The base row (slug "core") maps
// to the tinycld member. Always includes tinycld.
func buildCurrentMemberSet(app core.App) ([]MemberSpec, error) {
	recs, err := app.FindAllRecords("pkg_registry",
		dbx.In("status", "installed", "bundled"))
	if err != nil {
		return nil, err
	}
	out := make([]MemberSpec, 0, len(recs))
	for _, r := range recs {
		out = append(out, MemberSpec{
			Slug:    registrySlugToMember(r.GetString("slug")),
			Version: r.GetString("version"),
			Spec:    r.GetString("npm_package"),
		})
	}
	return out, nil
}

// commitRegistry mirrors the just-activated manifest into pkg_registry so the
// admin inventory reflects the live build:
//   - existing rows for present members: version/spec updated, status set to
//     installed (bundled rows keep "bundled");
//   - present members with NO row yet (a fresh install): a full row is created
//     from the member's manifest parsed out of the build dir;
//   - rows absent from the manifest (an uninstall): marked disabled.
//
// buildDir is the active build's root, used to parse newly-installed members'
// manifests for the create path.
func commitRegistry(app core.App, m RebuildManifest, buildDir string) error {
	present := map[string]MemberSpec{}
	for _, ms := range m.Members {
		present[memberSlugToRegistry(ms.Slug)] = ms
	}
	recs, err := app.FindAllRecords("pkg_registry")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range recs {
		slug := r.GetString("slug")
		ms, ok := present[slug]
		if !ok {
			// No longer in the desired set — uninstalled.
			if r.GetString("status") != "bundled" && r.GetString("status") != "disabled" {
				r.Set("status", "disabled")
				if err := app.Save(r); err != nil {
					return err
				}
				log.Printf("[pkg_install] registry: %s -> disabled (uninstalled)", slug)
			}
			continue
		}
		seen[slug] = true
		changed := false
		// For a CHANGED member (fetched fresh), the authoritative version is the
		// built package's manifest version (semver, e.g. "2.0.0"), NOT the delta's
		// target string which may be a git TAG ("v2.0.0"). Parse it from the build
		// so the registry stores what the rest of the system compares against.
		// FromCurrent members are unchanged — keep their existing version.
		version := ms.Version
		if !ms.FromCurrent {
			if v := changedMemberVersion(buildDir, slug, ms.Slug); v != "" {
				version = v
			}
		}
		if version != "" && r.GetString("version") != version {
			r.Set("version", version)
			changed = true
		}
		if ms.Spec != "" && r.GetString("npm_package") != ms.Spec {
			r.Set("npm_package", ms.Spec)
			changed = true
		}
		if r.GetString("status") != "bundled" && r.GetString("status") != "installed" {
			r.Set("status", "installed")
			changed = true
		}
		if changed {
			if err := app.Save(r); err != nil {
				return err
			}
			log.Printf("[pkg_install] registry: %s -> version=%s status=%s",
				slug, r.GetString("version"), r.GetString("status"))
		}
	}
	// Create rows for freshly-installed members (no existing row). Parse each
	// one's manifest from the build dir for the full registry fields.
	for regSlug, ms := range present {
		if seen[regSlug] {
			continue
		}
		if err := createRegistryRowFromBuild(app, buildDir, ms); err != nil {
			return fmt.Errorf("create registry row for %s: %w", ms.Slug, err)
		}
		log.Printf("[pkg_install] registry: created %s (newly installed)", regSlug)
	}
	return nil
}

// manifestVersionFromBuild returns the member's semver version parsed from its
// manifest in the build dir, or "" if it can't be read. Used to store the real
// package version in the registry instead of a delta's git-tag string.
func manifestVersionFromBuild(buildDir, slug string) string {
	manifest, err := parseManifestViaNode(filepath.Join(buildDir, slug))
	if err != nil {
		return ""
	}
	return manifest.Version
}

// changedMemberVersion returns the semver to store in the registry row `regSlug`
// for the just-built member `memberSlug`. The base is special: its registry slug
// is "core" but it ships as the `tinycld` member, and the version users track is
// CORE's (the nested tinycld/core/package.json), NOT the app shell's
// tinycld/package.json. Everything else reads its own member manifest.
func changedMemberVersion(buildDir, regSlug, memberSlug string) string {
	if regSlug == baseRegistrySlug {
		if v := packageJSONVersion(filepath.Join(buildDir, baseMemberSlug, "core", "package.json")); v != "" {
			return v
		}
	}
	return manifestVersionFromBuild(buildDir, memberSlug)
}

// packageJSONVersion reads the "version" field from a package.json, or "" on any
// error. A small, dependency-free read (no node subprocess).
func packageJSONVersion(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pkg struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ""
	}
	return pkg.Version
}

// createRegistryRowFromBuild parses the member's manifest out of the build dir
// and inserts a full pkg_registry row via upsertPkgRegistry (which handles the
// create path). The member source lives at <buildDir>/<memberSlug>.
func createRegistryRowFromBuild(app core.App, buildDir string, ms MemberSpec) error {
	pkgDir := filepath.Join(buildDir, ms.Slug)
	manifest, err := parseManifestViaNode(pkgDir)
	if err != nil {
		return err
	}
	mj, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return upsertPkgRegistry(app, manifest, ms.Spec, mj)
}

// setDelta is the single mutation applied to the current member set.
// op ∈ {"install","version","uninstall"}. For uninstall only slug matters.
type setDelta struct {
	op      string
	slug    string
	version string
	spec    string
}

// desiredSet computes the target member set: the current set with delta applied.
// Install/version replaces (or appends) the slug's spec+version; uninstall drops
// it. Every other member is carried through unchanged.
func desiredSet(buildID string, current []MemberSpec, d setDelta) RebuildManifest {
	out := make([]MemberSpec, 0, len(current)+1)
	replaced := false
	for _, ms := range current {
		if ms.Slug == d.slug {
			switch d.op {
			case "uninstall":
				continue // drop it
			case "install", "version":
				// The changed member is fetched fresh (FromCurrent stays false).
				out = append(out, MemberSpec{Slug: d.slug, Version: d.version, Spec: d.spec})
				replaced = true
				continue
			}
		}
		// Unchanged members are copied from the currently-active build, NOT
		// re-fetched — re-resolving their spec could drift them (e.g. the tinycld
		// base from github HEAD) below the running version.
		ms.FromCurrent = true
		out = append(out, ms)
	}
	if !replaced && (d.op == "install" || d.op == "version") {
		out = append(out, MemberSpec{Slug: d.slug, Version: d.version, Spec: d.spec})
	}
	return RebuildManifest{BuildID: buildID, Members: out}
}
