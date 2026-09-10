package coreserver

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/installjob"
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
	// buildId comes straight from the request and is joined into filesystem
	// paths (and used to repoint the live `current` symlink) by the revert
	// pipeline. Constrain it to the only shapes the install pipeline mints so a
	// `../…` value can't escape the builds dir.
	if !buildIDPattern.MatchString(body.BuildID) {
		return re.BadRequestError("Invalid buildId", nil)
	}

	job := installjob.New("revert", "", "")
	job.BuildID = body.BuildID
	if busy, ok := installjob.Claim(job); !ok {
		return re.JSON(http.StatusConflict, map[string]any{
			"error":      "Another operation is in progress",
			"currentJob": busy.Info(),
		})
	}

	go runRevertRebuild(app, job)

	return re.JSON(http.StatusAccepted, map[string]any{"jobId": job.ID})
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
	// Validated before buildArchiveFor joins it into a path that gets RemoveAll'd
	// — a `../…` value must not be able to delete a tree outside builds/.
	if !buildIDPattern.MatchString(body.BuildID) {
		return re.BadRequestError("Invalid buildId", nil)
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
