package coreserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// ---------- types ----------

type installJob struct {
	ID      string
	Action  string // "install", "uninstall", "revert", or "version_change"
	Slug    string
	NpmPkg  string
	BuildID string // revert target (action == "revert")
	// version_change: the ordered set of {slug → targetVersion} to apply together.
	Changes   []versionChange
	Progress  int
	Step      string
	Status    string // "running", "success", "failed", "rolled_back"
	Error     string
	LogLines  []string
	Done      chan struct{}
	listeners []chan sseEvent
	mu        sync.Mutex
}

type sseEvent struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

type progressData struct {
	Step     string `json:"step"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

type completeData struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// ---------- global state ----------

var (
	installMu  sync.Mutex
	currentJob *installJob
)

// ---------- registration ----------

func RegisterPackageInstallEndpoints(app *pocketbase.PocketBase) {
	// Guard against a hooks-watcher (HooksWatch) restart firing in the MIDDLE of
	// an install/revert/version-change pipeline. Those pipelines re-run the
	// generator, which rewrites the watched pb_hooks symlinks; with HooksWatch on,
	// PocketBase's jsvm watcher would call app.Restart() (an in-process re-exec)
	// and tear the process down between, say, the file swap and the migration step
	// — leaving a half-applied state with no rollback. app.Restart() routes
	// through OnTerminate with IsRestart=true, so we veto it while a job holds the
	// single-flight lock. Our OWN intentional restarts use os.Exit(75)
	// (requestRestart), a different path this guard never sees.
	app.OnTerminate().BindFunc(func(e *core.TerminateEvent) error {
		if shouldSuppressRestart(e.IsRestart) {
			log.Println("pkg_install: suppressing watcher restart — a package operation is in progress")
			return nil // short-circuit: don't run the execve handler
		}
		return e.Next()
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/admin/packages")

		g.POST("/install", func(re *core.RequestEvent) error {
			return handleInstall(app, re)
		}).BindFunc(requireSuperuser)

		g.POST("/uninstall", func(re *core.RequestEvent) error {
			return handleUninstall(app, re)
		}).BindFunc(requireSuperuser)

		g.POST("/revert", func(re *core.RequestEvent) error {
			return handleRevert(app, re)
		}).BindFunc(requireSuperuser)

		g.POST("/builds/delete", func(re *core.RequestEvent) error {
			return handleDeleteBuild(app, re)
		}).BindFunc(requireSuperuser)

		g.GET("/events/{jobId}", func(re *core.RequestEvent) error {
			return handleEvents(re)
		}).BindFunc(func(re *core.RequestEvent) error {
			return requireSuperuserOrToken(app, re)
		})

		g.GET("/status/{slug}", func(re *core.RequestEvent) error {
			return handleStatus(app, re)
		}).BindFunc(requireSuperuser)

		g.GET("/versions", func(re *core.RequestEvent) error {
			return handleVersions(app, re)
		}).BindFunc(requireSuperuser)

		g.POST("/versions/check", func(re *core.RequestEvent) error {
			return handleVersionsCheck(app, re)
		}).BindFunc(requireSuperuser)

		g.POST("/versions/drop-report", func(re *core.RequestEvent) error {
			return handleDropReport(app, re)
		}).BindFunc(requireSuperuser)

		g.POST("/versions/apply", func(re *core.RequestEvent) error {
			return handleVersionChange(app, re)
		}).BindFunc(requireSuperuser)

		return e.Next()
	})
}

// shouldSuppressRestart reports whether an in-process restart (isRestart) must be
// vetoed because a package pipeline is mid-flight. Pure read of the single-flight
// state under the lock, so it's unit-testable.
func shouldSuppressRestart(isRestart bool) bool {
	if !isRestart {
		return false
	}
	installMu.Lock()
	defer installMu.Unlock()
	return currentJob != nil
}

func requireSuperuser(re *core.RequestEvent) error {
	if !re.HasSuperuserAuth() {
		return re.ForbiddenError("Superuser access required", nil)
	}
	return re.Next()
}

// requireSuperuserOrToken allows SSE connections to authenticate via query param
// since browser EventSource does not support custom headers.
func requireSuperuserOrToken(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	// Try standard auth first
	if re.HasSuperuserAuth() {
		return re.Next()
	}

	// Fall back to query param token for SSE
	token := re.Request.URL.Query().Get("token")
	if token != "" {
		superusers, err := app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
		if err == nil {
			record, err := app.FindAuthRecordByToken(token, superusers.Id)
			if err == nil && record != nil {
				return re.Next()
			}
		}
	}

	return re.ForbiddenError("Superuser access required", nil)
}

// ---------- handlers ----------

func handleInstall(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		NpmPackage string `json:"npmPackage"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	if err := validatePackageSpec(body.NpmPackage); err != nil {
		return re.BadRequestError(err.Error(), nil)
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
			"error":      "Another install operation is in progress",
			"currentJob": info,
		})
	}

	jobId := fmt.Sprintf("job_%d", time.Now().UnixMilli())
	job := &installJob{
		ID:     jobId,
		Action: "install",
		NpmPkg: body.NpmPackage,
		Status: "running",
		Done:   make(chan struct{}),
	}
	currentJob = job
	installMu.Unlock()

	go runInstallPipeline(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": jobId})
}

