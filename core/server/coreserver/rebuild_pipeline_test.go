package coreserver

import (
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
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := runBuildPipelineWith(job, build, runner); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, " | ")
	pnpm := strings.Index(joined, "pnpm install")
	gob := strings.Index(joined, "go build")
	expo := strings.Index(joined, "expo")
	if !(pnpm >= 0 && gob > pnpm && expo > gob) {
		t.Fatalf("bad step order: %s", joined)
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
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := runBuildPipelineWith(job, build, runner); err == nil {
		t.Fatal("expected error from failing pnpm step")
	}
	if calls != 1 {
		t.Fatalf("pipeline should stop after the first failure, ran %d steps", calls)
	}
}

type cmdErr struct{ msg string }

func (e *cmdErr) Error() string { return e.msg }
