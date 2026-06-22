package coreserver

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// cmdRunner runs a command in dir; injectable for tests. Mirrors runCmd.
type cmdRunner func(dir, name string, args ...string) (string, error)

// streamingRunner runs a command, forwarding each output line to onLine as it
// arrives, and returns the full output + error. Injectable for tests; production
// uses runCmdStreaming.
type streamingRunner func(onLine func(string), dir, name string, args ...string) (string, error)

// pnpmStream is the streaming runner the pnpm-install step uses. A package var so
// tests can stub the long real install while still exercising runPnpmInstall's
// progress parsing.
var pnpmStream streamingRunner = runCmdStreaming

// expoStream is the streaming runner the expo-export steps (web + native) use, so
// Metro's per-module bundling progress reaches the bar instead of the step sitting
// frozen for the minutes a cold bundle takes. A package var for the same
// test-stubbing reason as pnpmStream.
var expoStream streamingRunner = runCmdStreaming

// pnpmProgressInterval is the minimum gap between forwarded "Progress:" lines.
// pnpm's non-TTY reporter emits one every few hundred ms on a large graph, which
// floods the SSE stream and makes the bar look busier than the install actually
// is; we forward at most one per window so the UI reflects real pace. The first
// one always passes (so the bar starts moving promptly). Other, lower-frequency
// milestones are not throttled.
const pnpmProgressInterval = 10 * time.Second

// stageReleaseFn moves the exported dist/ into release-staging/<id>/ and returns
// the staged dir; injectable so the pipeline step-order test stays filesystem-free.
type stageReleaseFn func(appDir string) (string, error)

// nativeExportFn exports the iOS/Android OTA bundles for a build; injectable so
// the step-order test doesn't run a real export. Mirrors exportNativeBundles.
type nativeExportFn func(job *installJob, appDir, buildID, runtimeVersion string) ([]bundleMeta, error)

// buildOutput captures what a successful pipeline produced for the orchestrator
// to record in pkg_build and serve via /api/app/update.
type buildOutput struct {
	releaseID      string
	stageDir       string
	runtimeVersion string
	bundles        []bundleMeta
}

// runBuildPipeline turns an assembled build dir into a runnable one: install
// dependencies (the workspace postinstall runs the generator + link-members),
// compile the server binary, export + stage the web bundle, and export the
// native OTA bundles. Each step emits progress to the job's SSE stream.
func runBuildPipeline(job *installJob, buildDir, buildID string) (buildOutput, error) {
	return runBuildPipelineWith(job, buildDir, buildID, runCmd, stageRelease, exportNativeBundles)
}

