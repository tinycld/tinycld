// Package ui holds the CLI's interactive prompts. Every prompt has a
// non-interactive escape hatch: --yes skips it, and a non-TTY stdin never
// blocks — commands either proceed (informational pauses) or fail with an
// instruction to pass a flag (confirmations).
package ui

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"tinycld.org/cli/output"
)

// WaitEnter pauses until the user presses Enter. Skipped (returning
// immediately) under --yes or when stdin is not a terminal.
func WaitEnter(o output.Options, yes bool, stdin io.Reader, stdout io.Writer, msg string) {
	if yes || !o.Interactive {
		return
	}
	fmt.Fprint(stdout, msg)
	bufio.NewReader(stdin).ReadString('\n')
}

// Confirm asks a yes/no question. --yes answers yes without asking; a
// non-TTY run without --yes refuses rather than hanging, so scripts must opt
// in to destructive actions explicitly.
func Confirm(o output.Options, yes bool, stdin io.Reader, stdout io.Writer, msg string) (bool, error) {
	if yes {
		return true, nil
	}
	if !o.Interactive {
		return false, errors.New("refusing to prompt without a terminal — pass --yes to proceed")
	}
	fmt.Fprintf(stdout, "%s [y/N] ", msg)
	answer, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes", nil
}

// ConfirmOverwrite asks before a download clobbers an existing local file.
//
// One helper rather than the check open-coded per command: `drive get`,
// `drive export`, `mail download` and the contacts/calendar `--out` writers all
// stream onto a caller-named path, and a silent overwrite of the wrong file is
// not recoverable from the CLI.
//
// Returns nil when the path is free, when the user agrees, or under --yes.
// A refusal (or a non-TTY run without --yes) comes back as an error the caller
// can return unchanged — the download simply does not happen.
func ConfirmOverwrite(o output.Options, yes bool, stdin io.Reader, stdout io.Writer, path string) error {
	if _, err := os.Stat(path); err != nil {
		// Not there (or unreadable — the write itself will report that
		// properly): nothing to overwrite, so nothing to ask about.
		return nil
	}
	ok, err := Confirm(o, yes, stdin, stdout, fmt.Sprintf("%s already exists. Overwrite?", path))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("%s: not overwritten", path)
	}
	return nil
}
