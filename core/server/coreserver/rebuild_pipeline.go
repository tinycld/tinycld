package coreserver

import (
	"fmt"
	"path/filepath"
	"strings"
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

	emitProgress(job, "Installing dependencies", progPnpmInstall, "pnpm install")
	if err := timeStep(job, "pnpm install (+ generator postinstall)", func() error {
		return runPnpmInstall(job, buildDir)
	}); err != nil {
		return buildOutput{}, wrapStep("pnpm install", err)
	}
	emitProgress(job, "Building server", progGoBuild, "go build")
	if err := timeStep(job, "go build (server binary)", func() error {
		_, e := run(goDir, "go", "build", "-o", filepath.Join(appDir, binaryName), ".")
		return e
	}); err != nil {
		return buildOutput{}, wrapStep("go build", err)
	}
	emitProgress(job, "Exporting web bundle", progExpoWeb, "expo export")
	if err := timeStep(job, "expo export (web bundle)", func() error {
		_, e := run(appDir, "npx", "expo", "export", "--platform", "web")
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
	_, err := pnpmStream(
		func(line string) { reportPnpmProgress(job, line) },
		buildDir, "pnpm", "install", "--no-frozen-lockfile",
	)
	return err
}

// reportPnpmProgress maps a single pnpm output line onto the install progress
// band [progPnpmInstall, progGoBuild). pnpm's non-TTY reporter prints periodic
// "Progress: resolved N, reused M, downloaded K, added W" lines and "Packages:
// +N" / "Done" markers; we nudge the bar a little on each so it visibly moves,
// and surface the raw line in the step log. Unrecognized lines (postinstall /
// generator output) advance nothing but still log.
func reportPnpmProgress(job *installJob, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	pct := pnpmLineProgress(line)
	if pct == 0 {
		return // not a milestone line — keep the bar where it is
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
