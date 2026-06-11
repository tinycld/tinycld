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

// rebuildDeps holds the orchestrator's steps as injectable functions so the
// control flow (ordering, failure handling, rollback) is unit-testable without
// running a real build.
type rebuildDeps struct {
	assemble func(m RebuildManifest, buildDir string) error
	pipeline func(job *installJob, buildDir string) (buildOutput, error)
	backupDB func() error
	restoreDB func() error
	syncMig  func(buildDir string) (SyncResult, error)
	activate func(buildID string) error
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

	if err := d.assemble(m, buildDir); err != nil {
		return d.fail(job, "assemble", err)
	}
	out, err := d.pipeline(job, buildDir)
	if err != nil {
		// Failure precedes the DB backup; live state untouched. Discard build.
		_ = os.RemoveAll(buildDir)
		return d.fail(job, "build", err)
	}
	// From here the DB may change — back it up so we can roll back.
	if err := d.backupDB(); err != nil {
		_ = os.RemoveAll(buildDir)
		return d.fail(job, "backup", err)
	}
	if _, err := d.syncMig(buildDir); err != nil {
		restore(d)
		_ = os.RemoveAll(buildDir)
		return d.fail(job, "migrate", err)
	}
	if err := d.activate(m.BuildID); err != nil {
		restore(d)
		return d.fail(job, "activate", err)
	}
	// backupDatabase (VACUUM INTO) and the migration sync access the SQLite file
	// out-of-band, which leaves the live app's connection pools pointed at a stale
	// view (observed: recordBuild/commitRegistry hit "no such table: pkg_registry"
	// against a detached DB). Re-bootstrap the pools so the post-activate registry
	// + build-record writes land in the real DB. (The old in-place pipeline did the
	// same via recoverLiveDBAfterExternalWrite after its migrate subprocess.)
	if d.recoverDB != nil {
		if err := d.recoverDB(); err != nil {
			log.Printf("rebuild: recoverDB warning: %v", err)
		}
	}
	if d.recordBuild != nil {
		if err := d.recordBuild(out); err != nil {
			// The build is already live; failing to record the pkg_build row only
			// affects OTA-update advertisement + the rollback UI, not correctness.
			// Log and continue rather than abort an already-activated build.
			log.Printf("rebuild: recordBuild warning: %v", err)
		}
	}
	if d.commitRegistry != nil {
		if err := d.commitRegistry(); err != nil {
			// The build is already live; a registry mirror failure is logged but
			// not fatal — the next boot reconciles from the live tree. Don't
			// restore the DB (the new schema is correct) or unflip the symlink.
			log.Printf("rebuild: commitRegistry warning: %v", err)
		}
	}
	if d.prune != nil {
		if err := d.prune(buildsToKeep); err != nil {
			log.Printf("rebuild: prune warning: %v", err)
		}
	}
	job.Status = "success"
	// Finalize the install log BEFORE restart — restart os.Exit's the process.
	if d.finalizeLog != nil {
		d.finalizeLog("success", "")
	}
	emitProgress(job, "Restarting", 99, "Activating new build")
	emitComplete(job, "success", "")
	d.restart()
	return nil
}

func restore(d rebuildDeps) {
	if d.restoreDB != nil {
		if err := d.restoreDB(); err != nil {
			log.Printf("rebuild: DB restore failed: %v", err)
		}
	}
}

func failJob(job *installJob, step string, err error) error {
	job.Status = "failed"
	job.Error = err.Error()
	emitProgress(job, step, job.Progress, "FAILED: "+err.Error())
	emitComplete(job, "failed", job.Error)
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
		assemble: assembleBuild,
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
			v := manifestVersionFromBuild(buildDir, ms.Slug)
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
			if v := manifestVersionFromBuild(buildDir, ms.Slug); v != "" {
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
