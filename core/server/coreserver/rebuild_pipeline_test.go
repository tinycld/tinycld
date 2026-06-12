package coreserver

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunBuildPipeline_StepOrder(t *testing.T) {
	build := t.TempDir()
	var calls []string
	runner := func(dir, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return "", nil
	}
	// pnpm install runs via the streaming runner — stub it to record the call
	// alongside the buffered ones so the step-order check still sees "pnpm install".
	defer stubPnpmStream(&calls)()
	staged := false
	stage := func(appDir string) (string, error) {
		staged = true
		return filepath.Join(appDir, "release-staging", "rel-1"), nil
	}
	nativeExported := false
	nativeExport := func(j *installJob, appDir, buildID, rv string) ([]bundleMeta, error) {
		nativeExported = true
		return nil, nil // toolchain-absent no-op
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	out, err := runBuildPipelineWith(job, build, "build-1", runner, stage, nativeExport)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, " | ")
	pnpm := strings.Index(joined, "pnpm install")
	gob := strings.Index(joined, "go build")
	expo := strings.Index(joined, "expo")
	if !(pnpm >= 0 && gob > pnpm && expo > gob) {
		t.Fatalf("bad step order: %s", joined)
	}
	if !staged {
		t.Fatal("release was not staged after expo export")
	}
	if !nativeExported {
		t.Fatal("native bundles were not exported after staging")
	}
	if out.releaseID != "rel-1" {
		t.Fatalf("releaseID = %q, want rel-1", out.releaseID)
	}
}

func TestRunBuildPipeline_StopsOnFailure(t *testing.T) {
	build := t.TempDir()
	// pnpm install (the first step) fails via the streaming runner; the buffered
	// runner must then never be called (the pipeline stops before go build).
	prev := pnpmStream
	pnpmStream = func(_ func(string), _, _ string, _ ...string) (string, error) {
		return "", &cmdErr{"boom"}
	}
	defer func() { pnpmStream = prev }()

	var bufferedCalls int
	runner := func(dir, name string, args ...string) (string, error) {
		bufferedCalls++
		return "", nil
	}
	noopStage := func(appDir string) (string, error) { return appDir, nil }
	noopNative := func(j *installJob, appDir, buildID, rv string) ([]bundleMeta, error) { return nil, nil }
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if _, err := runBuildPipelineWith(job, build, "build-1", runner, noopStage, noopNative); err == nil {
		t.Fatal("expected error from failing pnpm step")
	}
	if bufferedCalls != 0 {
		t.Fatalf("pipeline should stop after pnpm fails, ran %d later steps", bufferedCalls)
	}
}

// stubPnpmStream replaces the pnpm streaming runner with a no-op that records a
// "pnpm install" entry into calls, and returns a restore func.
func stubPnpmStream(calls *[]string) func() {
	prev := pnpmStream
	pnpmStream = func(_ func(string), _, name string, args ...string) (string, error) {
		*calls = append(*calls, name+" "+strings.Join(args, " "))
		return "", nil
	}
	return func() { pnpmStream = prev }
}

// TestPnpmLineProgress locks the parser to pnpm's real non-TTY reporter lines
// (captured from `pnpm install` without a TTY) so the bar advances through the
// install band and never crosses the go-build milestone.
func TestPnpmLineProgress(t *testing.T) {
	cases := []struct {
		line string
		want int
	}{
		{"Progress: resolved 1, reused 0, downloaded 0, added 0", 49},
		{"Progress: resolved 3, reused 1, downloaded 2, added 3, done", 49},
		{"Packages: +3", 54},
		{"Done in 807ms using pnpm v11.3.0", 58},
		{"+ lodash 4.18.1", 0}, // dependency listing — no milestone
		{"", 0},                // blank
		{"some postinstall noise", 0},
	}
	for _, c := range cases {
		if got := pnpmLineProgress(c.line); got != c.want {
			t.Errorf("pnpmLineProgress(%q) = %d, want %d", c.line, got, c.want)
		}
	}
	// Every recognized milestone must stay within [progPnpmInstall, progGoBuild).
	for _, pct := range []int{49, 54, 58} {
		if pct < progPnpmInstall || pct >= progGoBuild {
			t.Errorf("pnpm milestone %d outside install band [%d,%d)", pct, progPnpmInstall, progGoBuild)
		}
	}
}

