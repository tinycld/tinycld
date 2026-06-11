package coreserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

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

	go runRevertRebuild(app, job)

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
