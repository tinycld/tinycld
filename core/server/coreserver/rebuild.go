package coreserver

import (
	"log"
	"os"
	"path/filepath"

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
	restart        func()
}

// rebuildWith runs the full rebuild: assemble → build → backup → migrate →
// activate → commit registry → prune → restart. UP migrations are NOT applied
// here; the freshly-built binary applies them on its post-swap boot. On any
// failure before activation the build dir is discarded and (if the DB was
// already backed up) the DB is restored, leaving the live `current` unchanged.
func rebuildWith(job *installJob, m RebuildManifest, d rebuildDeps) error {
	buildDir := filepath.Join(stateBuildsDir(), m.BuildID)

	if err := d.assemble(m, buildDir); err != nil {
		return failJob(job, "assemble", err)
	}
	if err := d.pipeline(job, buildDir); err != nil {
		// Failure precedes the DB backup; live state untouched. Discard build.
		_ = os.RemoveAll(buildDir)
		return failJob(job, "build", err)
	}
	// From here the DB may change — back it up so we can roll back.
	if err := d.backupDB(); err != nil {
		_ = os.RemoveAll(buildDir)
		return failJob(job, "backup", err)
	}
	if _, err := d.syncMig(buildDir); err != nil {
		restore(d)
		_ = os.RemoveAll(buildDir)
		return failJob(job, "migrate", err)
	}
	if err := d.activate(m.BuildID); err != nil {
		restore(d)
		return failJob(job, "activate", err)
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
func rebuild(app *pocketbase.PocketBase, job *installJob, m RebuildManifest) error {
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
		restart:        func() { requestRestart("") },
	}
	return rebuildWith(job, m, deps)
}

// commitRegistry mirrors the just-activated manifest into pkg_registry so the
// admin inventory reflects the live build. Defined fully in Task 16; the
// no-op default keeps the production wiring compiling until then.
func commitRegistry(app core.App, m RebuildManifest) error {
	_ = app
	_ = m
	return nil
}
