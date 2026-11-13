package classifier

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

func monthKey(now time.Time) string { return now.UTC().Format("2006-01") }

func usageRow(app core.App, now time.Time) (*core.Record, error) {
	return app.FindFirstRecordByFilter("classifier_usage", "month = {:m}", map[string]any{"m": monthKey(now)})
}

// MonthSpend is the month's recorded spend and call count.
func MonthSpend(app core.App, now time.Time) (float64, int) {
	cost, calls, _ := monthSpend(app, now)
	return cost, calls
}

// monthSpend separates "no row yet" (zero spend) from a real lookup error.
func monthSpend(app core.App, now time.Time) (float64, int, error) {
	rec, err := usageRow(app, now)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return rec.GetFloat("cost_usd"), rec.GetInt("calls"), nil
}

// checkBudget refuses a call whose estimate would take the month past the
// cap. Concurrent jobs can each pass before either records its cost, so the
// cap is soft by at most one call per running job.
func checkBudget(app core.App, cfg Config, estimate float64, now time.Time) error {
	spent, _, err := monthSpend(app, now)
	if err != nil {
		return err
	}
	if spent+estimate > cfg.Budget {
		return ErrBudgetExceeded
	}
	return nil
}

func addUsage(app core.App, cost float64, now time.Time) error {
	return app.RunInTransaction(func(tx core.App) error {
		rec, err := usageRow(tx, now)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err != nil {
			col, cerr := tx.FindCollectionByNameOrId("classifier_usage")
			if cerr != nil {
				return fmt.Errorf("classifier_usage: %w", cerr)
			}
			rec = core.NewRecord(col)
			rec.Set("month", monthKey(now))
		}
		rec.Set("cost_usd", rec.GetFloat("cost_usd")+cost)
		rec.Set("calls", rec.GetInt("calls")+1)
		return tx.Save(rec)
	})
}