// TestPnpmProgressThrottle_Allow checks the bare throttle window: the first call
// passes, calls inside the window are dropped, and a call once the window has
// elapsed passes and re-arms the window.
func TestPnpmProgressThrottle_Allow(t *testing.T) {
	base := time.Unix(0, 0)
	th := newPnpmProgressThrottle()

	if !th.allow(base) {
		t.Fatal("first call should always pass")
	}
	if th.allow(base.Add(pnpmProgressInterval - time.Nanosecond)) {
		t.Fatal("call just inside the window should be dropped")
	}
	if !th.allow(base.Add(pnpmProgressInterval)) {
		t.Fatal("call once the window elapsed should pass")
	}
	// The pass above re-armed the window from base+interval, so another call a
	// hair later is dropped again.
	if th.allow(base.Add(pnpmProgressInterval + time.Second)) {
		t.Fatal("window should re-arm from the last passed call")
	}
}

// TestReportPnpmProgress_Throttled asserts the throttle applies ONLY to the
// high-frequency "Progress:" lines: a burst within one window forwards just the
// first, while other milestones ("Packages:", "Done") always pass even while a
// Progress window is still open.
func TestReportPnpmProgress_Throttled(t *testing.T) {
	now := time.Unix(0, 0)
	prev := nowFunc
	nowFunc = func() time.Time { return now }
	defer func() { nowFunc = prev }()

	job := &installJob{ID: "j", Done: make(chan struct{})}
	throttle := newPnpmProgressThrottle()

	// Five "Progress:" lines arriving within one second — only the first passes.
	for i := 0; i < 5; i++ {
		now = now.Add(200 * time.Millisecond)
		reportPnpmProgress(job, "Progress: resolved 1324, reused 1142, downloaded 43, added 1250", throttle)
	}
	if got := len(job.LogLines); got != 1 {
		t.Fatalf("Progress: burst within window forwarded %d updates, want 1", got)
	}

	// Non-"Progress:" milestones are NOT throttled — they forward even though the
	// Progress window opened above is still wide open.
	reportPnpmProgress(job, "Packages: +42", throttle)
	reportPnpmProgress(job, "Done in 807ms using pnpm v11.3.0", throttle)
	if got := len(job.LogLines); got != 3 {
		t.Fatalf("non-Progress milestones were throttled; total = %d, want 3", got)
	}

	// A further Progress: line is still gated (its window never re-armed because
	// the milestones above don't touch the throttle).
	now = now.Add(time.Second)
	reportPnpmProgress(job, "Progress: resolved 1400, reused 1200, downloaded 60, added 1300", throttle)
	if got := len(job.LogLines); got != 3 {
		t.Fatalf("Progress: line within window forwarded; total = %d, want 3", got)
	}

	// Cross the window — the next Progress: line forwards.
	now = now.Add(pnpmProgressInterval)
	reportPnpmProgress(job, "Progress: resolved 1500, reused 1300, downloaded 80, added 1400", throttle)
	if got := len(job.LogLines); got != 4 {
		t.Fatalf("Progress: line past window dropped; total = %d, want 4", got)
	}

	// Non-milestone lines never forward.
	reportPnpmProgress(job, "some postinstall noise", throttle)
	if got := len(job.LogLines); got != 4 {
		t.Fatalf("non-milestone line forwarded; total = %d, want 4", got)
	}
}

type cmdErr struct{ msg string }

func (e *cmdErr) Error() string { return e.msg }
