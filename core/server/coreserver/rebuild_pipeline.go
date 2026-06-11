package coreserver

import (
	"fmt"
	"path/filepath"
)

// cmdRunner runs a command in dir; injectable for tests. Mirrors runCmd.
type cmdRunner func(dir, name string, args ...string) (string, error)

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

	emitProgress(job, "Installing dependencies", 50, "pnpm install")
	if err := timeStep(job, "pnpm install (+ generator postinstall)", func() error {
		_, e := run(buildDir, "pnpm", "install", "--no-frozen-lockfile")
		return e
	}); err != nil {
		return buildOutput{}, wrapStep("pnpm install", err)
	}
	emitProgress(job, "Building server", 70, "go build")
	if err := timeStep(job, "go build (server binary)", func() error {
		_, e := run(goDir, "go", "build", "-o", filepath.Join(appDir, binaryName), ".")
		return e
	}); err != nil {
		return buildOutput{}, wrapStep("go build", err)
	}
	emitProgress(job, "Exporting web bundle", 85, "expo export")
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
	emitProgress(job, "Staging web bundle", 88, "release-staging")
	stageDir, err := stage(appDir)
	if err != nil {
		return buildOutput{}, wrapStep("stage release", err)
	}
	releaseID := filepath.Base(stageDir)
	runtimeVersion := appVersionFromManifest(appDir)

	// Export the native iOS/Android OTA bundles and stage them into the release so
	// /api/app/update can advertise them. nativeExport no-ops (returns nil) when the
	// RN toolchain is absent, leaving mobile on the embedded bundle.
	emitProgress(job, "Exporting native bundles", 92, "expo export --platform ios/android")
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
	} else {
		jobLogf(job, "native OTA export skipped (RN toolchain absent) — mobile stays on embedded bundle")
	}

	emitProgress(job, "Build complete", 94, "workspace built")
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
