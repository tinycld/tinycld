package main

import (
	"errors"
	"testing"
)

func TestUsageWrapsWithCode2(t *testing.T) {
	inner := errors.New("bad flag")
	err := Usage(inner)
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("Usage did not produce an *ExitError: %v", err)
	}
	if ee.Code != 2 {
		t.Fatalf("Code = %d, want 2", ee.Code)
	}
	if !errors.Is(err, inner) {
		t.Fatalf("Unwrap did not reach inner error")
	}
}

func TestFailedWrapsWithCode1(t *testing.T) {
	inner := errors.New("boom")
	err := Failed(inner)
	var ee *ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("Failed did not produce an *ExitError: %v", err)
	}
	if ee.Code != 1 {
		t.Fatalf("Code = %d, want 1", ee.Code)
	}
	if !errors.Is(err, inner) {
		t.Fatalf("Unwrap did not reach inner error")
	}
}

func TestExitCodeDefaultsToOneForPlainError(t *testing.T) {
	if got := exitCode(errors.New("plain")); got != 1 {
		t.Fatalf("exitCode = %d, want 1", got)
	}
}

func TestExitCodeFromExitError(t *testing.T) {
	if got := exitCode(Usage(errors.New("x"))); got != 2 {
		t.Fatalf("exitCode = %d, want 2", got)
	}
}
