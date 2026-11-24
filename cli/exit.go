package main

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/exitcode"
)

// ExitError carries the process exit code main should use. It is
// exitcode.Error: the type lives in an importable package so the command
// modules of packages can set a code too. Usage errors (bad flags/arguments)
// exit 2; a run that failed exits 1. An error that isn't an *ExitError also
// exits 1.
type ExitError = exitcode.Error

// Usage wraps err as an exit-2 error: the caller misused the command.
func Usage(err error) error { return exitcode.New(exitcode.Usage, err) }

// Failed wraps err as an exit-1 error: the command ran but did not succeed.
func Failed(err error) error { return exitcode.New(exitcode.Failed, err) }

// exitCode picks the process exit code for err: the code carried by an
// *ExitError anywhere in its chain, or 1 for any other non-nil error.
func exitCode(err error) int { return exitcode.Of(err) }

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
