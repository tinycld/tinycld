package coreserver

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/readonly"
)

// This file replaces the SSE event stream as the way a client learns an
// install/upgrade job's progress. The stream dies at the server restart that
// ends every successful apply (the new process has no in-memory job), which
// forced the client onto a token-in-the-URL EventSource plus a durable-poll
// fallback for the restart seam — leaking an admin credential into server
// logs. Saving progress onto the pkg_install_log row instead means a client
// watching the row with a live query gets the same update through one path,
// before AND after the restart (pbtsdb reconnects and reloads live queries
// once the new process is back).
//
// ProgressStep is one headline milestone — NOT pkgbuild's detail log lines
// (those stay in `log`, written once at finalize). Keeping only milestones on
// the live row bounds its size regardless of how chatty a build gets.
type ProgressStep struct {
	Step         string `json:"step"`
	Progress     int    `json:"progress"`
	Message      string `json:"message"`
	StepProgress *int   `json:"stepProgress,omitempty"`
}

// progressThrottle is ~1 save/sec — frequent enough that a client watching
// the row sees smooth progress, far below PocketBase's write capacity for a
// single row, and low enough that a build emitting dozens of milestones a
// second (the fast early assemble steps) doesn't turn into dozens of writes.
const progressThrottle = time.Second

// installLogProgressSaver throttles a running job's progress writes onto its
// pkg_install_log row. One instance per job, registered at createInstallLog
// and looked up by emitProgress/emitStepProgress, which only have the job —
// threading app + the record through every one of their ~15 call sites
// would be far more invasive than a small per-job registry.
type installLogProgressSaver struct {
	app    core.App
	record *core.Record

	mu       sync.Mutex
	steps    []ProgressStep
	lastSave time.Time
	pending  bool // a step arrived since the last save and was dropped by the throttle
}

var (
	saversMu sync.Mutex
	savers   = map[string]*installLogProgressSaver{}
)

// registerProgressSaver wires a job id to the row its progress is saved onto.
// Safe to call with a nil record (createInstallLog already logs that failure);
// the saver then simply no-ops.
func registerProgressSaver(app core.App, jobID string, record *core.Record) {
	saversMu.Lock()
	defer saversMu.Unlock()
	savers[jobID] = &installLogProgressSaver{app: app, record: record}
}

// unregisterProgressSaver drops a finished job's saver. Called from
// finalizeInstallLog's caller via finishJob so the map does not grow for the
// life of the process.
func unregisterProgressSaver(jobID string) {
	saversMu.Lock()
	defer saversMu.Unlock()
	delete(savers, jobID)
}

func progressSaverFor(jobID string) *installLogProgressSaver {
	saversMu.Lock()
	defer saversMu.Unlock()
	return savers[jobID]
}

// recordStep appends step to the saver's in-memory history and persists it to
// the row, throttled to roughly one write per second. The LAST step of a burst
// within the throttle window is saved on the NEXT tick (flushPending), so a
// client never sees progress freeze at a stale percentage between ticks.
//
// Never blocks and never fails the job: a save is skipped outright while
// read-only mode is on (a second process is about to migrate this database —
// see package readonly) rather than queued or retried, because the row's
// schema itself may be mid-migration. The next tick after the mode lifts
// picks up wherever progress is by then; a few seconds of missed live rows
// cost nothing, since `log` still carries the full history at finalize and
// the final status always lands on the row synchronously once writes resume.
func (s *installLogProgressSaver) recordStep(step ProgressStep) {
	if s == nil || s.record == nil {
		return
	}
	s.mu.Lock()
	s.steps = append(s.steps, step)
	due := time.Since(s.lastSave) >= progressThrottle
	s.mu.Unlock()

	if !due {
		s.mu.Lock()
		s.pending = true
		s.mu.Unlock()
		return
	}
	s.flush()
}

// flush saves the current step history + latest headline to the row right
// now, bypassing the throttle. The install pipeline calls it once after the
// last emitProgress of a run so the terminal row the client sees reflects the
// very last milestone, not a throttled-away one.
func (s *installLogProgressSaver) flush() {
	if s == nil || s.record == nil || readonly.Active() {
		return
	}
	s.mu.Lock()
	steps := append([]ProgressStep{}, s.steps...)
	s.lastSave = time.Now()
	s.pending = false
	s.mu.Unlock()

	if len(steps) == 0 {
		return
	}
	latest := steps[len(steps)-1]
	s.record.Set("steps", steps)
	s.record.Set("current_step", latest.Step)
	s.record.Set("current_message", latest.Message)
	if err := s.app.Save(s.record); err != nil {
		// Best-effort: progress is a convenience, not the job's outcome. Log
		// and move on rather than failing the install over a UI nicety.
		srvLog.Warn("failed to save install progress", "recordID", s.record.Id, "err", err)
	}
}
