package backup

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// The manual-run ceiling is a seam an embedder claims once; a deployment
// nobody claims uses the default. Scheduled runs never count: a ceiling is
// there to stop a person clicking Back up now forty times, not to stop the
// schedule the same person configured.
var (
	limitMu      sync.Mutex
	limitFn      func(core.App) int
	limitClaimed bool
	manualRuns   []time.Time
)

const defaultDailyLimit = 10

// SetDailyLimit claims the manual-run ceiling. The first caller wins, so an
// embedder cannot have its ceiling replaced by a later import.
func SetDailyLimit(fn func(app core.App) int) {
	limitMu.Lock()
	defer limitMu.Unlock()
	if limitClaimed || fn == nil {
		return
	}
	limitFn, limitClaimed = fn, true
}

func resetDailyLimitForTesting() {
	limitMu.Lock()
	defer limitMu.Unlock()
	limitFn, limitClaimed, manualRuns = nil, false, nil
}

func dailyLimit(app core.App) int {
	limitMu.Lock()
	fn := limitFn
	limitMu.Unlock()
	if fn == nil {
		return defaultDailyLimit
	}
	return fn(app)
}

// allowManual records the run when it is under the ceiling. A limit of zero or
// less is no ceiling at all.
func allowManual(app core.App) bool {
	limit := dailyLimit(app)
	limitMu.Lock()
	defer limitMu.Unlock()
	cutoff := time.Now().Add(-24 * time.Hour)
	kept := manualRuns[:0]
	for _, t := range manualRuns {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	manualRuns = kept
	if limit > 0 && len(manualRuns) >= limit {
		return false
	}
	manualRuns = append(manualRuns, time.Now())
	return true
}
