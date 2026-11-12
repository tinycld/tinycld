package coreserver

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// The batch API runs several record writes in one database transaction; the
// client's atomic() mutations depend on it. A batch holds the only write
// connection until it ends, so the timeout is also the longest time one batch
// can block every other write.
const (
	batchMaxRequests = 1100
	batchTimeoutSecs = 30
	batchMaxBodySize = 8 << 20
)

// RegisterBatchSettings applies the batch settings on every start, so a
// deployment cannot drift from them: an atomic mutation against a server with
// batch off fails for every user. A start that cannot store them fails.
func RegisterBatchSettings(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := applyBatchSettings(e.App); err != nil {
			return err
		}
		return e.Next()
	})
}

func applyBatchSettings(app core.App) error {
	want := core.BatchConfig{
		Enabled:     true,
		MaxRequests: batchMaxRequests,
		Timeout:     batchTimeoutSecs,
		MaxBodySize: batchMaxBodySize,
	}
	settings := app.Settings()
	if settings.Batch == want {
		return nil
	}
	settings.Batch = want
	if err := app.Save(settings); err != nil {
		return fmt.Errorf("apply batch settings: %w", err)
	}
	return nil
}