func handleUninstall(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	var body struct {
		Slug string `json:"slug"`
	}
	if err := json.NewDecoder(re.Request.Body).Decode(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	if body.Slug == "" {
		return re.BadRequestError("slug is required", nil)
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
		ID:     jobId,
		Action: "uninstall",
		Slug:   body.Slug,
		Status: "running",
		Done:   make(chan struct{}),
	}
	currentJob = job
	installMu.Unlock()

	go runUninstallPipeline(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": jobId})
}

func handleEvents(re *core.RequestEvent) error {
	jobId := re.Request.PathValue("jobId")

	installMu.Lock()
	job := currentJob
	installMu.Unlock()

	if job == nil || job.ID != jobId {
		return re.NotFoundError("Job not found", nil)
	}

	// Set up SSE
	w := re.Response
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return re.InternalServerError("Streaming not supported", nil)
	}

	ch := make(chan sseEvent, 64)
	job.mu.Lock()
	job.listeners = append(job.listeners, ch)
	// Backfill the FULL progress history to a late-connecting client, not just
	// the latest step. The pipeline can blow through several fast early stages
	// (npm pack, manifest parse, file copy) in well under a second; a client
	// whose EventSource connects after that would otherwise never see those
	// messages, since only live events flow afterward. Replaying every recorded
	// LogLine (each is "[N%] Step: message") makes the stream's history complete
	// regardless of connect timing. We write these directly to the response here
	// — while holding job.mu so the snapshot is consistent — rather than through
	// the bounded channel, which could overflow on a long history.
	backfill := make([]progressData, 0, len(job.LogLines))
	for _, line := range job.LogLines {
		if pct, step, msg, ok := parseLogLine(line); ok {
			backfill = append(backfill, progressData{Step: step, Progress: pct, Message: msg})
		}
	}
	jobStatus := job.Status
	jobErr := job.Error
	jobDone := job.Status == "success" || job.Status == "failed" || job.Status == "rolled_back"
	job.mu.Unlock()

	for _, pd := range backfill {
		data, _ := json.Marshal(pd)
		fmt.Fprintf(w, "event: progress\ndata: %s\n\n", data)
	}
	// If the job already finished before this client connected, emit the
	// terminal event now so the modal resolves instead of hanging on the
	// (never-arriving) live complete event.
	if jobDone {
		status := "success"
		if jobStatus != "success" {
			status = "failed"
		}
		data, _ := json.Marshal(completeData{Status: status, Error: jobErr})
		fmt.Fprintf(w, "event: complete\ndata: %s\n\n", data)
		flusher.Flush()
		return nil
	}
	flusher.Flush()

	ctx := re.Request.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			data, _ := json.Marshal(evt.Data)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt.Event, data)
			flusher.Flush()
			if evt.Event == "complete" {
				return nil
			}
		}
	}
}

func handleStatus(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	slug := re.Request.PathValue("slug")

	// Return the MOST RECENT log for the slug, not an arbitrary one — a package
	// can have several entries over time (install, then revert), and callers want
	// the status of the latest operation.
	records, err := app.FindRecordsByFilter(
		"pkg_install_log",
		"pkg_slug = {:slug}",
		"-created",
		1,
		0,
		map[string]any{"slug": slug},
	)
	if err != nil || len(records) == 0 {
		return re.NotFoundError("No install log found for this package", nil)
	}
	record := records[0]

	return re.JSON(http.StatusOK, map[string]any{
		"id":          record.Id,
		"action":      record.GetString("action"),
		"status":      record.GetString("status"),
		"error":       record.GetString("error"),
		"startedAt":   record.GetString("started_at"),
		"completedAt": record.GetString("completed_at"),
	})
}

// ---------- install pipeline ----------

