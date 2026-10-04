// Package autoupgrade is the seam through which a deployment decides how its
// packages are upgraded automatically. Core stores the owner's choice and shows
// the result; the Delegate does the work. A deployment that can rebuild itself
// installs a local scheduler; a composing server that owns this app's lifecycle
// installs its own Delegate and controls the cadence. Core does not know which.
//
// Like syscfg, it imports nothing from coreserver, so any package can read it.
package autoupgrade

import (
	"context"
	"sync"
	"time"
)

const (
	KeyEnabled    = "autoupgrade.enabled"
	KeyWindow     = "autoupgrade.window"
	DefaultWindow = "02:00-05:00"
)

// Status is what the Packages page shows. The pause and the blocked sets are
// rows the page reads itself; this carries only computed values.
type Status struct {
	Available  bool      `json:"available"`
	Reason     string    `json:"reason,omitempty"`
	LastRun    time.Time `json:"lastRun"`
	LastResult string    `json:"lastResult"`
	NextCheck  time.Time `json:"nextCheck"`
}

type Delegate interface {
	// PolicyChanged is called once at boot and on every save or delete of the
	// flag, including a save that does not change its value. It must be
	// idempotent: the same value may arrive many times in a row.
	PolicyChanged(ctx context.Context, enabled bool) error
	Status(ctx context.Context) (Status, error)
}

// Starter is implemented by a Delegate that runs its own loop.
type Starter interface {
	Start(ctx context.Context)
}

var (
	mu      sync.RWMutex
	current Delegate
)

func SetDelegate(d Delegate) {
	mu.Lock()
	defer mu.Unlock()
	current = d
}

// Current returns the installed Delegate, or nil when this build has none.
func Current() Delegate {
	mu.RLock()
	defer mu.RUnlock()
	return current
}

func Unavailable(reason string) Status {
	return Status{Available: false, Reason: reason}
}
