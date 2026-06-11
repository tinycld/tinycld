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

		// All admin endpoints accept a PB superuser OR a super-admin app user.
		adminGuard := func(re *core.RequestEvent) error {
			return requireAdmin(app, re)
		}

		g.POST("/install", func(re *core.RequestEvent) error {
			return handleInstall(app, re)
		}).BindFunc(adminGuard)

		g.POST("/uninstall", func(re *core.RequestEvent) error {
			return handleUninstall(app, re)
		}).BindFunc(adminGuard)

		g.POST("/revert", func(re *core.RequestEvent) error {
			return handleRevert(app, re)
		}).BindFunc(adminGuard)

		g.POST("/builds/delete", func(re *core.RequestEvent) error {
			return handleDeleteBuild(app, re)
		}).BindFunc(adminGuard)

		g.GET("/events/{jobId}", func(re *core.RequestEvent) error {
			return handleEvents(re)
		}).BindFunc(func(re *core.RequestEvent) error {
			return requireSuperuserOrToken(app, re)
		})

		g.GET("/status/{slug}", func(re *core.RequestEvent) error {
			return handleStatus(app, re)
		}).BindFunc(adminGuard)

		g.GET("/versions", func(re *core.RequestEvent) error {
			return handleVersions(app, re)
		}).BindFunc(adminGuard)

		g.POST("/versions/check", func(re *core.RequestEvent) error {
			return handleVersionsCheck(app, re)
		}).BindFunc(adminGuard)

		g.POST("/versions/drop-report", func(re *core.RequestEvent) error {
			return handleDropReport(app, re)
		}).BindFunc(adminGuard)

		g.POST("/versions/apply", func(re *core.RequestEvent) error {
			return handleVersionChange(app, re)
		}).BindFunc(adminGuard)

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

// isSuperAdmin reports whether the given app-user id is listed in the
// super_admins junction — i.e. a regular user granted cross-org admin powers.
// PB superusers are NOT in this table (they're authorized separately via
// IsSuperuser); the two paths are unioned by the guards below.
func isSuperAdmin(app core.App, userID string) bool {
	if userID == "" {
		return false
	}
	_, err := app.FindFirstRecordByFilter(
		"super_admins",
		"user = {:user}",
		map[string]any{"user": userID},
	)
	return err == nil
}

// requireAdmin authorizes either a PB superuser OR an app user listed in
// super_admins. Takes core.App so the super_admins lookup is unit-testable.
func requireAdmin(app core.App, re *core.RequestEvent) error {
	if re.HasSuperuserAuth() {
		return re.Next()
	}
	if re.Auth != nil && isSuperAdmin(app, re.Auth.Id) {
		return re.Next()
	}
	return re.ForbiddenError("Admin access required", nil)
}

// requireSuperuserOrToken allows SSE connections to authenticate via query param
// since browser EventSource does not support custom headers. Takes core.App (not
// *pocketbase.PocketBase) so it's unit-testable against tests.TestApp — it only
// needs FindAuthRecordByToken, a core.App method.
func requireSuperuserOrToken(app core.App, re *core.RequestEvent) error {
	// Try standard auth first — a PB superuser or a super-admin app user.
	if re.HasSuperuserAuth() {
		return re.Next()
	}
	if re.Auth != nil && isSuperAdmin(app, re.Auth.Id) {
		return re.Next()
	}

	// Fall back to query param token for SSE. FindAuthRecordByToken's variadic
	// arg is the token TYPE (core.TokenTypeAuth), not a collection id — passing a
	// collection id makes it match no valid type and reject every token (the 403
	// the install progress stream hit). Validate as an auth token, then accept it
	// only if the record is a superuser or a super-admin app user — a plain
	// user's token must not pass.
	token := re.Request.URL.Query().Get("token")
	if token != "" {
		record, err := app.FindAuthRecordByToken(token, core.TokenTypeAuth)
		if err == nil && record != nil && (record.IsSuperuser() || isSuperAdmin(app, record.Id)) {
			return re.Next()
		}
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

	go runInstallRebuild(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": jobId})
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

	go runUninstallRebuild(app, job)

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
		return
	}
	log.Printf("pkg_install: finalized install log %s -> %s", record.Id, status)
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