func runInstallPipeline(app *pocketbase.PocketBase, job *installJob) {
	defer func() {
		installMu.Lock()
		currentJob = nil
		installMu.Unlock()
		close(job.Done)
	}()

	// Resolve paths for the standalone-workspace layout. In the runtime image
	// the binary lives at <appDir>/tinycld, so resolveServerDir() == appDir
	// (/app): it holds pb_data/, scripts/ (the generator), the app rebuild
	// files, dist/, releases/, and release-staging/. The Go source + go.work
	// live one level deeper at <appDir>/server (goSrcDir). The npm workspace
	// root is one level UP at filepath.Dir(appDir) (/): it holds package.json,
	// node_modules, tinycld.packages.ts, and the feature members at <wsRoot>/<slug>.
	appDir := resolveServerDir()
	goSrcDir := filepath.Join(appDir, "server")
	wsRoot := filepath.Dir(appDir)

	// Create install log record
	logRecord := createInstallLog(app, job, "install")

	var rollbackStack []func()
	rollback := func() {
		for i := len(rollbackStack) - 1; i >= 0; i-- {
			rollbackStack[i]()
		}
	}

	fail := func(step string, err error) {
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed at %s: %v", step, err)
		emitProgress(job, step, job.Progress, "FAILED: "+err.Error())
		emitComplete(job, "failed", job.Error)
		rollback()
		finalizeInstallLog(app, logRecord, "failed", job.Error, job.LogLines)
	}

	// Step 1: Validate npm package name (5%)
	emitProgress(job, "Validating package name", 5, "Checking "+job.NpmPkg)
	if err := validatePackageSpec(job.NpmPkg); err != nil {
		fail("validate", err)
		return
	}
	if !isTrustedScope(job.NpmPkg) {
		emitProgress(job, "Security warning", 8, "Package is not in @tinycld/ scope — proceed with caution")
	}

	// Step 2: download the published package tarball (20%). This uses `npm pack
	// <spec>` deliberately — it's a registry-download operation (fetch a
	// published tarball by name/version), NOT a workspace install. pnpm has no
	// equivalent that takes a remote spec (`pnpm pack` only packs the local
	// package), so npm is the right tool here even though the workspace installs
	// with pnpm. The actual install (relinking the workspace) happens in step 7
	// via `pnpm install`.
	emitProgress(job, "Downloading package", 15, "Running npm pack "+job.NpmPkg)
	tmpDir, err := os.MkdirTemp("", "tinycld-pkg-*")
	if err != nil {
		fail("tmpdir", err)
		return
	}
	defer os.RemoveAll(tmpDir)

	packOut, err := runCmd(wsRoot, "npm", "pack", job.NpmPkg, "--pack-destination", tmpDir)
	if err != nil {
		fail("npm pack", fmt.Errorf("%v: %s", err, packOut))
		return
	}
	emitProgress(job, "Downloaded package", 20, "Package downloaded")

	// Step 3: Untar and parse manifest (30%)
	emitProgress(job, "Parsing manifest", 25, "Extracting and validating manifest")
	tgzFiles, _ := filepath.Glob(filepath.Join(tmpDir, "*.tgz"))
	if len(tgzFiles) == 0 {
		fail("untar", fmt.Errorf("no .tgz file found after npm pack"))
		return
	}
	_, err = runCmd(tmpDir, "tar", "xzf", tgzFiles[0], "-C", tmpDir)
	if err != nil {
		fail("untar", err)
		return
	}
	// npm pack always extracts into a subdirectory named "package"
	extractDir := filepath.Join(tmpDir, "package")
	if _, statErr := os.Stat(extractDir); statErr != nil {
		fail("untar", fmt.Errorf("extracted package directory not found"))
		return
	}

	manifest, err := parseManifestViaNode(extractDir)
	if err != nil {
		fail("parse manifest", err)
		return
	}
	job.Slug = manifest.Slug
	// The log row was created before the slug was known (it fell back to the npm
	// spec); rewrite it to the real slug now so status lookups by slug find it.
	updateInstallLogSlug(app, logRecord, job.Slug)
	emitProgress(job, "Manifest parsed", 30, fmt.Sprintf("Package: %s (%s)", manifest.Name, manifest.Slug))

	// Step 4: Validate manifest (35%)
	emitProgress(job, "Validating manifest", 33, "Checking manifest requirements")
	bundledSlugs := getBundledSlugs(app)
	hasGoPrereqs := checkGoBuildPrereqs() == nil
	if err := validateManifest(manifest, hasGoPrereqs, bundledSlugs); err != nil {
		fail("validate manifest", err)
		return
	}
	emitProgress(job, "Manifest valid", 35, "All validation checks passed")

	// Step 5: Copy package source to <wsRoot>/<slug>/ (40%). Feature packages
	// are top-level workspace members, not entries under a packages/ dir.
	pkgDest := filepath.Join(wsRoot, manifest.Slug)
	emitProgress(job, "Installing files", 38, "Copying to "+pkgDest)
	if err := copyDir(extractDir, pkgDest); err != nil {
		fail("copy", err)
		return
	}
	rollbackStack = append(rollbackStack, func() {
		os.RemoveAll(pkgDest)
		log.Printf("pkg_install: rollback — removed %s", pkgDest)
	})
	emitProgress(job, "Files installed", 40, "Package files copied")

	// Step 6: Add the member to the workspace package.json workspaces[] (45%).
	// npm only creates the node_modules/@tinycld/<name> symlink for declared
	// members; the generator discovers packages by scanning member dirs, but
	// metro/vitest resolution needs the symlink. There is no longer an
	// installed-packages.json — present members ARE the installed set.
	emitProgress(job, "Updating workspace", 43, "Adding member to package.json")
	wsPkgPath := filepath.Join(wsRoot, "package.json")
	prevWsPkg, readErr := os.ReadFile(wsPkgPath)
	if readErr != nil {
		fail("read workspace package.json", readErr)
		return
	}
	if err := addWorkspaceMember(wsPkgPath, manifest.Slug); err != nil {
		fail("update workspace package.json", err)
		return
	}
	rollbackStack = append(rollbackStack, func() {
		os.WriteFile(wsPkgPath, prevWsPkg, 0o644)
		log.Printf("pkg_install: rollback — restored workspace package.json")
	})
	emitProgress(job, "Workspace updated", 45, "Member added to workspaces[]")

	// Step 7: pnpm install at the workspace root (55%). The workspace is a pnpm
	// workspace; pnpm discovers the new member from pnpm-workspace.yaml (updated
	// above) and the postinstall (link-members + generator) wires it in.
	emitProgress(job, "Installing dependencies", 50, "Running pnpm install")
	npmOut, err := runCmdEnv(wsRoot, []string{"CI=true"}, "pnpm", "install", "--no-frozen-lockfile")
	if err != nil {
		fail("pnpm install", fmt.Errorf("%v: %s", err, npmOut))
		return
	}
	emitProgress(job, "Dependencies installed", 55, "pnpm install complete")

	// Step 8: Regenerate wiring (65%). The generator lives at app/scripts/generate.ts
	// and runs from the app dir.
	emitProgress(job, "Generating wiring", 60, "Running package generation script")
	genOut, err := runCmd(appDir, "npx", "tsx", "scripts/generate.ts")
	if err != nil {
		fail("generate", fmt.Errorf("%v: %s", err, genOut))
		return
	}
	rollbackStack = append(rollbackStack, func() {
		// Re-run generation after the member was removed above
		runCmd(appDir, "npx", "tsx", "scripts/generate.ts")
		log.Printf("pkg_install: rollback — re-ran generation")
	})
	emitProgress(job, "Wiring generated", 65, "Package wiring regenerated")

	// Go package steps (Phase 3): build new binary, backup DB, swap. The Go
	// toolchain runs in goSrcDir (<appDir>/server, where go.work lives); the
	// new/live binary and pb_data are in appDir.
	if manifest.HasServer {
		// Reconcile the Go workspace with `go work sync`, NOT `go mod tidy`.
		// The feature server modules (tinycld.org/packages/<slug>) are local
		// modules wired only through the generated go.work `use` directives;
		// `go mod tidy` ignores go.work for those imports and tries to fetch
		// them from the network ("unrecognized import path …/packages/<slug>"),
		// which fails. `go work sync` honors the `use` entries and reconciles
		// go.sum offline — the same step the Docker image's go-builder runs
		// before building. Skip when there's no go.work (defensive; the
		// generator always emits one for a workspace with server packages).
		emitProgress(job, "Updating Go modules", 67, "Running go work sync")
		goWorkPath := filepath.Join(goSrcDir, "go.work")
		if _, statErr := os.Stat(goWorkPath); statErr == nil {
			syncOut, syncErr := runCmd(goSrcDir, "go", "work", "sync")
			if syncErr != nil {
				fail("go work sync", fmt.Errorf("%v: %s", syncErr, syncOut))
				return
			}
		}

		emitProgress(job, "Building server", 70, "Compiling new server binary")
		if buildErr := buildNewBinary(goSrcDir, appDir); buildErr != nil {
			fail("go build", buildErr)
			return
		}
		rollbackStack = append(rollbackStack, func() {
			os.Remove(filepath.Join(appDir, "tinycld.new"))
			log.Printf("pkg_install: rollback — removed tinycld.new")
		})

		emitProgress(job, "Validating binary", 73, "Running binary health check")
		if valErr := validateBinary(filepath.Join(appDir, "tinycld.new")); valErr != nil {
			fail("validate binary", valErr)
			return
		}

		emitProgress(job, "Backing up database", 75, "Creating SQLite backup")
		dbRollback, dbErr := backupDatabase(appDir)
		if dbErr != nil {
			fail("database backup", dbErr)
			return
		}
		rollbackStack = append(rollbackStack, func() {
			if err := dbRollback(); err != nil {
				log.Printf("pkg_install: rollback — database restore failed: %v", err)
			}
		})

		emitProgress(job, "Swapping binary", 77, "Installing new server binary")
		binRollback, binErr := swapBinary(appDir)
		if binErr != nil {
			fail("binary swap", binErr)
			return
		}
		rollbackStack = append(rollbackStack, func() {
			if err := binRollback(); err != nil {
				log.Printf("pkg_install: rollback — binary restore failed: %v", err)
			}
		})
	}

	// Snapshot the _migrations history before applying so we can record exactly
	// which migrations this install adds — the count drives `migrate down N` on a
	// future revert.
	migrationsBefore, beforeErr := appliedMigrationFiles(app)
	if beforeErr != nil {
		log.Printf("pkg_install: failed to snapshot migrations before apply: %v", beforeErr)
	}

	// Run migrations (via new binary if Go package, otherwise current)
	emitProgress(job, "Running migrations", 80, "Applying database migrations")
	migrateBin := resolveServerBinary()
	if manifest.HasServer {
		migrateBin = filepath.Join(appDir, binaryName)
	}
	migrateOut, err := runCmd(appDir, migrateBin, "migrate")
	if err != nil {
		fail("migrate", fmt.Errorf("%v: %s", err, migrateOut))
		return
	}
	emitProgress(job, "Migrations applied", 83, "Database migrations complete")

	// Determine which migrations the apply step added.
	migrationsAfter, afterErr := appliedMigrationFiles(app)
	if afterErr != nil {
		log.Printf("pkg_install: failed to snapshot migrations after apply: %v", afterErr)
	}
	appliedNow := newMigrationFiles(migrationsBefore, migrationsAfter)
	// Narrow the install's migration delta to the files this package actually
	// owns (per the generator's attribution map). For a normal single-package
	// install appliedNow is already exactly this package's files, but the
	// intersection is the source of truth a per-package version revert reads, so
	// record it explicitly rather than relying on the delta staying clean.
	pkgMigrationFiles := intersectStrings(appliedNow, migrationsForPackage(manifest.Slug))

	// Rebuild web bundle. expo export runs from the app dir and writes to
	// <appDir>/dist.
	emitProgress(job, "Building web app", 85, "Running expo export")
	buildOut, err := runCmd(appDir, "npx", "expo", "export", "--platform", "web")
	if err != nil {
		fail("build", fmt.Errorf("%v: %s", err, buildOut))
		return
	}
	emitProgress(job, "Web app built", 88, "Web bundle rebuilt")

	// Stage the new bundle as a release for the entrypoint to promote on the
	// post-restart boot (see stageRelease + entrypoint.sh promote_release).
	emitProgress(job, "Staging release", 90, "Preparing web bundle for promotion")
	stageDest, err := stageRelease(appDir)
	if err != nil {
		fail("stage release", err)
		return
	}
	rollbackStack = append(rollbackStack, func() {
		os.RemoveAll(stageDest)
		log.Printf("pkg_install: rollback — removed staged release %s", filepath.Base(stageDest))
	})
	emitProgress(job, "Release staged", 92, "Web bundle staged for next boot")

	// Update pkg_registry
	emitProgress(job, "Updating database", 95, "Updating package registry")
	manifestJSON, _ := json.Marshal(manifest.RawJSON)
	if err := upsertPkgRegistry(app, manifest, job.NpmPkg, manifestJSON); err != nil {
		fail("registry update", err)
		return
	}
	emitProgress(job, "Database updated", 97, "Package registry updated")

	// Archive this build as a restorable snapshot (binary + web bundle + the
	// metadata a revert needs). The build_id doubles as the on-disk archive dir.
	emitProgress(job, "Archiving build", 98, "Saving build for revert")
	buildID := fmt.Sprintf("build-%d", time.Now().UnixMilli())
	releaseID := filepath.Base(stageDest)
	// build.json on disk mirrors the pkg_build record so a build is self-describing
	// for offline restore.
	buildFields := map[string]any{
		"build_id":            buildID,
		"pkg_slug":            manifest.Slug,
		"npm_package":         job.NpmPkg,
		"version":             manifest.Version,
		"action":              "install",
		"binary_archived":     manifest.HasServer,
		"release_id":          releaseID,
		"migrations_applied":  len(appliedNow),
		"migration_files":     appliedNow,
		"pkg_migration_files": pkgMigrationFiles,
	}
	_, archiveCleanup, archErr := archiveBuild(appDir, buildID, stageDest, manifest.HasServer, buildFields)
	if archErr != nil {
		fail("archive build", archErr)
		return
	}
	rollbackStack = append(rollbackStack, archiveCleanup)

	if _, err := recordBuild(app, buildFields); err != nil {
		fail("record build", err)
		return
	}
	emitProgress(job, "Build archived", 99, "Build saved")

	// Request restart
	emitProgress(job, "Requesting restart", 99, "Signaling server restart")
	job.Status = "success"
	finalizeInstallLog(app, logRecord, "success", "", job.LogLines)
	emitComplete(job, "success", "")

	// Allow SSE events to flush to clients before process exit
	time.Sleep(2 * time.Second)
	requestRestart(appDir)
}

