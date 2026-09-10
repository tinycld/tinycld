// Package installjob owns the process-wide "one package operation at a time"
// interlock and the job it tracks.
//
// It exists as its own package because MORE THAN ONE composition serves the
// package-install API: the deployment that rebuilds itself in process, and one
// whose deploys ride a supervisor's control socket. Both must see the SAME
// running job — the interlock is what stops two operations mutating one data
// dir at once — and Go package-level state cannot be shared across packages.
// Left in either composition's own package, the other would get a second,
// empty registry and the interlock would silently do nothing.
//
// The mutex is deliberately NOT exported. Callers claim, read, or release
// through the functions here, so there is no way to hold the lock across a
// caller's own work and no second lock ordering to reason about.
package installjob

import (
	"fmt"
	"sync"
	"time"
)

// Event is one server-sent event on a job's progress stream.
type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// ProgressData is the payload of a "progress" Event.
type ProgressData struct {
	Step     string `json:"step"`
	Progress int    `json:"progress"`
	Message  string `json:"message"`
}

// CompleteData is the payload of a "complete" Event.
type CompleteData struct {
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// VersionChange is one {slug → target version} entry of a version_change
// operation. It lives here because Job carries the ordered set.
type VersionChange struct {
	Slug          string `json:"slug"`
	TargetVersion string `json:"targetVersion"`
}

// Job is one package operation. Its exported fields are set by the composition
// that claimed it; the listener set and the fields Progress/Step/LogLines are
// mutated only through this package's methods, which hold the per-job lock.
type Job struct {
	ID      string
	Action  string // "install", "uninstall", "revert", or "version_change"
	Slug    string
	NpmPkg  string
	BuildID string // revert target (action == "revert")
	// Changes is the ordered set a version_change applies together.
	Changes  []VersionChange
	Progress int
	Step     string
	Status   string // "running", "success", "failed", "rolled_back"
	Error    string
	LogLines []string
	Done     chan struct{}

	mu        sync.Mutex
	listeners []chan Event
}

// ---------- the interlock ----------

var (
	mu      sync.Mutex
	current *Job
)

// New mints a job with a timestamp-derived id and an open Done channel. It does
// NOT claim the interlock — pass it to Claim.
func New(action, slug, npmPkg string) *Job {
	return &Job{
		ID:     fmt.Sprintf("job_%d", time.Now().UnixMilli()),
		Action: action,
		Slug:   slug,
		NpmPkg: npmPkg,
		Status: "running",
		Done:   make(chan struct{}),
	}
}

// Claim installs job as the running one, or reports the job already running.
// The check and the set happen under one lock, so two callers racing cannot
// both believe they won.
func Claim(job *Job) (busy *Job, ok bool) {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		return current, false
	}
	current = job
	return nil, true
}

// Current reports the running job, or nil.
func Current() *Job {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// Running reports whether any job holds the interlock.
func Running() bool {
	mu.Lock()
	defer mu.Unlock()
	return current != nil
}

// Release clears the interlock. Safe to call on a job that no longer holds it
// (a late unwind after a restart), which is why it compares before clearing.
func Release(job *Job) {
	mu.Lock()
	defer mu.Unlock()
	if current == job {
		current = nil
	}
}

// Info is the shape the API returns for a busy interlock.
func (j *Job) Info() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	return map[string]any{
		"jobId":  j.ID,
		"action": j.Action,
		"slug":   j.Slug,
		"status": j.Status,
	}
}

// ---------- per-job state ----------

// Subscribe adds a listener for this job's events and returns it. The channel
// is buffered; Emit drops rather than blocks, so a slow client cannot stall a
// running install.
func (j *Job) Subscribe(buffer int) chan Event {
	ch := make(chan Event, buffer)
	j.mu.Lock()
	defer j.mu.Unlock()
	j.listeners = append(j.listeners, ch)
	return ch
}

// SubscribeWithHistory adds a listener AND returns the job's state as of the
// same instant, under one lock. A late-connecting client needs both: the
// backlog it missed and the live stream, with no window between them where an
// event could be recorded into neither.
func (j *Job) SubscribeWithHistory(buffer int) (ch chan Event, history []string, status, errMsg string) {
	ch = make(chan Event, buffer)
	j.mu.Lock()
	defer j.mu.Unlock()
	j.listeners = append(j.listeners, ch)
	history = make([]string, len(j.LogLines))
	copy(history, j.LogLines)
	return ch, history, j.Status, j.Error
}

// Unsubscribe removes a listener.
func (j *Job) Unsubscribe(ch chan Event) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i, existing := range j.listeners {
		if existing == ch {
			j.listeners = append(j.listeners[:i], j.listeners[i+1:]...)
			return
		}
	}
}

// RecordProgress advances the job and fans the update out to its listeners.
// Returns the log line it recorded so the caller can log it with its own
// package's logger.
func (j *Job) RecordProgress(step string, progress int, message string) string {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Step = step
	j.Progress = progress
	line := fmt.Sprintf("[%d%%] %s: %s", progress, step, message)
	j.LogLines = append(j.LogLines, line)
	j.fanout(Event{Event: "progress", Data: ProgressData{Step: step, Progress: progress, Message: message}})
	return line
}

// RecordComplete fans a terminal event out to the job's listeners.
func (j *Job) RecordComplete(status, errMsg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.fanout(Event{Event: "complete", Data: CompleteData{Status: status, Error: errMsg}})
}

// AppendLog records a line without touching the progress percentage.
func (j *Job) AppendLog(line string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.LogLines = append(j.LogLines, line)
}

// Snapshot copies the log lines under the lock, for a reader that must not race
// a running job.
func (j *Job) Snapshot() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]string, len(j.LogLines))
	copy(out, j.LogLines)
	return out
}

// fanout must be called with j.mu held. A listener that cannot keep up drops
// the event rather than blocking the install.
func (j *Job) fanout(evt Event) {
	for _, ch := range j.listeners {
		select {
		case ch <- evt:
		default:
		}
	}
}