func runBuildPipelineWith(
	job *installJob,
	buildDir, buildID string,
	run cmdRunner,
	stage stageReleaseFn,
	nativeExport nativeExportFn,
) (buildOutput, error) {
	appDir := filepath.Join(buildDir, "tinycld")
	goDir := filepath.Join(appDir, "server")

	// TRUST BOUNDARY (intentional, not an oversight — see the "Trust model &
	// security" section of docs/superpowers/specs/2026-06-10-rebuild-from-scratch-design.md).
	// This step runs the installed members' own build scripts (and, via the
	// generator postinstall, evaluates their manifest.ts). Installing a package is
	// therefore equivalent to running its author's code on the host; this is gated
	// to super-admins at the install endpoint. Do not "harden" this by sandboxing
	// the build — member Go gets compiled into the server below and runs as the
	// server at runtime regardless, so a build sandbox would buy no real isolation.
	emitProgress(job, "Installing dependencies", progPnpmInstall, "pnpm install")
	if err := timeStep(job, "pnpm install (+ generator postinstall)", func() error {
		return runPnpmInstall(job, buildDir)
	}); err != nil {
		return buildOutput{}, wrapStep("pnpm install", err)
	}
	// TRUST BOUNDARY: a member's server/ Go module is compiled INTO this binary and
	// then runs in-process as the server, on every boot, with full privileges (DB
	// handle, filesystem, secrets). Installing a package = trusting its author with
	// the server. By design — see the doc section referenced above.
	emitProgress(job, "Building server", progGoBuild, "go build")
	if err := timeStep(job, "go build (server binary)", func() error {
		_, e := run(goDir, "go", "build", "-o", filepath.Join(appDir, binaryName), ".")
		return e
	}); err != nil {
		return buildOutput{}, wrapStep("go build", err)
	}
	// The web bundle owns [progGoBuild, progExpoWeb): runExportWithProgress climbs
	// the bar through it from Metro's per-module progress so a cold (multi-minute)
	// bundle visibly advances instead of parking at one value.
	emitProgress(job, "Exporting web bundle", progGoBuild, "expo export --platform web")
	if err := timeStep(job, "expo export (web bundle)", func() error {
		_, e := runExportWithProgress(job, progGoBuild, progExpoWeb,
			"Exporting web bundle", appDir, "--platform", "web")
		return e
	}); err != nil {
		return buildOutput{}, wrapStep("expo export", err)
	}
	// Stage the exported dist/ into <appDir>/release-staging/<id>/ so the
	// entrypoint's promote_release (which reads /workspace/current/release-staging
	// after the swap) finds the new bundle. Without this the server serves the old
	// bundle or 404s ("Unmatched Route") on a newly-installed package's routes.
	emitProgress(job, "Staging web bundle", progStageRelease, "release-staging")
	stageDir, err := stage(appDir)
	if err != nil {
		return buildOutput{}, wrapStep("stage release", err)
	}
	releaseID := filepath.Base(stageDir)
	runtimeVersion := appVersionFromManifest(appDir)

	// Export the native iOS/Android OTA bundles and stage them into the release so
	// /api/app/update can advertise them. nativeExport no-ops (returns nil) when the
	// RN toolchain is absent, leaving mobile on the embedded bundle.
	emitProgress(job, "Exporting native bundles", progNativeStart, "expo export --platform ios/android")
	jobLogf(job, "web bundle staged: release %s (runtime version %s)", releaseID, runtimeVersion)
	var bundles []bundleMeta
	if err := timeStep(job, "native OTA export (ios/android)", func() error {
		var e error
		bundles, e = nativeExport(job, appDir, buildID, runtimeVersion)
		return e
	}); err != nil {
		return buildOutput{}, wrapStep("native export", err)
	}
	if len(bundles) > 0 {
		jobLogf(job, "native OTA bundles produced: %d", len(bundles))
		if err := stageNativeBundlesIntoRelease(stageDir, bundles); err != nil {
			return buildOutput{}, wrapStep("stage native bundles", err)
		}
		// The per-platform dist-<platform> dirs have been staged into the release;
		// drop them so the build dir doesn't retain a second copy of every bundle.
		cleanupNativeExportDirs(bundles)
	} else {
		jobLogf(job, "native OTA export skipped (RN toolchain absent) — mobile stays on embedded bundle")
	}

	emitProgress(job, "Build complete", progNativeEnd, "workspace built")
	return buildOutput{
		releaseID:      releaseID,
		stageDir:       stageDir,
		runtimeVersion: runtimeVersion,
		bundles:        bundles,
	}, nil
}

func wrapStep(step string, err error) error {
	return fmt.Errorf("%s: %w", step, err)
}

// runPnpmInstall runs the per-build `pnpm install`, forwarding pnpm's own
// progress lines to the job so the bar advances within the install band instead
// of sitting frozen at progPnpmInstall for the (often minutes-long) install. The
// generator + link-members run via the workspace postinstall, so their output
// streams here too.
func runPnpmInstall(job *installJob, buildDir string) error {
	throttle := newPnpmProgressThrottle()
	_, err := pnpmStream(
		func(line string) { reportPnpmProgress(job, line, throttle) },
		buildDir, "pnpm", "install", "--no-frozen-lockfile",
	)
	return err
}

// pnpmProgressThrottle rate-limits forwarded "Progress:" lines to one per
// pnpmProgressInterval. It is not safe for concurrent use; runCmdStreaming
// invokes the line callback serially from a single reader goroutine.
type pnpmProgressThrottle struct {
	last    time.Time
	started bool
}

func newPnpmProgressThrottle() *pnpmProgressThrottle { return &pnpmProgressThrottle{} }

// allow reports whether a milestone reached at time now may be forwarded. The
// first call always passes (so the bar starts moving immediately); afterwards a
// call passes only once pnpmProgressInterval has elapsed since the last pass.
func (t *pnpmProgressThrottle) allow(now time.Time) bool {
	if t.started && now.Sub(t.last) < pnpmProgressInterval {
		return false
	}
	t.started = true
	t.last = now
	return true
}

// reportPnpmProgress maps a single pnpm output line onto the install progress
// band [progPnpmInstall, progGoBuild). pnpm's non-TTY reporter prints periodic
// "Progress: resolved N, reused M, downloaded K, added W" lines and "Packages:
// +N" / "Done" markers; we nudge the bar a little on each so it visibly moves,
// and surface the raw line in the step log. Unrecognized lines (postinstall /
// generator output) advance nothing but still log.
//
// Only the high-frequency "Progress:" lines are throttle-gated (pnpm emits one
// every few hundred ms on a large graph, flooding the SSE stream); they forward
// at most one update per pnpmProgressInterval. The lower-frequency milestones
// ("Packages: +N", "Done", generator) always pass so they never get swallowed by
// a still-open Progress window.
func reportPnpmProgress(job *installJob, line string, throttle *pnpmProgressThrottle) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	pct := pnpmLineProgress(line)
	if pct == 0 {
		return // not a milestone line — keep the bar where it is
	}
	if strings.HasPrefix(line, "Progress:") && !throttle.allow(nowFunc()) {
		return // a "Progress:" line still inside the throttle window — drop it
	}
	emitProgress(job, "Installing dependencies", pct, line)
}