// ---------- uninstall pipeline ----------

func runUninstallPipeline(app *pocketbase.PocketBase, job *installJob) {
	defer func() {
		installMu.Lock()
		currentJob = nil
		installMu.Unlock()
		close(job.Done)
	}()

	appDir := resolveServerDir()
	wsRoot := filepath.Dir(appDir)

	logRecord := createInstallLog(app, job, "uninstall")

	fail := func(step string, err error) {
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed at %s: %v", step, err)
		emitProgress(job, step, job.Progress, "FAILED: "+err.Error())
		emitComplete(job, "failed", job.Error)
		finalizeInstallLog(app, logRecord, "failed", job.Error, job.LogLines)
	}

	// Step 1: Verify package exists and is not bundled
	emitProgress(job, "Verifying package", 10, "Checking "+job.Slug)
	record, err := app.FindFirstRecordByFilter(
		"pkg_registry",
		"slug = {:slug}",
		map[string]any{"slug": job.Slug},
	)
	if err != nil {
		fail("verify", fmt.Errorf("package %s not found in registry", job.Slug))
		return
	}
	if record.GetString("status") == "bundled" {
		fail("verify", fmt.Errorf("cannot uninstall bundled package %s", job.Slug))
		return
	}

	// Step 2: Remove the package's workspace-member directory at <wsRoot>/<slug>.
	emitProgress(job, "Removing files", 25, "Removing package directory")
	pkgDir := filepath.Join(wsRoot, job.Slug)
	if err := os.RemoveAll(pkgDir); err != nil {
		fail("remove", err)
		return
	}
	emitProgress(job, "Files removed", 30, "Package directory removed")

	// Step 3: Drop the member from the workspace package.json workspaces[].
	emitProgress(job, "Updating workspace", 38, "Removing member from package.json")
	wsPkgPath := filepath.Join(wsRoot, "package.json")
	if err := removeWorkspaceMember(wsPkgPath, job.Slug); err != nil {
		fail("update workspace package.json", err)
		return
	}
	emitProgress(job, "Workspace updated", 42, "Member removed from workspaces[]")

	// Step 4: pnpm install at the workspace root to clean the now-orphaned
	// node_modules/@tinycld/<slug> symlink before the rebuild. Without this the
	// dangling symlink can break expo export's module resolution.
	emitProgress(job, "Updating dependencies", 48, "Running pnpm install")
	npmOut, err := runCmdEnv(wsRoot, []string{"CI=true"}, "pnpm", "install", "--no-frozen-lockfile")
	if err != nil {
		fail("pnpm install", fmt.Errorf("%v: %s", err, npmOut))
		return
	}
	emitProgress(job, "Dependencies updated", 52, "pnpm install complete")

	// Step 5: Regenerate wiring (generator lives at app/scripts/generate.ts).
	emitProgress(job, "Regenerating wiring", 58, "Running package generation script")
	genOut, err := runCmd(appDir, "npx", "tsx", "scripts/generate.ts")
	if err != nil {
		fail("generate", fmt.Errorf("%v: %s", err, genOut))
		return
	}
	emitProgress(job, "Wiring regenerated", 65, "Package wiring regenerated")

	// Step 6: Rebuild web bundle (expo export from the app dir → <appDir>/dist).
	emitProgress(job, "Building web app", 75, "Running expo export")
	buildOut, err := runCmd(appDir, "npx", "expo", "export", "--platform", "web")
	if err != nil {
		fail("build", fmt.Errorf("%v: %s", err, buildOut))
		return
	}
	emitProgress(job, "Web app built", 85, "Web bundle rebuilt")

	// Step 7: Stage the rebuilt bundle as a release for the entrypoint to
	// promote on the post-restart boot (same contract as install).
	emitProgress(job, "Staging release", 88, "Preparing web bundle for promotion")
	if _, err := stageRelease(appDir); err != nil {
		fail("stage release", err)
		return
	}

	// Step 8: Update pkg_registry to disabled
	emitProgress(job, "Updating database", 93, "Marking package as disabled")
	record.Set("status", "disabled")
	if err := app.Save(record); err != nil {
		fail("registry update", err)
		return
	}

	emitProgress(job, "Complete", 98, "Package uninstalled")
	job.Status = "success"
	finalizeInstallLog(app, logRecord, "success", "", job.LogLines)
	emitComplete(job, "success", "")

	// Allow SSE events to flush to clients before process exit
	time.Sleep(2 * time.Second)
	requestRestart(appDir)
}

