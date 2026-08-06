package pkgbuild

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// pnpmProgressInterval is the minimum gap between forwarded "Progress:" lines.
// pnpm's non-TTY reporter emits one every few hundred ms on a large graph, which
// floods the progress sink and makes the bar look busier than the install
// actually is; we forward at most one per window so the UI reflects real pace.
// The first one always passes (so the bar starts moving promptly). Other,
// lower-frequency milestones are not throttled.
const pnpmProgressInterval = 10 * time.Second

// Pipeline turns an assembled build dir into a runnable one: install
// dependencies (the workspace postinstall runs the generator + link-members),
// compile the server binary, export + stage the web bundle, and export the
// native OTA bundles.
//
// Every field is optional; the zero value runs the real pipeline in-process
// with the default binary name. Fields exist so hosts can inject their
// sandbox (runners), their staging, and their configuration — and so tests
// stub steps per-instance instead of mutating package state (a Pipeline value
// is safe to use concurrently with others, which the multi-org builder's
// parallel jobs require).
type Pipeline struct {
	// Run executes buffered commands (go build). Default RunCmd.
	Run CmdRunner
	// RunEnv executes buffered commands with scoped extra env (the CLI
	// cross-compiles' GOOS/GOARCH/CGO_ENABLED). Default RunCmdEnv.
	RunEnv CmdEnvRunner
	// PnpmStream executes the pnpm install with line streaming. Default
	// RunCmdStreaming.
	PnpmStream StreamingRunner
	// ExpoStream executes the expo exports (web + native) with line streaming,
	// so Metro's per-module bundling progress reaches the bar instead of the
	// step sitting frozen for the minutes a cold bundle takes. Default
	// RunCmdStreaming.
	ExpoStream StreamingRunner
	// Stage moves the exported dist/ into release-staging/<id>/ and returns
	// the staged dir. Default StageRelease.
	Stage func(appDir string) (string, error)
	// NativeExport exports the iOS/Android OTA bundles. Default
	// (Pipeline).ExportNativeBundles.
	NativeExport func(sink ProgressSink, appDir, buildID, runtimeVersion string) ([]BundleMeta, error)
	// BinaryName is the server binary filename `go build -o` produces. The
	// single-tenant host passes its Register-time binary name; default
	// "tinycld".
	BinaryName string
	// ConfigValue resolves host configuration (the Sentry keys: "sentry.dsn",
	// "sentry.auth_token", "sentry.org", "sentry.project"). nil resolves
	// everything to "" — Sentry-less builds are the normal self-hosted case.
	ConfigValue func(key string) string
	// Now is the throttle clock. Default time.Now.
	Now func() time.Time
}

// BuildOutput captures what a successful pipeline produced for the host to
// record (pkg_build row, OTA advertisement) and stage.
type BuildOutput struct {
	ReleaseID      string
	StageDir       string
	RuntimeVersion string
	Bundles        []BundleMeta
}

func (p Pipeline) run() CmdRunner {
	if p.Run != nil {
		return p.Run
	}
	return RunCmd
}

func (p Pipeline) runEnv() CmdEnvRunner {
	if p.RunEnv != nil {
		return p.RunEnv
	}
	return RunCmdEnv
}

func (p Pipeline) pnpmStream() StreamingRunner {
	if p.PnpmStream != nil {
		return p.PnpmStream
	}
	return RunCmdStreaming
}

func (p Pipeline) expoStream() StreamingRunner {
	if p.ExpoStream != nil {
		return p.ExpoStream
	}
	return RunCmdStreaming
}

func (p Pipeline) stage() func(string) (string, error) {
	if p.Stage != nil {
		return p.Stage
	}
	return StageRelease
}

func (p Pipeline) nativeExport() func(ProgressSink, string, string, string) ([]BundleMeta, error) {
	if p.NativeExport != nil {
		return p.NativeExport
	}
	return p.ExportNativeBundles
}

func (p Pipeline) binaryName() string {
	if p.BinaryName != "" {
		return p.BinaryName
	}
	return "tinycld"
}

func (p Pipeline) configValue(key string) string {
	if p.ConfigValue == nil {
		return ""
	}
	return p.ConfigValue(key)
}

func (p Pipeline) now() time.Time {
	if p.Now != nil {
		return p.Now()
	}
	return time.Now()
}

