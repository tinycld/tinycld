package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// PassphraseEnv is the environment variable a backup passphrase can be
	// read from, so it never has to appear as a command-line flag (flags
	// show up in `ps` and shell history).
	PassphraseEnv = "TINYCLD_BACKUP_PASSPHRASE"
	// MinPassphrase is the minimum accepted passphrase length.
	MinPassphrase = 12
)

var (
	ErrNoPassphrase       = errors.New("no passphrase: pass --passphrase-file, set " + PassphraseEnv + ", or run interactively")
	ErrPassphraseTooShort = fmt.Errorf("the passphrase must be at least %d characters", MinPassphrase)
	ErrPassphraseMismatch = errors.New("the passphrases do not match")
)

// PassphraseSource resolves a backup passphrase without ever taking it as a
// command-line flag. It checks, in order: a file, an environment variable,
// then an interactive prompt.
type PassphraseSource struct {
	// File is the path given via --passphrase-file, or empty to skip it.
	File string
	// Env reads an environment variable; os.Getenv in production.
	Env func(string) string
	// Interactive allows falling back to a terminal prompt.
	Interactive bool
	// ReadPassword reads a line without echoing it; term.ReadPassword in
	// production.
	ReadPassword func(fd int) ([]byte, error)
	// Stdin supplies the fd for ReadPassword. Nil in tests, where
	// ReadPassword is stubbed and never inspects it.
	Stdin *os.File
	// Stderr receives the prompt text.
	Stderr io.Writer
}

// Read resolves the passphrase. When confirm is true and the value came from
// an interactive prompt, the user is asked to type it twice and the two
// must match.
func (s PassphraseSource) Read(confirm bool) (string, error) {
	if s.File != "" {
		b, err := os.ReadFile(s.File)
		if err != nil {
			return "", fmt.Errorf("read passphrase file: %w", err)
		}
		return check(strings.TrimSuffix(strings.TrimSuffix(string(b), "\n"), "\r"))
	}
	if s.Env != nil {
		if v := s.Env(PassphraseEnv); v != "" {
			return check(v)
		}
	}
	if !s.Interactive || s.ReadPassword == nil {
		return "", ErrNoPassphrase
	}
	first, err := s.prompt("Passphrase: ")
	if err != nil {
		return "", err
	}
	if _, err := check(first); err != nil {
		return "", err
	}
	if confirm {
		second, err := s.prompt("Confirm passphrase: ")
		if err != nil {
			return "", err
		}
		if !bytes.Equal([]byte(first), []byte(second)) {
			return "", ErrPassphraseMismatch
		}
	}
	return first, nil
}

func (s PassphraseSource) prompt(label string) (string, error) {
	fmt.Fprint(s.Stderr, label)
	fd := 0
	if s.Stdin != nil {
		fd = int(s.Stdin.Fd())
	}
	b, err := s.ReadPassword(fd)
	fmt.Fprintln(s.Stderr)
	if err != nil {
		return "", fmt.Errorf("read passphrase: %w", err)
	}
	return string(b), nil
}

func check(p string) (string, error) {
	if len([]rune(p)) < MinPassphrase {
		return "", ErrPassphraseTooShort
	}
	return p, nil
}
