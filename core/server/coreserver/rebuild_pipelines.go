package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase"
)

// newBuildID returns a fresh, server-generated build id. Never client-settable.
func newBuildID() string {
	return fmt.Sprintf("build-%d", time.Now().UnixMilli())
}

// finishJob is the deferred cleanup every rebuild-based pipeline shares: clear
// the single-flight slot and close the job's Done channel so SSE listeners and
// the handler's caller unblock.
func finishJob(job *installJob) {
	installMu.Lock()
	currentJob = nil
	installMu.Unlock()
	close(job.Done)
}

// resolveInstallSlugVersion packs the install spec just far enough to read its
// manifest, returning the package slug + version. It mirrors the old install
// pipeline's validate→pack→parse→validate prologue but performs NO workspace
// mutation — the real fetch happens again inside the rebuild's assemble step.
func resolveInstallSlugVersion(app *pocketbase.PocketBase, job *installJob) (slug, version string, err error) {
	emitProgress(job, "Validating package", 5, "Checking "+job.NpmPkg)
	if err := validatePackageSpec(job.NpmPkg); err != nil {
		return "", "", err
	}
	tmp, err := os.MkdirTemp("", "tinycld-resolve-*")
	if err != nil {
		return "", "", err
	}
	defer os.RemoveAll(tmp)

	emitProgress(job, "Downloading package", 12, "npm pack "+job.NpmPkg)
	if _, err := runCmd(tmp, "npm", "pack", job.NpmPkg, "--pack-destination", tmp); err != nil {
		return "", "", fmt.Errorf("npm pack: %w", err)
	}
	tgz, _ := filepath.Glob(filepath.Join(tmp, "*.tgz"))
	if len(tgz) == 0 {
		return "", "", fmt.Errorf("no .tgz after npm pack")
	}
	if _, err := runCmd(tmp, "tar", "xzf", tgz[0], "-C", tmp); err != nil {
		return "", "", err
	}
	extractDir := filepath.Join(tmp, "package")
	manifest, err := parseManifestViaNode(extractDir)
	if err != nil {
		return "", "", fmt.Errorf("parse manifest: %w", err)
	}
	emitProgress(job, "Manifest parsed", 20, fmt.Sprintf("%s (%s)", manifest.Name, manifest.Slug))

	bundledSlugs := getBundledSlugs(app)
	hasGoPrereqs := checkGoBuildPrereqs() == nil
	if err := validateManifest(manifest, hasGoPrereqs, bundledSlugs); err != nil {
		return "", "", err
	}
	return manifest.Slug, manifest.Version, nil
}

// runInstallRebuild installs a package by computing the desired set (current +
// the new member) and triggering a full rebuild.
func runInstallRebuild(app *pocketbase.PocketBase, job *installJob) {
	defer finishJob(job)

	slug, version, err := resolveInstallSlugVersion(app, job)
	if err != nil {
		_ = failJob(job, "resolve", err)
		return
	}
	job.Slug = slug

	current, err := buildCurrentMemberSet(app)
	if err != nil {
		_ = failJob(job, "registry", err)
		return
	}
	m := desiredSet(newBuildID(), current, setDelta{
		op: "install", slug: slug, version: version, spec: job.NpmPkg,
	})
	if err := rebuild(app, job, m); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
	}
}

// runUninstallRebuild uninstalls a package by computing the desired set (current
// minus the member) and triggering a full rebuild.
func runUninstallRebuild(app *pocketbase.PocketBase, job *installJob) {
	defer finishJob(job)

	current, err := buildCurrentMemberSet(app)
	if err != nil {
		_ = failJob(job, "registry", err)
		return
	}
	member := registrySlugToMember(job.Slug)
	m := desiredSet(newBuildID(), current, setDelta{op: "uninstall", slug: member})
	if err := rebuild(app, job, m); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
	}
}

