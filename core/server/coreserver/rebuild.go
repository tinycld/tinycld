package coreserver

import (
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
// git+file:// remote (used by the integration test). Every member,
// including the tinycld app shell + core, is fetched this way.
type MemberSpec struct {
	Slug    string `json:"slug"`
	Version string `json:"version"`
	Spec    string `json:"spec"`
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
	assemble       func(m RebuildManifest, buildDir string) error
	pipeline       func(job *installJob, buildDir string) error
	backupDB       func() error
	restoreDB      func() error
	syncMig        func(buildDir string) (SyncResult, error)
	activate       func(buildID string) error
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
	if err := d.pipeline(job, buildDir); err != nil {
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
		pipeline: runBuildPipeline,
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
		activate:       activateBuild,
		commitRegistry: func() error { return commitRegistry(app, m) },
		prune:          pruneBuilds,
		finalizeLog: func(status, errMsg string) {
			finalizeInstallLog(app, logRecord, status, errMsg, job.LogLines)
		},
		restart: func() { requestRestart("") },
	}
	return rebuildWith(job, m, deps)
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
// admin inventory reflects the live build: each present member's version is
// updated and its status set to installed (bundled rows keep "bundled"), and
// any registry row absent from the manifest is marked disabled.
func commitRegistry(app core.App, m RebuildManifest) error {
	present := map[string]MemberSpec{}
	for _, ms := range m.Members {
		present[memberSlugToRegistry(ms.Slug)] = ms
	}
	recs, err := app.FindAllRecords("pkg_registry")
	if err != nil {
		return err
	}
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
		changed := false
		if ms.Version != "" && r.GetString("version") != ms.Version {
			r.Set("version", ms.Version)
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
	return nil
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
				out = append(out, MemberSpec{Slug: d.slug, Version: d.version, Spec: d.spec})
				replaced = true
				continue
			}
		}
		out = append(out, ms)
	}
	if !replaced && (d.op == "install" || d.op == "version") {
		out = append(out, MemberSpec{Slug: d.slug, Version: d.version, Spec: d.spec})
	}
	return RebuildManifest{BuildID: buildID, Members: out}
}
