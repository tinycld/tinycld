package webforms

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const (
	retentionJobID    = "webforms-retention"
	retentionSchedule = "30 3 * * *"
	pruneBatch        = 200
)

// pruneSubmissions deletes each publication's submissions older than its
// retention_days. Their files cascade; cards and rows rules made stay.
func pruneSubmissions(app core.App, now time.Time) (int, error) {
	pubs, err := app.FindAllRecords(publicationsName)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, pub := range pubs {
		cutoff, _ := types.ParseDateTime(now.Add(-time.Duration(pub.GetInt("retention_days")) * 24 * time.Hour))
		for {
			old, err := app.FindRecordsByFilter(submissionsName, "publication = {:p} && created < {:c}", "created", pruneBatch, 0,
				map[string]any{"p": pub.Id, "c": cutoff.String()})
			if err != nil {
				return deleted, err
			}
			for _, rec := range old {
				if err := app.Delete(rec); err != nil {
					return deleted, err
				}
				deleted++
			}
			if len(old) < pruneBatch {
				break
			}
		}
	}
	return deleted, nil
}

func scheduleRetention(app core.App) {
	// Add, not MustAdd: a second Register on the same app replaces the job.
	if err := app.Cron().Add(retentionJobID, retentionSchedule, func() {
		n, err := pruneSubmissions(app, time.Now())
		if err != nil {
			log.Error("prune web form submissions", "err", err)
			return
		}
		if n > 0 {
			log.Info("pruned web form submissions", "count", n)
		}
	}); err != nil {
		log.Error("schedule web form retention", "err", err)
	}
}