// loadBuildManifest reads builds/<id>/manifest.json for a retained build.
func loadBuildManifest(buildID string) (RebuildManifest, error) {
	path := filepath.Join(stateBuildsDir(), buildID, "manifest.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return RebuildManifest{}, err
	}
	var m RebuildManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return RebuildManifest{}, err
	}
	return m, nil
}

// runRevertRebuild reactivates a previously-built, still-retained build dir.
// No rebuild or re-fetch happens: the target tree already exists complete on
// disk. We back up the DB, sync the schema to the target build's migration set
// (reverting any migrations newer builds applied), flip the `current` symlink
// to the target, mirror its manifest into the registry, and restart.
func runRevertRebuild(app *pocketbase.PocketBase, job *installJob) {
	defer finishJob(job)

	targetID := job.BuildID
	emitProgress(job, "Validating build", 5, "Checking "+targetID)
	targetDir := filepath.Join(stateBuildsDir(), targetID, "tinycld")
	if _, err := os.Stat(targetDir); err != nil {
		_ = failJob(job, "validate", fmt.Errorf("build %s not retained on disk: %w", targetID, err))
		return
	}
	m, err := loadBuildManifest(targetID)
	if err != nil {
		_ = failJob(job, "validate", fmt.Errorf("read target manifest: %w", err))
		return
	}

	emitProgress(job, "Backing up database", 20, "Creating SQLite backup")
	restoreDB, err := backupDatabase(filepath.Join(stateBuildsDir(), targetID, "tinycld"))
	if err != nil {
		_ = failJob(job, "backup", err)
		return
	}

	emitProgress(job, "Reconciling schema", 40, "Syncing migrations to target build")
	newSet, err := buildMigrationFiles(filepath.Join(stateBuildsDir(), targetID))
	if err != nil {
		_ = restoreDB()
		_ = failJob(job, "migrate", err)
		return
	}
	applied, err := appliedMigrationFiles(app)
	if err != nil {
		_ = restoreDB()
		_ = failJob(job, "migrate", err)
		return
	}
	if _, err := syncMigrations(app, applied, newSet); err != nil {
		_ = restoreDB()
		_ = failJob(job, "migrate", err)
		return
	}

	emitProgress(job, "Activating build", 70, "Flipping current symlink")
	if err := activateBuild(targetID); err != nil {
		_ = restoreDB()
		_ = failJob(job, "activate", err)
		return
	}
	if err := commitRegistry(app, m); err != nil {
		log.Printf("revert: commitRegistry warning: %v", err)
	}
	emitProgress(job, "Restarting", 99, "Activating reverted build")
	emitComplete(job, "success", "")
	requestRestart("")
}

// runVersionChangeRebuild applies one or more version changes (upgrades or
// downgrades, including the base/core) by folding each change into the current
// member set, then triggering a single rebuild for the whole set.
func runVersionChangeRebuild(app *pocketbase.PocketBase, job *installJob) {
	defer finishJob(job)

	current, err := buildCurrentMemberSet(app)
	if err != nil {
		_ = failJob(job, "registry", err)
		return
	}
	buildID := newBuildID()
	m := RebuildManifest{BuildID: buildID, Members: current}
	for _, ch := range job.Changes {
		reg, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:s}", map[string]any{"s": ch.Slug})
		if err != nil {
			_ = failJob(job, "registry", fmt.Errorf("unknown package %q: %w", ch.Slug, err))
			return
		}
		spec, err := specForVersion(reg.GetString("npm_package"), ch.TargetVersion)
		if err != nil {
			_ = failJob(job, "spec", err)
			return
		}
		member := registrySlugToMember(ch.Slug)
		m = desiredSet(buildID, m.Members, setDelta{
			op: "version", slug: member, version: ch.TargetVersion, spec: spec,
		})
	}
	if err := rebuild(app, job, m); err != nil {
		job.Status = "failed"
		job.Error = err.Error()
	}
}
