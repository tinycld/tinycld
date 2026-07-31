package coreserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgbuild"
)

// The HOSTED package pipeline (DESIGN-org-package-agency §7 step 4): the
// existing Packages UI, wired to propose a deploy over the per-org control
// socket instead of rebuilding in-process and exit-75ing.
//
// The HTTP surface is the same /api/admin/packages group the single-tenant
// app serves — same paths, same request/response shapes, same auth, same SSE
// progress and durable pkg_install_log rows — so the client needs no hosted
// mode. What changes is everything behind the handlers (D6 steps 1–2):
//
//  1. metadata pre-flight through the control socket (resolve/versions) —
//     the confined tenant has no npm/node/git;
//  2. advisory compat solve against pkg_registry (same gates as the host);
//  3. warm build via POST /v1/build — the org keeps serving its current
//     build while a novel set pays its minutes (§6 build-latency UX);
//  4. snapshot the org's own DB (VACUUM INTO .deploy/backup.db);
//  5. run the DOWN delta in-tenant (syncMigrations against the built
//     artifact's migration list — the running process still has the
//     outgoing migrations registered);
//  6. POST /v1/deploy and wait to be killed. The pkg_install_log row stays
//     "running" across the restart discontinuity; the respawned tenant's
//     boot reconcile (tenant_pkg_state.go) finalizes it from the router's
//     durable deploy-result — success on commit, rolled_back on revert.
//
// Known limitation, recorded: hosted installs and version changes require
// npm-published packages. The org lockfile is a flat {name: version} map, so
// a git spec has nowhere to carry its provenance yet (§5's "per-entry
// integrity/provenance" lockfile extension is the future home).

// RegisterHostedPackageEndpoints registers the Packages UI's API in a tenant
// whose deploys ride the control socket. Called by RegisterTenant when the
// tenant is artifact-booted and a control socket exists.
func RegisterHostedPackageEndpoints(app *pocketbase.PocketBase, ch *DeployChannel, orgDir, artifactDir string) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/admin/packages")

		adminGuard := func(re *core.RequestEvent) error {
			return requireAdmin(app, re)
		}

		g.POST("/install", func(re *core.RequestEvent) error {
			return handleHostedInstall(app, ch, orgDir, re)
		}).BindFunc(adminGuard)

		g.POST("/uninstall", func(re *core.RequestEvent) error {
			return handleHostedUninstall(app, ch, orgDir, re)
		}).BindFunc(adminGuard)

		g.GET("/events/{jobId}", func(re *core.RequestEvent) error {
			return handleEvents(re)
		}).BindFunc(func(re *core.RequestEvent) error {
			return requireSuperuserOrToken(app, re)
		})

		// Both status reads first consume a pending deploy result: a
		// committed deploy's outcome lands on disk only after THIS process's
		// respawn settled, so the poll waiting for the verdict is what folds
		// it into pkg_install_log/pkg_registry (see tenant_pkg_state.go).
		g.GET("/status/{slug}", func(re *core.RequestEvent) error {
			consumePendingDeployResult(app, orgDir, artifactDir)
			return handleStatus(app, re)
		}).BindFunc(adminGuard)

		g.GET("/job-status/{jobId}", func(re *core.RequestEvent) error {
			consumePendingDeployResult(app, orgDir, artifactDir)
			return handleJobStatus(app, re)
		}).BindFunc(adminGuard)

		g.GET("/versions", func(re *core.RequestEvent) error {
			return handleHostedVersions(app, ch, re)
		}).BindFunc(adminGuard)

		g.POST("/versions/check", func(re *core.RequestEvent) error {
			return handleVersionsCheck(app, re)
		}).BindFunc(adminGuard)

		g.POST("/versions/drop-report", func(re *core.RequestEvent) error {
			return handleHostedDropReport(app, ch, re)
		}).BindFunc(adminGuard)

		g.POST("/versions/apply", func(re *core.RequestEvent) error {
			return handleHostedVersionChange(app, ch, orgDir, re)
		}).BindFunc(adminGuard)

		return e.Next()
	})
}

