package webforms

import (
	"errors"
	"fmt"
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
	var errs []error
	for _, pub := range pubs {
		cutoff, _ := types.ParseDateTime(now.Add(-time.Duration(pub.GetInt("retention_days")) * 24 * time.Hour))
		// Submissions that failed to delete stay in the query's results, so
		// each batch reads past them and skips them.
		failed := map[string]bool{}
		for {
			limit := pruneBatch + len(failed)
			old, err := app.FindRecordsByFilter(submissionsName, "publication = {:p} && created < {:c}", "created", limit, 0,
				map[string]any{"p": pub.Id, "c": cutoff.String()})
			if err != nil {
				errs = append(errs, err)
				break
			}
			progressed := false
			for _, rec := range old {
				if failed[rec.Id] {
					continue
				}
				// One submission that cannot go must not keep every other
				// publication's old submissions past their retention.
				if err := app.Delete(rec); err != nil {
					failed[rec.Id] = true
					errs = append(errs, fmt.Errorf("delete submission %s: %w", rec.Id, err))
					continue
				}
				deleted++
				progressed = true
			}
			if !progressed || len(old) < limit {
				break
			}
		}
	}
	return deleted, errors.Join(errs...)
}

func scheduleRetention(app core.App) {
	// Add, not MustAdd: a second Register on the same app replaces the job.
	if err := app.Cron().Add(retentionJobID, retentionSchedule, func() {
		n, err := pruneSubmissions(app, time.Now())
		if err != nil {
			log.Error("prune web form submissions", "deleted", n, "err", err)
		}
		if n > 0 {
			log.Info("pruned web form submissions", "count", n)
		}
	}); err != nil {
		log.Error("schedule web form retention", "err", err)
	}
}
