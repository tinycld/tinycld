package coreserver

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunBuildPipeline_StepOrder(t *testing.T) {
	build := t.TempDir()
	var calls []string
	runner := func(dir, name string, args ...string) (string, error) {
		calls = append(calls, name+" "+strings.Join(args, " "))
		return "", nil
	}
	staged := false
	stage := func(appDir string) (string, error) { staged = true; return filepath.Join(appDir, "release-staging", "rel-1"), nil }
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
	var calls int
	runner := func(dir, name string, args ...string) (string, error) {
		calls++
		if name == "pnpm" {
			return "", &cmdErr{"boom"}
		}
		return "", nil
	}
	noopStage := func(appDir string) (string, error) { return appDir, nil }
	noopNative := func(j *installJob, appDir, buildID, rv string) ([]bundleMeta, error) { return nil, nil }
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if _, err := runBuildPipelineWith(job, build, "build-1", runner, noopStage, noopNative); err == nil {
		t.Fatal("expected error from failing pnpm step")
	}
	if calls != 1 {
		t.Fatalf("pipeline should stop after the first failure, ran %d steps", calls)
	}
}

type cmdErr struct{ msg string }

func (e *cmdErr) Error() string { return e.msg }