// pnpmLineProgress returns the progress percentage a recognized pnpm reporter
// line should move the bar to, or 0 to leave it unchanged. The install band is
// [progPnpmInstall=45, progGoBuild=60); each phase parks a few points higher so
// the bar climbs through resolve → download → link → postinstall without ever
// reaching the go-build milestone.
func pnpmLineProgress(line string) int {
	// Order matters: a "Progress:" line also contains "added"/"reused", so the
	// generic markers must come AFTER the specific prefixes. Match on prefixes
	// (pnpm's stable non-TTY reporter shape) rather than loose substrings.
	switch {
	case strings.HasPrefix(line, "Progress:"):
		return 49 // resolving / downloading the graph
	case strings.HasPrefix(line, "Packages: +"):
		return 54 // packages linked into node_modules
	case strings.HasPrefix(line, "Done in"), strings.Contains(line, "packages:generate"):
		return 58 // install finished / postinstall (generator) running
	default:
		return 0
	}
}

// runExportWithProgress runs an `expo export` invocation through the streaming
// runner, forwarding Metro's bundling progress onto the [lo, hi) band so the bar
// climbs through the (minutes-long, on a cold cache) bundle instead of sitting
// frozen at lo. step is the SSE step label; args are passed to `pnpm exec expo
// export …`. Returns the buffered output + error like runCmd.
func runExportWithProgress(job *installJob, lo, hi int, step, dir string, args ...string) (string, error) {
	throttle := newPnpmProgressThrottle()
	return expoStream(
		func(line string) { reportExpoProgress(job, line, step, lo, hi, throttle) },
		dir, "pnpm", append([]string{"exec", "expo", "export"}, args...)...,
	)
}

// reportExpoProgress maps one `expo export` output line onto the [lo, hi) band.
// Metro's non-TTY reporter prints per-module bundling lines carrying a running
// percentage ("Web …entry.js ▓▓░░ 34.5% (1252/2155)"), then a "… Bundled <ms> …"
// line and a final "Exported: <dir>". We translate the percentage linearly into
// the band so the bar tracks the actual bundle, and park near the top on the
// terminal markers. Lines without a recognizable signal keep the bar where it is.
//
// The high-frequency percentage lines are throttle-gated (one update per
// pnpmProgressInterval) exactly like pnpm's "Progress:" lines; the low-frequency
// "Bundled"/"Exported" markers always pass.
func reportExpoProgress(job *installJob, line, step string, lo, hi int, throttle *pnpmProgressThrottle) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	frac, isPct := expoLineFraction(line)
	switch {
	case isPct:
		if !throttle.allow(nowFunc()) {
			return // a percentage line still inside the throttle window — drop it
		}
		emitProgress(job, step, bandPct(lo, hi, frac), line)
	case strings.Contains(line, "Bundled "), strings.HasPrefix(line, "Exported:"):
		// Bundle finished / written — park just below hi (staging owns hi).
		emitProgress(job, step, hi-1, line)
	default:
		// Asset listings, the per-file output dump, warnings — keep the bar put.
	}
}

// expoLineFraction extracts the bundling fraction (0..1) from a Metro progress
// line of the form "… <NN.N>% (n/total)". Returns ok=false when the line carries
// no such percentage. Parsed by hand (no regexp) against Metro's stable shape:
// find "% (" and read the float immediately before the percent sign.
func expoLineFraction(line string) (float64, bool) {
	i := strings.Index(line, "% (")
	if i <= 0 {
		return 0, false
	}
	// Walk back over the number (digits and a single dot) preceding '%'.
	start := i
	for start > 0 {
		c := line[start-1]
		if (c >= '0' && c <= '9') || c == '.' {
			start--
			continue
		}
		break
	}
	num := line[start:i]
	if num == "" || num == "." {
		return 0, false
	}
	var pct float64
	if _, err := fmt.Sscanf(num, "%f", &pct); err != nil {
		return 0, false
	}
	if pct < 0 {
		pct = 0
	} else if pct > 100 {
		pct = 100
	}
	return pct / 100, true
}

// bandPct maps a fraction (0..1) onto [lo, hi), clamped to never reach hi (the
// next milestone owns hi) so the bar stays monotonic across phase boundaries.
func bandPct(lo, hi int, frac float64) int {
	span := hi - lo
	p := lo + int(frac*float64(span))
	if p >= hi {
		p = hi - 1
	}
	if p < lo {
		p = lo
	}
	return p
}