// Execute runs the pipeline against an assembled build dir. Each step reports
// through sink.
func (p Pipeline) Execute(sink ProgressSink, buildDir, buildID string) (BuildOutput, error) {
	sink = sinkOrNop(sink)
	appDir := filepath.Join(buildDir, "tinycld")
	goDir := filepath.Join(appDir, "server")

	// TRUST BOUNDARY (intentional, not an oversight — see the "Trust model &
	// security" section of docs/superpowers/specs/2026-06-10-rebuild-from-scratch-design.md).
	// This step runs the installed members' own build scripts (and, via the
	// generator postinstall, evaluates their manifest.ts). Installing a package is
	// therefore equivalent to running its author's code on the host; single-tenant
	// gates this to the org owner at the install endpoint, and the multi-org
	// builder confines the whole job. Do not "harden" this by sandboxing the
	// build alone — member Go gets compiled into the server below and runs as the
	// server at runtime regardless, so a build sandbox would buy no real isolation.
	sink.Progress("Installing dependencies", ProgPnpmInstall, "pnpm install")
	if err := TimeStep(sink, "pnpm install (+ generator postinstall)", func() error {
		return p.runPnpmInstall(sink, buildDir)
	}); err != nil {
		return BuildOutput{}, wrapStep("pnpm install", err)
	}
	// TRUST BOUNDARY: a member's server/ Go module is compiled INTO this binary and
	// then runs in-process as the server, on every boot, with full privileges (DB
	// handle, filesystem, secrets). Installing a package = trusting its author with
	// the server. By design — see the doc section referenced above.
	sink.Progress("Building server", ProgGoBuild, "go build")
	if err := TimeStep(sink, "go build (server binary)", func() error {
		out, e := p.run()(goDir, "go", "build", "-o", filepath.Join(appDir, p.binaryName()), ".")
		if e != nil {
			// Same rationale as runPnpmInstall: in the multi-org builder the
			// error string is all that leaves the job child, so the compile
			// errors must ride it.
			return ErrFromCmd("go build", lastLines(out, 30), e)
		}
		return nil
	}); err != nil {
		return BuildOutput{}, wrapStep("go build", err)
	}
	// Cross-compile the per-org CLI binaries. Shares the ProgGoBuild slot (the
	// progress bands are contiguous; a new constant would renumber them) and
	// never fails the build — see buildCLIBinaries.
	sink.Progress("Building CLI binaries", ProgGoBuild, "go build (cli)")
	_ = TimeStep(sink, "cli cross-compile (best-effort)", func() error {
		p.buildCLIBinaries(sink, appDir)
		return nil
	})
	// The web bundle owns [ProgGoBuild, ProgExpoWeb): runExportWithProgress climbs
	// the bar through it from Metro's per-module progress so a cold (multi-minute)
	// bundle visibly advances instead of parking at one value.
	sink.Progress("Exporting web bundle", ProgGoBuild, "expo export --platform web")
	if err := TimeStep(sink, "expo export (web bundle)", func() error {
		_, e := p.runExportWithProgress(sink, ProgGoBuild, ProgExpoWeb,
			"Exporting web bundle", appDir, "--platform", "web")
		return e
	}); err != nil {
		return BuildOutput{}, wrapStep("expo export", err)
	}
	// Stage the exported dist/ into <appDir>/release-staging/<id>/ so the
	// entrypoint's promote_release (which reads /workspace/current/release-staging
	// after the swap) finds the new bundle. Without this the server serves the old
	// bundle or 404s ("Unmatched Route") on a newly-installed package's routes.
	sink.Progress("Staging web bundle", ProgStageRelease, "release-staging")
	stageDir, err := p.stage()(appDir)
	if err != nil {
		return BuildOutput{}, wrapStep("stage release", err)
	}
	releaseID := filepath.Base(stageDir)
	runtimeVersion := appVersionFromManifest(appDir)

	// Export the native iOS/Android OTA bundles and stage them into the release so
	// /api/app/update can advertise them. nativeExport no-ops (returns nil) when the
	// RN toolchain is absent, leaving mobile on the embedded bundle.
	sink.Progress("Exporting native bundles", ProgNativeStart, "expo export --platform ios/android")
	sink.Logf("web bundle staged: release %s (runtime version %s)", releaseID, runtimeVersion)
	var bundles []BundleMeta
	if err := TimeStep(sink, "native OTA export (ios/android)", func() error {
		var e error
		bundles, e = p.nativeExport()(sink, appDir, buildID, runtimeVersion)
		return e
	}); err != nil {
		return BuildOutput{}, wrapStep("native export", err)
	}
	if len(bundles) > 0 {
		sink.Logf("native OTA bundles produced: %d", len(bundles))
		if err := stageNativeBundlesIntoRelease(stageDir, bundles); err != nil {
			return BuildOutput{}, wrapStep("stage native bundles", err)
		}
		// The per-platform dist-<platform> dirs have been staged into the release;
		// drop them so the build dir doesn't retain a second copy of every bundle.
		cleanupNativeExportDirs(bundles)
	} else {
		sink.Logf("native OTA export skipped (RN toolchain absent) — mobile stays on embedded bundle")
	}

	sink.Progress("Build complete", ProgNativeEnd, "workspace built")
	return BuildOutput{
		ReleaseID:      releaseID,
		StageDir:       stageDir,
		RuntimeVersion: runtimeVersion,
		Bundles:        bundles,
	}, nil
}

