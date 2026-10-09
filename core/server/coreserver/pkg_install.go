package coreserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/installjob"
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
			srvLog.Info("suppressing watcher restart — a package operation is in progress")
			return nil // short-circuit: don't run the execve handler
		}
		return e.Next()
	})

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		g := e.Router.Group("/api/admin/packages")

		// Package operations change what the whole deployment runs, so they
		// accept a PB superuser OR the org owner — not every admin.
		ownerGuard := func(re *core.RequestEvent) error {
			return requireOwner(re)
		}

		g.POST("/install", func(re *core.RequestEvent) error {
			return handleInstall(app, re)
		}).BindFunc(ownerGuard)

		g.POST("/uninstall", func(re *core.RequestEvent) error {
			return handleUninstall(app, re)
		}).BindFunc(ownerGuard)

		g.POST("/revert", func(re *core.RequestEvent) error {
			return handleRevert(app, re)
		}).BindFunc(ownerGuard)

		g.POST("/builds/delete", func(re *core.RequestEvent) error {
			return handleDeleteBuild(app, re)
		}).BindFunc(ownerGuard)

		g.GET("/status/{slug}", func(re *core.RequestEvent) error {
			return handleStatus(app, re)
		}).BindFunc(ownerGuard)

		g.GET("/versions", func(re *core.RequestEvent) error {
			return handleVersions(app, re)
		}).BindFunc(ownerGuard)

		g.POST("/versions/check", func(re *core.RequestEvent) error {
			return handleVersionsCheck(app, re)
		}).BindFunc(ownerGuard)

		g.POST("/versions/drop-report", func(re *core.RequestEvent) error {
			return handleDropReport(app, re)
		}).BindFunc(ownerGuard)

		g.POST("/versions/apply", func(re *core.RequestEvent) error {
			return handleVersionChange(app, re)
		}).BindFunc(ownerGuard)

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
	return installjob.Running()
}

// isOwner reports whether the given user holds the owner role. Package
// operations rebuild the deployment's artifact and change what every user of
// the deployment runs, so they are the owner's alone — admins manage members
// and settings but never the installed package set.
func isOwner(user *core.Record) bool {
	return user != nil && user.GetString("role") == "owner"
}

// requireOwner authorizes a PB superuser OR an app user with role=owner.
func requireOwner(re *core.RequestEvent) error {
	if re.HasSuperuserAuth() {
		return re.Next()
	}
	if isOwner(re.Auth) {
		return re.Next()
	}
	return re.ForbiddenError("Owner access required", nil)
}

