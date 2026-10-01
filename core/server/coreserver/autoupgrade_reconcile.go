package coreserver

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/installjob"
)

const rolledBackReason = "The update did not pass its health check after the restart, so it was rolled back."

// reconcileAutoUpgradeResults runs at boot, after ReconcileRolledBackInstall
// has marked a stranded row rolled_back. Each rolled-back automatic job blocks
// its set so the next window does not try it again. A failed job is not
// blocked: it failed before the live app changed, so trying again is safe.
func reconcileAutoUpgradeResults(app core.App, now time.Time, notify func(notice)) {
	rows, err := app.FindRecordsByFilter("pkg_install_log",
		"trigger = 'auto' && status = 'rolled_back'", "-created", 0, 0)
	if err != nil {
		srvLog.Warn("auto-upgrade: reconcile query failed", "err", err)
		return
	}
	for _, r := range rows {
		var changes []installjob.VersionChange
		if err := r.UnmarshalJSONField("changes", &changes); err != nil || len(changes) == 0 {
			continue
		}
		target := make(map[string]string, len(changes))
		for _, c := range changes {
			target[c.Slug] = c.TargetVersion
		}
		if err := recordBlocked(app, fingerprint(target, nil), target, rolledBackReason, r.Id, now, notify); err != nil {
			srvLog.Warn("auto-upgrade: record blocked failed", "installLog", r.Id, "err", err)
		}
	}
}