// ---------- SSE helpers ----------

func emitProgress(job *installJob, step string, progress int, message string) {
	job.mu.Lock()
	defer job.mu.Unlock()
	job.Step = step
	job.Progress = progress
	job.LogLines = append(job.LogLines, fmt.Sprintf("[%d%%] %s: %s", progress, step, message))
	log.Printf("[pkg_install] [%s] [%d%%] %s: %s", job.ID, progress, step, message)

	evt := sseEvent{
		Event: "progress",
		Data: progressData{
			Step:     step,
			Progress: progress,
			Message:  message,
		},
	}
	for _, ch := range job.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
}

// parseLogLine reverses emitProgress's "[N%] Step: message" formatting back into
// its parts, for replaying recorded history to a late-connecting SSE client.
// Returns ok=false for any line that doesn't match the shape.
func parseLogLine(line string) (pct int, step, msg string, ok bool) {
	if !strings.HasPrefix(line, "[") {
		return 0, "", "", false
	}
	closeIdx := strings.Index(line, "%] ")
	if closeIdx < 1 {
		return 0, "", "", false
	}
	n, err := strconv.Atoi(line[1:closeIdx])
	if err != nil {
		return 0, "", "", false
	}
	rest := line[closeIdx+3:]
	colon := strings.Index(rest, ": ")
	if colon < 0 {
		return n, rest, "", true
	}
	return n, rest[:colon], rest[colon+2:], true
}

