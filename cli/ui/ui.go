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
	"strings"

	"tinycld.org/cli/output"
)

// WaitEnter pauses until the user presses Enter. Skipped (returning
// immediately) under --yes or when stdin is not a terminal.
func WaitEnter(o output.Options, yes bool, stdin io.Reader, stdout io.Writer, msg string) {
	if yes || !o.TTY {
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
	if !o.TTY {
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
