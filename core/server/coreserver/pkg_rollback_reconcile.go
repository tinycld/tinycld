package coreserver

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// rollbackPendingMarkerPath is the breadcrumb the entrypoint's rollback path
// writes (config/entrypoint.sh's write_rollback_pending) and this reconciler
// consumes. It lives under pb_data so it survives the per-build symlink swap and
// a crash in the rollback window. Its contents are the rolled-back build id (may
// be empty if the arm marker was unreadable). Keep this path in sync with the
// entrypoint's $ROLLBACK_PENDING_MARKER.
func rollbackPendingMarkerPath() string {
	return filepath.Join(statePbDataDir(), ".rollback-pending")
}

// ReconcileRolledBackInstall runs at boot (OnServe, before serving). When the
// entrypoint health-check rolled back the previous install, it left a
// .rollback-pending breadcrumb; the DB it restored is the PRE-install snapshot,
// taken (rebuild.go backupDB) while that install's pkg_install_log row was still
// "running" — so the later finalize("success") write was discarded by the
// restore and the row is stranded at "running" with no completed_at. There is no
// in-process job on a fresh boot (currentJob is nil), so nothing else will ever
// finalize it.
//
// This marks the stranded row "rolled_back" so the /admin status endpoint (and
// the integration test's waitForOpStatus / waitForRolledBack) sees a clean
// terminal state instead of a row stuck at "running" forever.
//
// Idempotent + safe:
//   - No marker  → no-op (the normal, healthy-boot case; the entrypoint commit
//     path never writes the marker, so a committed build is never touched here).
//   - Marker present but no "running" row → delete the marker, no-op (already
//     reconciled, or the rollback predated any log write).
//   - A concurrent in-flight job cannot exist on a fresh boot, but we guard on
//     currentJob == nil anyway so a future caller can't clobber a live install.
//
// It only ever transitions running → rolled_back; it never touches success,
// failed, or already-rolled_back rows.
func ReconcileRolledBackInstall(app core.App) {
	markerPath := rollbackPendingMarkerPath()
	data, err := os.ReadFile(markerPath)
	if err != nil {
		return // no breadcrumb → nothing was rolled back → no-op
	}
	rolledBackBuild := strings.TrimSpace(string(data))

	// A fresh boot has no in-memory job. Never reconcile a row out from under a
	// genuinely running operation (belt-and-suspenders; can't happen on boot).
	installMu.Lock()
	live := currentJob != nil
	installMu.Unlock()
	if live {
		log.Printf("pkg_rollback: .rollback-pending present but a job is in-flight; deferring reconcile")
		return
	}

	if _, cErr := app.FindCollectionByNameOrId("pkg_install_log"); cErr != nil {
		return // migration not applied yet — leave the marker for a later boot
	}

	// The stranded row is the single most-recent install-class row still at
	// "running" (its finalize was discarded by the restore). There is at most one
	// — the installer is single-flight (installMu/currentJob).
	rows, fErr := app.FindRecordsByFilter(
		"pkg_install_log",
		"status = 'running'",
		"-created",
		1,
		0,
	)
	if fErr != nil {
		log.Printf("pkg_rollback: query stranded running row failed: %v", fErr)
		return // keep the marker; retry next boot rather than lose the signal
	}
	if len(rows) == 0 {
		// Nothing stranded (e.g. the rollback happened before any log write, or a
		// prior boot already reconciled). Drop the consumed breadcrumb.
		_ = os.Remove(markerPath)
		return
	}

	row := rows[0]
	msg := "package install rolled back: new build failed its post-restart health check"
	if rolledBackBuild != "" {
		msg = "package install rolled back (build " + rolledBackBuild +
			"): new build failed its post-restart health check"
	}
	row.Set("status", "rolled_back")
	row.Set("error", msg)
	row.Set("completed_at", time.Now().UTC().Format("2006-01-02 15:04:05.000Z"))
	if sErr := app.Save(row); sErr != nil {
		log.Printf("pkg_rollback: failed to mark install-log %s rolled_back: %v", row.Id, sErr)
		return // keep the marker; retry next boot
	}
	log.Printf("pkg_rollback: marked install-log %s (pkg=%s) rolled_back",
		row.Id, row.GetString("pkg_slug"))

	// Consume the breadcrumb only after a successful write so a transient failure
	// retries on the next boot.
	if rmErr := os.Remove(markerPath); rmErr != nil && !os.IsNotExist(rmErr) {
		log.Printf("pkg_rollback: WARNING: failed to clear %s: %v", markerPath, rmErr)
	}
}
