package coreserver

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// These helpers give the rebuild pipeline rich, operator-facing logging for what
// are system-critical operations (every package install / upgrade / downgrade /
// rollback / delete / core-swap). They build on emitProgress's durability: a line
// appended to job.LogLines is persisted to pkg_install_log.log on finalize and so
// SURVIVES the exit-75 restart, unlike the SSE stream. Every helper also writes to
// the process log (docker logs) with the shared [pkg_install] prefix.
//
// Use jobLogf for detail lines that aren't a progress milestone (they keep the
// current %), emitProgress for milestones (which move the bar), and timeStep to
// bracket a step with a START line and an end line carrying its duration — so a
// hung build shows up as a START with no matching done line, and a slow one is
// visible in the recorded durations.

// jobLogf appends a durable, timestamped detail line to the job's log (persisted
// to pkg_install_log.log) and the process log, WITHOUT moving the progress bar.
// Detail lines are prefixed "  ·" so they read as sub-items under the milestone
// emitProgress lines in the recorded log.
func jobLogf(job *installJob, format string, args ...any) {
	if job == nil {
		log.Printf("[pkg_install] "+format, args...)
		return
	}
	msg := fmt.Sprintf(format, args...)
	job.mu.Lock()
	job.LogLines = append(job.LogLines, "  · "+msg)
	job.mu.Unlock()
	log.Printf("[pkg_install] [%s]   · %s", job.ID, msg)
}

// timeStep brackets a named step: it logs a START detail line, runs fn, then logs
// a completion line with the elapsed duration (or a FAILED line with the duration
// and error). The duration is the key debugging signal — which step is slow, and
// which step a hang is stuck in. Returns fn's error unchanged so callers keep
// their existing control flow.
func timeStep(job *installJob, step string, fn func() error) error {
	start := monoNow()
	jobLogf(job, "%s: started", step)
	err := fn()
	dur := monoSince(start)
	if err != nil {
		jobLogf(job, "%s: FAILED after %s: %v", step, dur, err)
		return err
	}
	jobLogf(job, "%s: done in %s", step, dur)
	return nil
}

// memberSetSummary renders a manifest's member set compactly for the log: which
// members are fetched fresh (the changed ones) vs copied from the current build,
// with their versions/specs. This documents EXACTLY what a build was assembled
// from — the single most useful line for reproducing a build later.
func memberSetSummary(m RebuildManifest) string {
	var fresh, current []string
	for _, ms := range m.Members {
		if ms.FromCurrent {
			current = append(current, ms.Slug)
		} else {
			label := ms.Slug
			if ms.Version != "" {
				label += "@" + ms.Version
			}
			if ms.Spec != "" {
				label += " (" + ms.Spec + ")"
			}
			fresh = append(fresh, label)
		}
	}
	fetched := "none"
	if len(fresh) > 0 {
		fetched = strings.Join(fresh, ", ")
	}
	return fmt.Sprintf("build %s: fetch=[%s] copy-from-current=[%s]",
		m.BuildID, fetched, strings.Join(current, ", "))
}

// monoNow/monoSince wrap time so the rest of the logging code reads cleanly.
// time.Now() is unavailable under the workflow runtime but this is production
// server code (not a workflow script), so it's fine here.
func monoNow() time.Time          { return time.Now() }
func monoSince(t time.Time) string { return monoNow().Sub(t).Round(time.Millisecond).String() }
