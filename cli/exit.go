package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// ExitError carries the process exit code main should use. Usage errors
// (bad flags/arguments) exit 2; a run that failed exits 1. An error that
// isn't an *ExitError also exits 1, same as before this type existed.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// Usage wraps err as an exit-2 error: the caller misused the command.
func Usage(err error) error { return &ExitError{Code: 2, Err: err} }

// Failed wraps err as an exit-1 error: the command ran but did not succeed.
func Failed(err error) error { return &ExitError{Code: 1, Err: err} }

// exitCode picks the process exit code for err: the code carried by an
// *ExitError anywhere in its chain, or 1 for any other non-nil error.
func exitCode(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return 1
}

// usageArgs wraps a cobra positional-argument validator so a wrong number of
// arguments exits 2 rather than 1.
//
// cobra's own validators return a plain error, which main exits 1 on — the same
// code a command that RAN and failed uses. Every command in this tree goes
// through this wrapper so a script can always tell "I called it wrong" from "it
// did not work". The root's SetFlagErrorFunc does the same for flag parsing,
// which cobra DOES inherit; Args is set per command, so it needs the wrapper at
// each one.
func usageArgs(fn cobra.PositionalArgs) cobra.PositionalArgs {
	return func(c *cobra.Command, args []string) error {
		if err := fn(c, args); err != nil {
			return Usage(err)
		}
		return nil
	}
}
