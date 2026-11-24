package main

import (
	"errors"
	"fmt"
	"testing"

	"tinycld.org/cli/exitcode"
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

// A package command module cannot import package main. It sets its code with
// tinycld.org/cli/exitcode, and main must exit with it.
func TestExitCodeFromAPackageCommand(t *testing.T) {
	err := fmt.Errorf("acct1: %w", exitcode.New(exitcode.Refused, errors.New("in use")))
	if got := exitCode(err); got != 6 {
		t.Fatalf("exitCode = %d, want 6", got)
	}
}

// A value of --output that is no format is a wrong call, as a flag that
// cannot be parsed is: exit 2, not the 1 of a command that ran and failed.
func TestAnUnknownOutputFormatIsExit2(t *testing.T) {
	d, _ := testDeps(t)
	_, _, err := runCLI(t, d, "version", "--output", "xml")
	if got := exitCode(err); got != 2 {
		t.Fatalf("exitCode = %d, want 2; err: %v", got, err)
	}
}

// A package command reaches the server through the lazy client. With no
// context at all the fix is to sign in (exit 3); a --context that does not
// exist is a wrong call (exit 2).
func TestResolvingTheClientContextSetsTheExitCode(t *testing.T) {
	d, _ := testDeps(t)
	_, _, err := d.resolveClientContext()
	if got := exitCode(err); got != 3 {
		t.Errorf("no context: exitCode = %d, want 3; err: %v", got, err)
	}
	d.ctxFlag = "nope"
	_, _, err = d.resolveClientContext()
	if got := exitCode(err); got != 2 {
		t.Errorf("unknown context: exitCode = %d, want 2; err: %v", got, err)
	}
}