// requireAdmin authorizes a PB superuser OR an owner/admin app user. This is
// the admin console's outer gate; the package endpoints sit behind the
// stricter requireOwner.
func requireAdmin(re *core.RequestEvent) error {
	if re.HasSuperuserAuth() {
		return re.Next()
	}
	if isOrgAdmin(re.Auth) {
		return re.Next()
	}
	return re.ForbiddenError("Admin access required", nil)
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

	job := installjob.New("install", "", body.NpmPackage)
	if busy, ok := installjob.Claim(job); !ok {
		return re.JSON(http.StatusConflict, map[string]any{
			"error":      "Another install operation is in progress",
			"currentJob": busy.Info(),
		})
	}

	go runInstallRebuild(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": job.ID})
}

// rejectBaseUninstall returns an error when slug is the TinyCld base (`core`).
// The base is the platform itself — uninstalling it (or disabling it, which is
// the uninstall pipeline's terminal state) would strand the deployment without
// its server. The /admin UI hides these controls for the core row; this guards
// the API path so a direct call can't remove the platform.
func rejectBaseUninstall(slug string) error {
	if slug == "core" {
		return fmt.Errorf("the TinyCld base cannot be uninstalled")
	}
	return nil
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

	if err := rejectBaseUninstall(body.Slug); err != nil {
		return re.BadRequestError(err.Error(), nil)
	}

	job := installjob.New("uninstall", body.Slug, "")
	if busy, ok := installjob.Claim(job); !ok {
		return re.JSON(http.StatusConflict, map[string]any{
			"error":      "Another operation is in progress",
			"currentJob": busy.Info(),
		})
	}

	go runUninstallRebuild(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": job.ID})
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

	return re.JSON(http.StatusOK, installLogStatusJSON(record))
}

// installLogStatusJSON is the status payload for the slug-keyed status
// endpoint.
func installLogStatusJSON(record *core.Record) map[string]any {
	return map[string]any{
		"id":          record.Id,
		"action":      record.GetString("action"),
		"status":      record.GetString("status"),
		"error":       record.GetString("error"),
		"startedAt":   record.GetString("started_at"),
		"completedAt": record.GetString("completed_at"),
	}
}

// emitProgress and emitStepProgress are a job's one path to report a
// milestone: they move the in-memory job's bar (RecordProgress, still read by
// Sentry capture and the busy-interlock Info()) AND throttle-save the same
// milestone onto the job's pkg_install_log row via its registered
// installLogProgressSaver, which is how a client now learns progress — see
// install_progress_rows.go.
func emitProgress(job *installjob.Job, step string, progress int, message string) {
	if job == nil {
		return
	}
	job.RecordProgress(step, progress, message)
	srvLog.Info("package install progress", "jobID", job.ID, "percent", progress, "step", step, "message", message)
	progressSaverFor(job.ID).recordStep(ProgressStep{Step: step, Progress: progress, Message: message})
}

func emitStepProgress(job *installjob.Job, step string, progress, stepProgress int, message string) {
	if job == nil {
		return
	}
	job.RecordStepProgress(step, progress, stepProgress, message)
	srvLog.Info("package install progress", "jobID", job.ID, "percent", progress, "step", step, "stepPercent", stepProgress, "message", message)
	sp := stepProgress
	progressSaverFor(job.ID).recordStep(ProgressStep{Step: step, Progress: progress, Message: message, StepProgress: &sp})
}

func emitComplete(job *installjob.Job, status string, errMsg string) {
	if errMsg != "" {
		srvLog.Info("package install complete", "jobID", job.ID, "status", status, "err", errMsg)
	} else {
		srvLog.Info("package install complete", "jobID", job.ID, "status", status)
	}
	job.RecordComplete(status, errMsg)
}

// ---------- install log helpers ----------

func createInstallLog(app core.App, job *installjob.Job, action string) *core.Record {
	collection, err := app.FindCollectionByNameOrId("pkg_install_log")
	if err != nil {
		srvLog.Error("failed to find pkg_install_log collection", "err", err)
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
	record.Set("job_id", job.ID)
	record.Set("npm_package", job.NpmPkg)
	record.Set("status", "running")
	record.Set("started_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))

	trigger := job.Trigger
	if trigger == "" {
		trigger = "manual"
	}
	record.Set("trigger", trigger)
	if len(job.Changes) > 0 {
		record.Set("changes", job.Changes)
	}

	if err := app.Save(record); err != nil {
		srvLog.Error("failed to create install log", "err", err)
		return nil
	}

	// Wire this job's progress onto its own row: emitProgress/emitStepProgress
	// only have the job (threading app + the record through their ~15 call
	// sites would be far more invasive), so they look the saver up by job id.
	registerProgressSaver(app, job.ID, record)

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
		srvLog.Warn("failed to update install log slug", "recordID", record.Id, "slug", slug, "err", err)
	}
}

// tagInstallLog saves the build id on the job's install-log row. It sets it
// on the record the rebuild holds, not on a fresh copy found by job_id: the
// finalize saves that same record later, and a copy without the field would
// write it back empty.
func tagInstallLog(app core.App, record *core.Record, buildID string) error {
	if record == nil {
		return nil
	}
	record.Set("build_id", buildID)
	if err := app.Save(record); err != nil {
		return fmt.Errorf("coreserver: save build id on install log %s: %w", record.Id, err)
	}
	return nil
}

func finalizeInstallLog(app core.App, record *core.Record, status string, errMsg string, logLines []string) {
	if record == nil {
		return
	}

	// Flush any throttled-away progress BEFORE setting the terminal fields on
	// the same record, so the save below carries the last step too — not just
	// whatever the throttle had already written — and so no later flush can
	// race this save and overwrite the terminal status back to "running".
	if jobID := record.GetString("job_id"); jobID != "" {
		progressSaverFor(jobID).flush()
		unregisterProgressSaver(jobID)
	}

	record.Set("status", status)
	record.Set("error", errMsg)
	record.Set("log", strings.Join(logLines, "\n"))
	record.Set("completed_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))

	if err := app.Save(record); err != nil {
		srvLog.Error("failed to finalize install log", "recordID", record.Id, "status", status, "err", err)
		return
	}
	srvLog.Info("finalized install log", "recordID", record.Id, "status", status)
}

// ---------- release staging ----------

// ---------- pkg_registry helpers ----------

func upsertPkgRegistry(app core.App, m *parsedManifest, npmPkg string, manifestJSON []byte) error {
	// nav is optional — a slot-only / settings-only contributor declares none, so
	// it has no nav-rail entry. Map a missing nav to an empty icon + order 0.
	navIcon, navOrder := "", 0
	if m.Nav != nil {
		navIcon, navOrder = m.Nav.Icon, m.Nav.Order
	}

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
		existing.Set("icon", navIcon)
		existing.Set("has_server", m.HasServer)
		// Preserve bundled status across an in-app version change: a bundled
		// feature that's been upgraded is still bundled (so the uninstall guard
		// keeps blocking it), only its version moved. Any other prior status
		// (available → being installed) promotes to installed as before.
		if existing.GetString("status") != "bundled" {
			existing.Set("status", "installed")
		}
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
	record.Set("icon", navIcon)
	record.Set("has_server", m.HasServer)
	record.Set("status", "installed")
	record.Set("manifest_json", json.RawMessage(manifestJSON))
	record.Set("nav_order", navOrder)
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

// runCmd/runCmdEnv/runCmdStreaming/copyDir moved to pkgbuild (the exec seam);
// coreserver reaches them through the delegates in pkgbuild_glue.go.

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

// automationDefsFile is the materialized automation catalog the generator
// writes (scripts/gen-automation.ts).
const automationDefsFile = "automation_defs.json"

// automationDefsPath is where the generator wrote automation_defs.json for
// the binary whose dir is serverDir (resolveServerDir()). The generator
// writes into the app's server/ dir (SERVER_DIR in scripts/paths.ts). The
// image, bare metal and the in-app rebuild put the binary one level above
// that dir, at <app>/tinycld; dev builds it into <app>/server/ itself. So the
// file is in serverDir/server/ when it is there, and in serverDir otherwise.
func automationDefsPath(serverDir string) string {
	nested := filepath.Join(serverDir, "server", automationDefsFile)
	if _, err := os.Stat(nested); err == nil {
		return nested
	}
	return filepath.Join(serverDir, automationDefsFile)
}