func wrapStep(step string, err error) error {
	return fmt.Errorf("%s: %w", step, err)
}

// CheckBuildPrereqs verifies that Go and gcc are available on PATH.
func CheckBuildPrereqs() error {
	for _, tool := range []string{"go", "gcc"} {
		if _, err := exec.LookPath(tool); err != nil {
			return fmt.Errorf("%s not found on PATH: %w", tool, err)
		}
	}
	return nil
}

// StageRelease moves the freshly-built <appDir>/dist into
// <appDir>/release-staging/<id>/ with a release-id.txt and index.html renamed
// to app.html, matching the layout entrypoint.sh's promote_release expects. The
// entrypoint promotes the staged release (merging assets into releases/_static/
// and pointing releases/current at it) on the next boot — which the install /
// uninstall pipelines trigger via requestRestart. Returns the staged dir.
func StageRelease(appDir string) (string, error) {
	releaseID := fmt.Sprintf("install-%d", time.Now().UnixMilli())
	distDir := filepath.Join(appDir, "dist")
	stagingDir := filepath.Join(appDir, "release-staging")
	stageDest := filepath.Join(stagingDir, releaseID)

	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}
	// Prefer a rename (atomic, same filesystem); fall back to copy across
	// devices or when the destination can't be renamed into.
	if err := os.Rename(distDir, stageDest); err != nil {
		if cpErr := CopyDir(distDir, stageDest); cpErr != nil {
			return "", fmt.Errorf("move dist failed: %v; copy fallback failed: %w", err, cpErr)
		}
		os.RemoveAll(distDir)
	}

	if err := os.WriteFile(filepath.Join(stageDest, "release-id.txt"), []byte(releaseID), 0o644); err != nil {
		return "", err
	}
	stagedIndex := filepath.Join(stageDest, "index.html")
	stagedApp := filepath.Join(stageDest, "app.html")
	if _, statErr := os.Stat(stagedIndex); statErr == nil {
		if err := os.Rename(stagedIndex, stagedApp); err != nil {
			return "", fmt.Errorf("rename index.html → app.html: %w", err)
		}
	}
	return stageDest, nil
}

// runPnpmInstall runs the per-build `pnpm install`, forwarding pnpm's own
// progress lines to the sink so the bar advances within the install band
// instead of sitting frozen at ProgPnpmInstall for the (often minutes-long)
// install. The generator + link-members run via the workspace postinstall, so
// their output streams here too.
func (p Pipeline) runPnpmInstall(sink ProgressSink, buildDir string) error {
	throttle := newPnpmProgressThrottle()
	out, err := p.pnpmStream()(
		func(line string) { p.reportPnpmProgress(sink, line, throttle) },
		buildDir, "pnpm", "install", "--no-frozen-lockfile",
	)
	if err != nil {
		// The failing output must ride the error itself: in the multi-org
		// builder this runs inside a re-exec'd job child whose structured
		// failure line is all the router keeps — a bare "exit status 1" left
		// the actual pnpm/generator error unreachable anywhere.
		return ErrFromCmd("pnpm install", lastLines(out, 30), err)
	}
	return nil
}