func emitComplete(job *installJob, status string, errMsg string) {
	job.mu.Lock()
	defer job.mu.Unlock()

	if errMsg != "" {
		log.Printf("[pkg_install] [%s] COMPLETE status=%s error=%s", job.ID, status, errMsg)
	} else {
		log.Printf("[pkg_install] [%s] COMPLETE status=%s", job.ID, status)
	}

	evt := sseEvent{
		Event: "complete",
		Data: completeData{
			Status: status,
			Error:  errMsg,
		},
	}
	for _, ch := range job.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
}

// ---------- install log helpers ----------

func createInstallLog(app core.App, job *installJob, action string) *core.Record {
	collection, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		log.Printf("pkg_install: failed to find pkg_install_log collection: %v", err)
		return nil
	}

	// pkg_slug is required, but for an install the real slug isn't known until
	// the manifest is parsed several steps in. Fall back to the npm spec so the
	// row always persists; updateInstallLogSlug rewrites it once the slug is
	// known. (uninstall/revert pass job.Slug up front, so they skip the fallback.)
	slug := job.Slug
	if slug == "" {
		slug = job.NpmPkg
	}

	record := core.NewRecord(collection)
	record.Set("action", action)
	record.Set("pkg_slug", slug)
	record.Set("npm_package", job.NpmPkg)
	record.Set("status", "running")
	record.Set("started_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))

	if err := app.Save(record); err != nil {
		log.Printf("pkg_install: failed to create install log: %v", err)
		return nil
	}

	return record
}

