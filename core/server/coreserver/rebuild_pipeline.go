package coreserver

import (
	"fmt"
	"path/filepath"
)

// cmdRunner runs a command in dir; injectable for tests. Mirrors runCmd.
type cmdRunner func(dir, name string, args ...string) (string, error)

// stageReleaseFn moves the exported dist/ into release-staging/<id>/; injectable
// so the pipeline step-order test doesn't touch the filesystem.
type stageReleaseFn func(appDir string) (string, error)

// runBuildPipeline turns an assembled build dir into a runnable one: install
// dependencies (the workspace postinstall runs the generator + link-members),
// compile the server binary, export the web bundle, and stage it for promotion.
// Each step emits progress to the job's SSE stream.
func runBuildPipeline(job *installJob, buildDir string) error {
	return runBuildPipelineWith(job, buildDir, runCmd, stageRelease)
}

func runBuildPipelineWith(job *installJob, buildDir string, run cmdRunner, stage stageReleaseFn) error {
	appDir := filepath.Join(buildDir, "tinycld")
	goDir := filepath.Join(appDir, "server")

	emitProgress(job, "Installing dependencies", 50, "pnpm install")
	if _, err := run(buildDir, "pnpm", "install", "--no-frozen-lockfile"); err != nil {
		return wrapStep("pnpm install", err)
	}
	emitProgress(job, "Building server", 70, "go build")
	if _, err := run(goDir, "go", "build", "-o", filepath.Join(appDir, binaryName), "."); err != nil {
		return wrapStep("go build", err)
	}
	emitProgress(job, "Exporting web bundle", 85, "expo export")
	if _, err := run(appDir, "npx", "expo", "export", "--platform", "web"); err != nil {
		return wrapStep("expo export", err)
	}
	// Stage the exported dist/ into <appDir>/release-staging/<id>/ so the
	// entrypoint's promote_release (which reads /workspace/current/release-staging
	// after the swap) finds the new bundle. Without this the server serves the old
	// bundle or 404s ("Unmatched Route") on a newly-installed package's routes.
	emitProgress(job, "Staging web bundle", 88, "release-staging")
	if _, err := stage(appDir); err != nil {
		return wrapStep("stage release", err)
	}
	emitProgress(job, "Build complete", 90, "workspace built")
	return nil
}

func wrapStep(step string, err error) error {
	return fmt.Errorf("%s: %w", step, err)
}
