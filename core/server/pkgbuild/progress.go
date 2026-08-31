package pkgbuild

import "time"

// ProgressSink is where the pipeline reports what it is doing. It is the
// host seam for user-facing progress: coreserver adapts it onto its SSE
// stream + durable pkg_install_log lines; the hosting builder writes a job
// log. Every pkgbuild step reports through the sink and NOTHING else, so the
// library stays silent by default (NopSink) and hosts own presentation.
type ProgressSink interface {
	// Progress marks a milestone: it moves the progress bar to percent and
	// records a headline step + message.
	Progress(step string, percent int, message string)
	// Logf records a detail line under the current milestone WITHOUT moving
	// the progress bar.
	Logf(format string, args ...any)
}

type nopSink struct{}

func (nopSink) Progress(string, int, string) {}
func (nopSink) Logf(string, ...any)          {}

// NopSink returns a ProgressSink that discards everything. It is the default
// for tests and for builder bring-up before a job log exists.
func NopSink() ProgressSink { return nopSink{} }

// sinkOrNop lets every entry point tolerate a nil sink.
func sinkOrNop(s ProgressSink) ProgressSink {
	if s == nil {
		return nopSink{}
	}
	return s
}

// TimeStep brackets a named step: it logs a START detail line, runs fn, then
// logs a completion line with the elapsed duration (or a FAILED line with the
// duration and error). The duration is the key debugging signal — which step
// is slow, and which step a hang is stuck in. Returns fn's error unchanged so
// callers keep their existing control flow.
func TimeStep(s ProgressSink, step string, fn func() error) error {
	s = sinkOrNop(s)
	start := time.Now()
	s.Logf("%s: started", step)
	err := fn()
	dur := time.Since(start).Round(time.Millisecond)
	if err != nil {
		s.Logf("%s: FAILED after %s: %v", step, dur, err)
		return err
	}
	s.Logf("%s: done in %s", step, dur)
	return nil
}

// Progress milestones for the pkgbuild-owned portion of a rebuild, in the
// order the pipeline hits them. The assemble band (members fetched/copied)
// runs from ProgAssembleStart to ProgAssembleEnd with per-member ticks spread
// across it; the build pipeline (pnpm/go/expo/native) owns the middle up to
// ProgNativeEnd. Host tails (DB backup, migrate, activate, restart in
// coreserver; artifact publish in the builder) own the 90..100 band ABOVE
// this ceiling. Keeping the whole scale here (rather than scattered magic
// numbers) keeps the bar monotonic across every host and rebuild path.
const (
	ProgAssembleStart = 5
	ProgAssembleEnd   = 42
	ProgPnpmInstall   = 45
	ProgGoBuild       = 60
	ProgExpoWeb       = 72
	ProgStageRelease  = 76
	ProgNativeStart   = 80
	ProgNativeEnd     = 88
)