// updateInstallLogSlug rewrites the log row's pkg_slug once the real slug is
// known (the install pipeline creates the row before parsing the manifest).
func updateInstallLogSlug(app core.App, record *core.Record, slug string) {
	if record == nil || slug == "" {
		return
	}
	record.Set("pkg_slug", slug)
	if err := app.Save(record); err != nil {
		log.Printf("pkg_install: failed to update install log slug: %v", err)
	}
}

func finalizeInstallLog(app core.App, record *core.Record, status string, errMsg string, logLines []string) {
	if record == nil {
		return
	}

	record.Set("status", status)
	record.Set("error", errMsg)
	record.Set("log", strings.Join(logLines, "\n"))
	record.Set("completed_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))

	if err := app.Save(record); err != nil {
		log.Printf("pkg_install: failed to finalize install log: %v", err)
	}
}

// ---------- release staging ----------

// stageRelease moves the freshly-built <appDir>/dist into
// <appDir>/release-staging/<id>/ with a release-id.txt and index.html renamed
// to app.html, matching the layout entrypoint.sh's promote_release expects. The
// entrypoint promotes the staged release (merging assets into releases/_static/
// and pointing releases/current at it) on the next boot — which the install /
// uninstall pipelines trigger via requestRestart. Returns the staged dir.
func stageRelease(appDir string) (string, error) {
	releaseID := fmt.Sprintf("install-%d", time.Now().UnixMilli())
	distDir := filepath.Join(appDir, "dist")
	stagingDir := filepath.Join(appDir, "release-staging")
	stageDest := filepath.Join(stagingDir, releaseID)

	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	// Prefer a rename (atomic, same filesystem); fall back to copy across
	// devices or when the destination can't be renamed into.
	if err := os.Rename(distDir, stageDest); err != nil {
		if cpErr := copyDir(distDir, stageDest); cpErr != nil {
			return "", fmt.Errorf("move dist failed: %v; copy fallback failed: %w", err, cpErr)
		}
		os.RemoveAll(distDir)
	}

	if err := os.WriteFile(filepath.Join(stageDest, "release-id.txt"), []byte(releaseID), 0o644); err != nil {
		return "", err
	}
	stagedIndex := filepath.Join(stageDest, "index.html")
	stagedApp := filepath.Join(stageDest, "app.html")
	if _, statErr := os.Stat(stagedIndex); statErr == nil {
		if err := os.Rename(stagedIndex, stagedApp); err != nil {
			return "", fmt.Errorf("rename index.html → app.html: %w", err)
		}
	}
	return stageDest, nil
}

// ---------- workspace package.json helpers ----------

// addWorkspaceMember adds slug to the workspaces[] array of the workspace-root
// package.json if absent. npm only creates the node_modules/@tinycld/<name>
// symlink for declared members. Idempotent. Preserves the file's other keys and
// the canonical 4-space indentation.
// addWorkspaceMember registers slug as a workspace member. pnpm reads members
// from pnpm-workspace.yaml (the authoritative list), so that's updated; the
// package.json workspaces[] array is also kept in sync as a tooling hint.
// Idempotent.
func addWorkspaceMember(pkgPath, slug string) error {
	pkg, err := readWorkspacePkg(pkgPath)
	if err != nil {
		return err
	}
	members := toStringSlice(pkg["workspaces"])
	present := false
	for _, m := range members {
		if m == slug {
			present = true
			break
		}
	}
	if !present {
		pkg["workspaces"] = append(members, slug)
		if err := writeWorkspacePkg(pkgPath, pkg); err != nil {
			return err
		}
	}
	return addPnpmWorkspaceMember(filepath.Join(filepath.Dir(pkgPath), "pnpm-workspace.yaml"), slug)
}

// removeWorkspaceMember unregisters slug from both pnpm-workspace.yaml and the
// package.json workspaces[] hint. Idempotent.
func removeWorkspaceMember(pkgPath, slug string) error {
	pkg, err := readWorkspacePkg(pkgPath)
	if err != nil {
		return err
	}
	members := toStringSlice(pkg["workspaces"])
	filtered := make([]string, 0, len(members))
	for _, m := range members {
		if m != slug {
			filtered = append(filtered, m)
		}
	}
	pkg["workspaces"] = filtered
	if err := writeWorkspacePkg(pkgPath, pkg); err != nil {
		return err
	}
	return removePnpmWorkspaceMember(filepath.Join(filepath.Dir(pkgPath), "pnpm-workspace.yaml"), slug)
}

// addPnpmWorkspaceMember inserts `  - <slug>` into the `packages:` block of
// pnpm-workspace.yaml if absent. The file is hand-maintained YAML (written by
// bootstrap); we do a targeted line edit rather than a full parse/serialize so
// comments and key order are preserved. A missing file is a no-op (a real
// pnpm workspace always has one; absence means nothing to update).
func addPnpmWorkspaceMember(yamlPath, slug string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	pkgIdx := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t") == "packages:" {
			pkgIdx = i
			break
		}
	}
	if pkgIdx == -1 {
		return fmt.Errorf("no packages: block in %s", yamlPath)
	}
	// Already a member, or find the last entry in the block.
	entry := "  - " + slug
	lastEntry := pkgIdx
	for i := pkgIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(lines[i], "  -") {
			if trimmed == "- "+slug {
				return nil // already present
			}
			lastEntry = i
		} else if len(lines[i]) > 0 && lines[i][0] != ' ' && trimmed != "" {
			break // next top-level key ends the block
		}
	}
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:lastEntry+1]...)
	out = append(out, entry)
	out = append(out, lines[lastEntry+1:]...)
	return os.WriteFile(yamlPath, []byte(strings.Join(out, "\n")), 0o644)
}