// ---------- handlers ----------

// claimHostedJob takes the single-flight slot, mirroring the host handlers'
// conflict response. Returns nil and writes the 409 when another operation is
// in flight.
func claimHostedJob(re *core.RequestEvent, action, slug, npmPkg string) (*installJob, error) {
	installMu.Lock()
	defer installMu.Unlock()
	if currentJob != nil {
		info := map[string]any{
			"jobId":  currentJob.ID,
			"action": currentJob.Action,
			"slug":   currentJob.Slug,
			"status": currentJob.Status,
		}
		return nil, re.JSON(http.StatusConflict, map[string]any{
			"error":      "Another install operation is in progress",
			"currentJob": info,
		})
	}
	job := &installJob{
		ID:     fmt.Sprintf("job_%d", time.Now().UnixMilli()),
		Action: action,
		Slug:   slug,
		NpmPkg: npmPkg,
		Status: "running",
		Done:   make(chan struct{}),
	}
	currentJob = job
	return job, nil
}

func handleHostedInstall(app *pocketbase.PocketBase, ch *DeployChannel, orgDir string, re *core.RequestEvent) error {
	var body struct {
		NpmPackage string `json:"npmPackage"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if err := validatePackageSpec(body.NpmPackage); err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	// The org lockfile is {npm name: version}; a git spec has no place to
	// carry its provenance yet. Refuse up front with the reason rather than
	// failing deep in the builder.
	if src, _ := classifySpec(body.NpmPackage); src != sourceNpm {
		return re.BadRequestError("hosted deployments currently install npm-published packages; git and URL specs are not yet supported here", nil)
	}

	job, err := claimHostedJob(re, "install", "", body.NpmPackage)
	if job == nil {
		return err
	}
	go runHostedInstall(app, job, productionHostedDeps(app, ch, orgDir))
	return re.JSON(http.StatusAccepted, map[string]any{"jobId": job.ID})
}

func handleHostedUninstall(app *pocketbase.PocketBase, ch *DeployChannel, orgDir string, re *core.RequestEvent) error {
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}
	if body.Slug == "" {
		return re.BadRequestError("slug is required", nil)
	}
	if err := rejectBaseUninstall(body.Slug); err != nil {
		return re.BadRequestError(err.Error(), nil)
	}

	job, err := claimHostedJob(re, "uninstall", body.Slug, "")
	if job == nil {
		return err
	}
	go runHostedUninstall(app, job, productionHostedDeps(app, ch, orgDir))
	return re.JSON(http.StatusAccepted, map[string]any{"jobId": job.ID})
}

func handleHostedVersionChange(app *pocketbase.PocketBase, ch *DeployChannel, orgDir string, re *core.RequestEvent) error {
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
		if !versionTokenPattern.MatchString(c.TargetVersion) {
			return re.BadRequestError("invalid targetVersion for "+c.Slug+": "+c.TargetVersion, nil)
		}
	}
	// The same authoritative pre-flight the host runs on /versions/apply; the
	// builder's peer gate re-checks from freshly-fetched manifests during the
	// warm build.
	if err := checkVersionChangeCompat(app, body.Changes); err != nil {
		return re.BadRequestError(err.Error(), err)
	}

	job, err := claimHostedJob(re, "version_change", body.Changes[0].Slug, "")
	if job == nil {
		return err
	}
	job.Changes = body.Changes
	go runHostedVersionChange(app, job, productionHostedDeps(app, ch, orgDir))
	return re.JSON(http.StatusAccepted, map[string]any{"jobId": job.ID})
}

func handleHostedVersions(app *pocketbase.PocketBase, ch *DeployChannel, re *core.RequestEvent) error {
	records, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
	if err != nil {
		return re.InternalServerError("Failed to load package registry", err)
	}
	discover := func(spec string) (pkgSource, []string, string) {
		v, err := ch.Versions(re.Request.Context(), spec)
		if err != nil {
			return sourceUnknown, nil, err.Error()
		}
		return pkgSource(v.Source), v.Versions, v.Error
	}
	infos := make([]versionInfo, 0, len(records))
	for _, rec := range records {
		infos = append(infos, versionInfoForRegistryRow(rec, discover))
	}
	return re.JSON(http.StatusOK, map[string]any{"packages": infos})
}

// handleHostedDropReport previews the schema a downgrade would destroy, like
// the host's handleDropReport, but with both migration lists resolved through
// the control socket (no owner map, no local npm): the drop set is
// resolve(current version) ∖ resolve(target version), dry-reverted in-tenant
// (the running process has the outgoing migrations registered).
func handleHostedDropReport(app *pocketbase.PocketBase, ch *DeployChannel, re *core.RequestEvent) error {
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

	name, current, err := hostedNpmSource(app, body.Slug)
	if err != nil {
		return re.BadRequestError(err.Error(), nil)
	}
	ctx := re.Request.Context()
	currentRes, err := ch.Resolve(ctx, name+"@"+current)
	if err != nil {
		return re.InternalServerError("Failed to resolve the installed version", err)
	}
	files := currentRes.Migrations
	if body.TargetVersion != "" {
		if targetRes, tErr := ch.Resolve(ctx, name+"@"+body.TargetVersion); tErr == nil {
			files = subtractStrings(files, targetRes.Migrations)
		} else {
			jobLogfStandalone("pkg_hosted: drop-report target resolve failed (%v); reporting full current set", tErr)
		}
	}
	// Only migrations the running binary can actually revert are reverted by
	// a real deploy; report exactly that.
	report, err := dryRevertNamedMigrations(app, registeredOnly(files))
	if err != nil {
		return re.InternalServerError("Failed to compute drop report", err)
	}
	return re.JSON(http.StatusOK, report)
}

// ---------- pipelines ----------

// hostedDeps injects the pipeline's effectful steps so orchestration
// (ordering, failure handling, restore) is unit-testable without a socket or
// a real SQLite snapshot — the same seam shape as rebuildDeps.
type hostedDeps struct {
	state   func(ctx context.Context) (ctlState, error)
	resolve func(ctx context.Context, spec string) (ctlResolvedSpec, error)
	build   func(ctx context.Context, lock map[string]string) (ctlBuildResult, error)
	deploy  func(ctx context.Context, lock map[string]string, jobID string) error
	applied func() ([]string, error)
	// snapshot returns two closures: restore puts the snapshot back as the
	// live DB (for failures after downs mutated it) and discard deletes it
	// without touching the DB (for failures where nothing was mutated — a
	// stale .deploy/backup.db MUST not linger, or a later failed operator
	// deploy would restore it over newer data).
	snapshot func() (restore, discard func() error, err error)
	syncMig  func(applied, newSet []string) (SyncResult, error)
	// recoverDB re-opens the live pools after an in-process restore (the
	// snapshot copy-back happens under the running app). Nil-safe.
	recoverDB   func() error
	finalizeLog func(status, errMsg string)
}

// productionHostedDeps wires the real steps for one operation.
func productionHostedDeps(app *pocketbase.PocketBase, ch *DeployChannel, orgDir string) hostedDeps {
	return hostedDeps{
		state:   ch.State,
		resolve: ch.Resolve,
		build:   ch.Build,
		deploy:  ch.Deploy,
		applied: func() ([]string, error) { return appliedMigrationFiles(app) },
		snapshot: func() (func() error, func() error, error) {
			return hostedSnapshot(app, orgDir)
		},
		syncMig: func(applied, newSet []string) (SyncResult, error) {
			return syncMigrations(app, applied, newSet)
		},
		recoverDB: func() error { return recoverLiveDBAfterExternalWrite(app) },
	}
}

func runHostedInstall(app core.App, job *installJob, d hostedDeps) {
	defer finishJob(job)
	logRecord := createInstallLog(app, job, "install")
	bindFinalize(&d, app, logRecord, job)
	jobLogf(job, "HOSTED INSTALL requested: spec=%s (job %s)", job.NpmPkg, job.ID)

	ctx := context.Background()
	emitProgress(job, "Resolving package", 2, "Fetching "+job.NpmPkg+" metadata via the router")
	res, err := d.resolve(ctx, job.NpmPkg)
	if err != nil {
		failHosted(d, job, "resolve", err)
		return
	}
	if res.Manifest == nil {
		failHosted(d, job, "resolve", fmt.Errorf("%s is not an installable feature package", job.NpmPkg))
		return
	}
	job.Slug = res.Slug
	updateInstallLogSlug(app, logRecord, res.Slug)

	manifest, err := pkgbuild.DecodeManifestJSON(res.Manifest)
	if err != nil {
		failHosted(d, job, "resolve", err)
		return
	}
	// hasGoPrereqs true: the build happens in the router's trusted builder,
	// which always has the toolchain — the tenant's own environment is not
	// the gate here.
	if err := validateManifest(manifest, true, getBundledSlugs(app)); err != nil {
		failHosted(d, job, "validate", err)
		return
	}
	if err := checkInstallCompat(app, manifest); err != nil {
		failHosted(d, job, "compat", err)
		return
	}
	emitProgress(job, "Manifest resolved", 4, fmt.Sprintf("%s (%s) %s", manifest.Name, res.Slug, res.Version))

	next, err := nextLockfile(ctx, d, func(lock map[string]string) {
		lock[res.Name] = res.Version
	})
	if err != nil {
		failHosted(d, job, "state", err)
		return
	}
	runHostedDeploy(job, d, next)
}

func runHostedUninstall(app core.App, job *installJob, d hostedDeps) {
	defer finishJob(job)
	logRecord := createInstallLog(app, job, "uninstall")
	bindFinalize(&d, app, logRecord, job)
	jobLogf(job, "HOSTED UNINSTALL requested: slug=%s (job %s)", job.Slug, job.ID)

	name, _, err := hostedNpmSource(app, job.Slug)
	if err != nil {
		failHosted(d, job, "registry", err)
		return
	}
	next, err := nextLockfile(context.Background(), d, func(lock map[string]string) {
		delete(lock, name)
	})
	if err != nil {
		failHosted(d, job, "state", err)
		return
	}
	runHostedDeploy(job, d, next)
}

func runHostedVersionChange(app core.App, job *installJob, d hostedDeps) {
	defer finishJob(job)
	logRecord := createInstallLog(app, job, "version_change")
	bindFinalize(&d, app, logRecord, job)
	jobLogf(job, "HOSTED VERSION CHANGE requested: %d change(s) (job %s)", len(job.Changes), job.ID)
	for _, ch := range job.Changes {
		jobLogf(job, "  change: %s -> %s", ch.Slug, ch.TargetVersion)
	}

	edits := map[string]string{}
	for _, change := range job.Changes {
		name, _, err := hostedNpmSource(app, change.Slug)
		if err != nil {
			failHosted(d, job, "registry", err)
			return
		}
		edits[name] = change.TargetVersion
	}
	next, err := nextLockfile(context.Background(), d, func(lock map[string]string) {
		for name, version := range edits {
			lock[name] = version
		}
	})
	if err != nil {
		failHosted(d, job, "state", err)
		return
	}
	runHostedDeploy(job, d, next)
}

// runHostedDeploy is D6 steps 1½–2 shared by all three operations: warm
// build → snapshot → downs → propose → wait to be killed. On a refused
// proposal after downs already ran, the tenant restores its own snapshot —
// every earlier failure leaves live state untouched.
func runHostedDeploy(job *installJob, d hostedDeps, next map[string]string) {
	ctx := context.Background()

	emitProgress(job, "Building artifact", 10,
		"The organization keeps serving its current build while the new one is prepared")
	build, err := d.build(ctx, next)
	if err != nil {
		failHosted(d, job, "build", err)
		return
	}
	if build.Cached {
		jobLogf(job, "artifact cache hit: %s", build.RecipeHash)
	} else {
		jobLogf(job, "artifact built: %s", build.RecipeHash)
	}

	applied, err := d.applied()
	if err != nil {
		failHosted(d, job, "migrate", err)
		return
	}

	emitProgress(job, "Backing up database", 85, "Creating pre-deploy snapshot")
	restore, discard, err := d.snapshot()
	if err != nil {
		failHosted(d, job, "backup", err)
		return
	}

	emitProgress(job, "Reconciling schema", 90, "Reverting migrations the new build drops")
	res, err := d.syncMig(applied, build.Migrations)
	if err != nil {
		// revertNamedMigrations aborts atomically, but the failed transaction
		// may be mid-batch state only in memory — restore the snapshot so the
		// live DB is exactly pre-deploy either way.
		jobLogf(job, "migration sync failed — restoring pre-deploy snapshot")
		restoreHosted(d, job, restore)
		failHosted(d, job, "migrate", err)
		return
	}
	logSyncResult(job, res)

	emitProgress(job, "Proposing deploy", 95, "Asking the router to restart onto the new build")
	if err := d.deploy(ctx, next, job.ID); err != nil {
		if len(res.Reverted) > 0 {
			jobLogf(job, "deploy proposal refused after %d down migration(s) — restoring pre-deploy snapshot", len(res.Reverted))
			restoreHosted(d, job, restore)
		} else if discardErr := discard(); discardErr != nil {
			jobLogf(job, "WARNING: could not discard unused snapshot: %v", discardErr)
		}
		failHosted(d, job, "deploy", err)
		return
	}

	// Accepted: the router owns the rest (evict → respawn → commit|revert).
	// The job and its log row deliberately stay "running" — the truthful
	// terminal state is decided by the respawned build's readiness, and the
	// boot reconcile writes it into pkg_install_log where the UI's job-status
	// poll picks it up across the restart discontinuity.
	jobLogf(job, "deploy accepted — the router will restart this organization onto %s; final status is recorded after the restart", build.RecipeHash)
	emitProgress(job, "Restarting", 100, "Deploy accepted — restarting onto the new build")
}

// ---------- helpers ----------

// bindFinalize wires the deps' log finalizer to the created row (deps are
// built before the row exists).
func bindFinalize(d *hostedDeps, app core.App, logRecord *core.Record, job *installJob) {
	d.finalizeLog = func(status, errMsg string) {
		finalizeInstallLog(app, logRecord, status, errMsg, job.LogLines)
	}
}

func failHosted(d hostedDeps, job *installJob, step string, err error) {
	if d.finalizeLog != nil {
		d.finalizeLog("failed", err.Error())
	}
	_ = failJob(job, step, err)
}

func restoreHosted(d hostedDeps, job *installJob, restore func() error) {
	if restore == nil {
		return
	}
	if err := restore(); err != nil {
		jobLogf(job, "WARNING: snapshot restore failed: %v", err)
		return
	}
	if d.recoverDB != nil {
		if err := d.recoverDB(); err != nil {
			jobLogf(job, "WARNING: DB reconnect after restore failed: %v", err)
		}
	}
}

// nextLockfile reads the org's authoritative current lockfile from the
// router and applies one edit to a copy.
func nextLockfile(ctx context.Context, d hostedDeps, edit func(map[string]string)) (map[string]string, error) {
	state, err := d.state(ctx)
	if err != nil {
		return nil, fmt.Errorf("read current package set: %w", err)
	}
	next := make(map[string]string, len(state.Lockfile)+1)
	for k, v := range state.Lockfile {
		next[k] = v
	}
	edit(next)
	return next, nil
}

// hostedNpmSource resolves a registry slug to its bare npm name (the org
// lockfile key) and current version. Hosted rows record the member's npm
// name in npm_package (tenant_pkg_state.go); a versioned or git spec is
// normalized/refused accordingly.
func hostedNpmSource(app core.App, slug string) (name, current string, err error) {
	rec, err := app.FindFirstRecordByFilter("pkg_registry", "slug = {:s}", map[string]any{"s": slug})
	if err != nil || rec == nil {
		return "", "", fmt.Errorf("package %q not in registry", slug)
	}
	spec := rec.GetString("npm_package")
	if spec == "" {
		return "", "", fmt.Errorf("package %q has no npm source recorded", slug)
	}
	src, key := classifySpec(spec)
	if src != sourceNpm {
		return "", "", fmt.Errorf("package %q has a non-npm source (%s); hosted deployments currently require npm-published packages", slug, spec)
	}
	return key, rec.GetString("version"), nil
}

// hostedSnapshot VACUUMs the live DB into <orgDir>/.deploy/backup.db — the
// path the router's Deployer restores from on a reverted deploy (D6 step 2).
// The tenant environment carries no sqlite3 CLI, and the app's own pool is
// opened with NoAttachDBConnect (SQLITE_LIMIT_ATTACHED = 0 — the jsvm
// ATTACH-DATABASE guard), which also refuses VACUUM INTO's internal attach —
// so the snapshot runs on a DEDICATED connection this function opens and
// closes. That connection is trusted coreserver Go, never reachable from the
// sandboxed VMs, so it does not weaken the boundary the pool restriction
// exists for. The returned restore closure is for IN-PROCESS failures only
// (a refused proposal); an accepted deploy leaves the snapshot in place for
// the router.
func hostedSnapshot(_ core.App, orgDir string) (restore, discard func() error, err error) {
	deployDir := filepath.Join(orgDir, ".deploy")
	if err := os.MkdirAll(deployDir, 0o755); err != nil {
		return nil, nil, err
	}
	backupPath := filepath.Join(deployDir, "backup.db")
	// VACUUM INTO refuses to overwrite; clear a stale snapshot first.
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("clear stale snapshot: %w", err)
	}
	// The path comes from our own --org-dir flag (router-controlled), not
	// user input; single quotes in it would break the literal, so refuse.
	if strings.ContainsRune(backupPath, '\'') {
		return nil, nil, fmt.Errorf("org dir path contains a quote: %q", backupPath)
	}
	dbPath := filepath.Join(orgDir, "pb_data", "data.db")
	db, err := core.DefaultDBConnect(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open snapshot connection: %w", err)
	}
	_, vacuumErr := db.NewQuery(fmt.Sprintf("VACUUM INTO '%s'", backupPath)).Execute()
	if closeErr := db.Close(); vacuumErr == nil {
		vacuumErr = closeErr
	}
	if vacuumErr != nil {
		return nil, nil, fmt.Errorf("snapshot database: %w", vacuumErr)
	}
	restore = func() error {
		if err := copyFileOver(backupPath, dbPath); err != nil {
			return fmt.Errorf("restore snapshot: %w", err)
		}
		// The snapshot is WAL-less; leftover -wal/-shm belong to the
		// overwritten file and would replay over the restore (the same lesson
		// as backupDatabase's rollbackFn and the router's restoreSnapshot).
		for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
			if err := os.Remove(sidecar); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove %s: %w", sidecar, err)
			}
		}
		return os.Remove(backupPath)
	}
	discard = func() error {
		if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return restore, discard, nil
}

func copyFileOver(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// jobLogfStandalone logs outside a job context (request-scoped paths).
func jobLogfStandalone(format string, args ...any) {
	log.Printf(format, args...)
}
