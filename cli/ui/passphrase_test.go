package ui

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPassphraseFromFileTrimsOneNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pp")
	_ = os.WriteFile(p, []byte("correct horse battery\n"), 0o600)
	got, err := PassphraseSource{File: p, Env: func(string) string { return "" }}.Read(false)
	if err != nil || got != "correct horse battery" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestPassphraseFromEnv(t *testing.T) {
	got, err := PassphraseSource{Env: func(k string) string {
		if k == PassphraseEnv {
			return "correct horse battery"
		}
		return ""
	}}.Read(false)
	if err != nil || got != "correct horse battery" {
		t.Fatalf("%q %v", got, err)
	}
}

func TestPassphraseTooShortIsRejectedBeforeAnyRequest(t *testing.T) {
	_, err := PassphraseSource{Env: func(string) string { return "short" }}.Read(false)
	if err == nil || !errors.Is(err, ErrPassphraseTooShort) {
		t.Fatalf("err %v", err)
	}
}

func TestPassphrasePromptTwiceMustMatch(t *testing.T) {
	answers := [][]byte{[]byte("correct horse battery"), []byte("different one here")}
	var stderr bytes.Buffer
	src := PassphraseSource{
		Env: func(string) string { return "" }, Interactive: true, Stderr: &stderr,
		ReadPassword: func(int) ([]byte, error) { a := answers[0]; answers = answers[1:]; return a, nil },
	}
	if _, err := src.Read(true); err == nil || !errors.Is(err, ErrPassphraseMismatch) {
		t.Fatalf("err %v", err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Passphrase:")) {
		t.Fatalf("no prompt written: %q", stderr.String())
	}
}

func TestPassphraseNonInteractiveWithoutSourceFails(t *testing.T) {
	_, err := PassphraseSource{Env: func(string) string { return "" }, Interactive: false}.Read(false)
	if !errors.Is(err, ErrNoPassphrase) {
		t.Fatalf("err %v", err)
	}
}