// lastLines returns the final n lines of s — the tail is where pnpm and the
// generator print the actual failure, and whole outputs are far too large to
// ride an error string.
func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// pnpmProgressThrottle rate-limits forwarded "Progress:" lines to one per
// pnpmProgressInterval. It is not safe for concurrent use; RunCmdStreaming
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
// band [ProgPnpmInstall, ProgGoBuild). pnpm's non-TTY reporter prints periodic
// "Progress: resolved N, reused M, downloaded K, added W" lines and "Packages:
// +N" / "Done" markers; we nudge the bar a little on each so it visibly moves,
// and surface the raw line in the step log. Unrecognized lines (postinstall /
// generator output) advance nothing but still log.
//
// Only the high-frequency "Progress:" lines are throttle-gated (pnpm emits one
// every few hundred ms on a large graph, flooding the stream); they forward
// at most one update per pnpmProgressInterval. The lower-frequency milestones
// ("Packages: +N", "Done", generator) always pass so they never get swallowed by
// a still-open Progress window.
func (p Pipeline) reportPnpmProgress(sink ProgressSink, line string, throttle *pnpmProgressThrottle) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	pct := pnpmLineProgress(line)
	if pct == 0 {
		return // not a milestone line — keep the bar where it is
	}
	if strings.HasPrefix(line, "Progress:") && !throttle.allow(p.now()) {
		return // a "Progress:" line still inside the throttle window — drop it
	}
	sink.Progress("Installing dependencies", pct, line)
}

// pnpmLineProgress returns the progress percentage a recognized pnpm reporter
// line should move the bar to, or 0 to leave it unchanged. The install band is
// [ProgPnpmInstall=45, ProgGoBuild=60); each phase parks a few points higher so
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
// frozen at lo. step is the progress step label; args are passed to `pnpm exec
// expo export …`. Returns the buffered output + error like RunCmd.
func (p Pipeline) runExportWithProgress(sink ProgressSink, lo, hi int, step, dir string, args ...string) (string, error) {
	throttle := newPnpmProgressThrottle()
	// EXPO_PUBLIC_* vars are inlined into the bundle at export time, so they must
	// be correct for the TARGET platform — NOT inherited from the container env.
	// The runtime image sets ENV EXPO_PUBLIC_ENV=web (for the baked web build), and
	// that value persists into this in-app rebuild. A NATIVE bundle exported with
	// EXPO_PUBLIC_ENV=web drives server-address.ts down the web branch
	// (webShortcut → window.location.origin), which throws on a device (RN has no
	// window.location) → the OTA bundle crashed at configureCore() module-init
	// before any screen rendered. So derive the env from --platform: the web export
	// keeps `web`; native (ios/android) is forced to `production` (the EAS profile
	// the embedded build ships), regardless of the container's EXPO_PUBLIC_ENV.
	// Scope the vars to THIS command via `env` so other pipeline steps are
	// unaffected. NODE_ENV defaults to production for `expo export`; set explicitly
	// for parity.
	publicEnv := "production"
	for i, a := range args {
		if a == "--platform" && i+1 < len(args) && args[i+1] == "web" {
			publicEnv = "web"
		}
	}
	scopedEnv := []string{
		"EXPO_PUBLIC_ENV=" + publicEnv,
		"NODE_ENV=production",
	}
	// Native bundles read the Sentry DSN from EXPO_PUBLIC_SENTRY_DSN inlined at
	// export time (they have no window to receive the server-injected runtime
	// config that the WEB bundle uses). Source it from the host's config — the
	// system-wide source of truth — so a native rebuild/OTA picks up the
	// operator's DSN. The web export deliberately omits it: web resolves the DSN
	// at runtime from window.__TINYCLD_PUBLIC_CONFIG__, and inlining a snapshot
	// here would let a stale build-time value shadow the live one.
	if publicEnv != "web" {
		if dsn := p.configValue("sentry.dsn"); dsn != "" {
			scopedEnv = append(scopedEnv, "EXPO_PUBLIC_SENTRY_DSN="+dsn)
		}
	}
	exportArgs := append(append(scopedEnv, "pnpm", "exec", "expo", "export"), args...)
	return p.expoStream()(
		func(line string) { p.reportExpoProgress(sink, line, step, lo, hi, throttle) },
		dir, "env", exportArgs...,
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
func (p Pipeline) reportExpoProgress(sink ProgressSink, line, step string, lo, hi int, throttle *pnpmProgressThrottle) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	frac, isPct := expoLineFraction(line)
	switch {
	case isPct:
		if !throttle.allow(p.now()) {
			return // a percentage line still inside the throttle window — drop it
		}
		sink.Progress(step, bandPct(lo, hi, frac), line)
	case strings.Contains(line, "Bundled "), strings.HasPrefix(line, "Exported:"):
		// Bundle finished / written — park just below hi (staging owns hi).
		sink.Progress(step, hi-1, line)
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