// removePnpmWorkspaceMember deletes the `  - <slug>` line from pnpm-workspace.yaml's
// packages: block if present. Missing file is a no-op. Idempotent.
func removePnpmWorkspaceMember(yamlPath, slug string) error {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l) == "- "+slug {
			continue
		}
		out = append(out, l)
	}
	return os.WriteFile(yamlPath, []byte(strings.Join(out, "\n")), 0o644)
}

func readWorkspacePkg(pkgPath string) (map[string]any, error) {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, err
	}
	var pkg map[string]any
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", pkgPath, err)
	}
	return pkg, nil
}

func writeWorkspacePkg(pkgPath string, pkg map[string]any) error {
	data, err := json.MarshalIndent(pkg, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(pkgPath, append(data, '\n'), 0o644)
}

// toStringSlice coerces a decoded JSON array (which json.Unmarshal yields as
// []any) into []string, dropping non-string entries.
func toStringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// ---------- pkg_registry helpers ----------

func upsertPkgRegistry(app core.App, m *parsedManifest, npmPkg string, manifestJSON []byte) error {
	existing, err := app.FindFirstRecordByFilter(
		"pkg_registry",
		"slug = {:slug}",
		map[string]any{"slug": m.Slug},
	)

	if err == nil {
		// Update existing
		existing.Set("name", m.Name)
		existing.Set("version", m.Version)
		existing.Set("npm_package", npmPkg)
		existing.Set("description", m.Description)
		existing.Set("icon", m.Nav.Icon)
		existing.Set("has_server", m.HasServer)
		existing.Set("status", "installed")
		existing.Set("manifest_json", json.RawMessage(manifestJSON))
		return app.Save(existing)
	}

	// Create new
	collection, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		return err
	}
	record := core.NewRecord(collection)
	record.Set("name", m.Name)
	record.Set("slug", m.Slug)
	record.Set("version", m.Version)
	record.Set("npm_package", npmPkg)
	record.Set("description", m.Description)
	record.Set("icon", m.Nav.Icon)
	record.Set("has_server", m.HasServer)
	record.Set("status", "installed")
	record.Set("manifest_json", json.RawMessage(manifestJSON))
	record.Set("nav_order", m.Nav.Order)
	return app.Save(record)
}

func getBundledSlugs(app core.App) map[string]bool {
	slugs := make(map[string]bool)
	records, err := app.FindRecordsByFilter(
		"pkg_registry",
		"status = 'bundled'",
		"",
		0,
		0,
	)
	if err != nil {
		return slugs
	}
	for _, r := range records {
		slugs[r.GetString("slug")] = true
	}
	return slugs
}

// ---------- utility helpers ----------

// runCmd runs a command, capturing its combined output to return to the
// caller (which surfaces it via SSE + the install-log record) AND echoing
// the command line and its output to the server's stdout so `docker logs`
// shows the full install trace — including the real npm/pnpm/go/expo errors
// that would otherwise be buried in the SSE stream / DB record only.
func runCmd(dir string, name string, args ...string) (string, error) {
	log.Printf("[pkg_install] $ (cd %s && %s %s)", dir, name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		log.Printf("[pkg_install] output of %s:\n%s", name, s)
	}
	if err != nil {
		log.Printf("[pkg_install] %s FAILED: %v", name, err)
	}
	return string(out), err
}

// runCmdEnv is runCmd with additional environment variables appended to the
// inherited environment. Used for the pnpm-install steps, which must run with
// CI=true so pnpm proceeds non-interactively (otherwise it blocks on a
// node_modules-purge confirmation and exits 1: "If you are running pnpm in CI,
// set the CI environment variable to 'true'…").
func runCmdEnv(dir string, env []string, name string, args ...string) (string, error) {
	log.Printf("[pkg_install] $ (cd %s && %s %s %s)", dir, strings.Join(env, " "), name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if s := strings.TrimRight(string(out), "\n"); s != "" {
		log.Printf("[pkg_install] output of %s:\n%s", name, s)
	}
	if err != nil {
		log.Printf("[pkg_install] %s FAILED: %v", name, err)
	}
	return string(out), err
}

func copyDir(src, dst string) error {
	_, err := runCmd(".", "cp", "-a", src+"/.", dst+"/")
	return err
}

func resolveServerDir() string {
	ex, err := os.Executable()
	if err != nil {
		return "./server"
	}
	dir := filepath.Dir(ex)
	// If running from a temp dir (go run), use ./server
	if strings.HasPrefix(dir, os.TempDir()) {
		return filepath.Join(".", "server")
	}
	return dir
}

func resolveServerBinary() string {
	ex, err := os.Executable()
	if err != nil {
		return "./" + binaryName
	}
	if strings.HasPrefix(ex, os.TempDir()) {
		return "go"
	}
	return ex
}
