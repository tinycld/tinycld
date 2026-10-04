package coreserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/installjob"
)

// legacyRollbackMarkerPath is where a supervisor before the state-dir
// record left its note: the failed build id as plain text, in pb_data. It is
// still read once, so a rollback recorded just before an upgrade is not lost.
func legacyRollbackMarkerPath() string {
	return filepath.Join(statePbDataDir(), ".rollback-pending")
}

// readRollbackRecord returns the build the supervisor rolled back from and
// the file that recorded it. The state-dir record wins; the legacy pb_data
// marker is the fallback. ok is false when neither exists.
func readRollbackRecord() (build, path string, ok bool) {
	path = stateRollbackRecordPath()
	if data, err := os.ReadFile(path); err == nil {
		var r struct {
			Build string `json:"build"`
		}
		if jErr := json.Unmarshal(data, &r); jErr != nil {
			// A rollback still happened; the row is marked without a build.
			srvLog.Warn("the rollback record does not parse; marking the install without its build", "path", path, "err", jErr)
		}
		return strings.TrimSpace(r.Build), path, true
	}
	path = legacyRollbackMarkerPath()
	if data, err := os.ReadFile(path); err == nil {
		return strings.TrimSpace(string(data)), path, true
	}
	return "", "", false
}

// rolledBackError is the error a rolled-back build's install-log rows carry.
const rolledBackError = "the build failed its health check and was rolled back"

// ReconcileRolledBackInstall runs at boot (OnServe, before serving). When the
// supervisor rolled a build back, it left a rollback record
// (readRollbackRecord) naming that build. Every install-log row the build
// produced is then marked "rolled_back", whatever state the live database
// holds it in:
//   - "running" when the supervisor restored the pre-install snapshot (taken
//     while the row was running, so the finalize was discarded);
//   - "success" when the finalized database stayed live: a restore whose swap
//     was rolled back, or a backup the supervisor could not restore.
//
// A row left at "success" would tell the admin screens the install worked
// and would let an automatic upgrade of the same set be tried again
// (reconcileAutoUpgradeResults blocks only rolled_back rows).
//
// Rows written before the build_id field cannot be matched by build; for
// those the newest "running" row without a build id is the one the rollback
// stranded, since the installer is single-flight (core/installjob).
//
// The record is removed only after every write succeeded, so a failure
// retries on the next boot. Rows already "rolled_back" or "failed" are never
// touched, so a retry is harmless.
func ReconcileRolledBackInstall(app core.App) {
	build, recordPath, ok := readRollbackRecord()
	if !ok {
		return
	}

	// Never mark a row out from under a running operation. A fresh boot has
	// no job, so this only guards a later caller.
	if installjob.Running() {
		srvLog.Info("a rollback record is present but a job is in flight; deferring the reconcile")
		return
	}

	if _, cErr := app.FindCollectionByNameOrId("pkg_install_log"); cErr != nil {
		return // migration not applied yet — keep the record for a later boot
	}

	rows, fErr := rolledBackInstallRows(app, build)
	if fErr != nil {
		srvLog.Warn("query for the rolled-back build's install-log rows failed, retrying next boot", "build", build, "err", fErr)
		return
	}

	completed := time.Now().UTC().Format("2006-01-02 15:04:05.000Z")
	for _, row := range rows {
		row.Set("status", "rolled_back")
		row.Set("error", rolledBackError)
		row.Set("completed_at", completed)
		if sErr := app.Save(row); sErr != nil {
			srvLog.Warn("failed to mark install-log rolled_back, retrying next boot", "recordID", row.Id, "err", sErr)
			return
		}
		srvLog.Info("marked install-log rolled_back", "recordID", row.Id, "pkgSlug", row.GetString("pkg_slug"), "build", build)
	}

	if rmErr := os.Remove(recordPath); rmErr != nil && !os.IsNotExist(rmErr) {
		srvLog.Warn("failed to clear the rollback record", "path", recordPath, "err", rmErr)
	}
}

// rolledBackInstallRows returns the install-log rows the rolled-back build
// produced that are still running or success: matched by build_id, or, when
// none match, the newest running row written before the build_id field.
//
// A revert re-activates a retained build, so an older row can carry the same
// build id. Only the rows since the newest row of another build belong to
// the run that was rolled back; the earlier ones record a run that worked.
func rolledBackInstallRows(app core.App, build string) ([]*core.Record, error) {
	if build != "" {
		filter := "build_id = {:b} && (status = 'running' || status = 'success')"
		params := dbx.Params{"b": build}
		prior, err := app.FindRecordsByFilter("pkg_install_log",
			"build_id != '' && build_id != {:b}", "-created", 1, 0, params)
		if err != nil {
			return nil, err
		}
		if len(prior) > 0 {
			filter += " && created >= {:since}"
			params["since"] = prior[0].GetDateTime("created").String()
		}
		rows, err := app.FindRecordsByFilter("pkg_install_log", filter, "-created", 0, 0, params)
		if err != nil || len(rows) > 0 {
			return rows, err
		}
	}
	return app.FindRecordsByFilter(
		"pkg_install_log",
		"status = 'running' && build_id = ''",
		"-created",
		1,
		0,
	)
}
