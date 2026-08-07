// Package keychain stores per-context credentials in the OS keychain
// (macOS Keychain, Windows Credential Manager, Linux Secret Service),
// falling back to 0600 files under the config dir when no keychain is
// available (headless Linux, CI).
package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

const service = "tinycld"

var ErrNotFound = errors.New("keychain: credential not found")

// Seams over the keyring library so tests can simulate a keychain that
// answers reads but refuses writes — go-keyring's mock cannot split the two.
var (
	keyringGet    = keyring.Get
	keyringSet    = keyring.Set
	keyringDelete = keyring.Delete
)

type Store interface {
	Get(account string) (string, error)
	Set(account, value string) error
	// Delete is idempotent: removing an absent credential is not an error.
	Delete(account string) error
}

// Open returns the OS keychain when one responds, else a file store with a
// one-time warning on warn. The probe read distinguishes "backend works but
// has no such item" (fine) from "no backend at all" (fall back).
//
// The keychain store still degrades per-write: a keychain can pass the read
// probe yet refuse writes (sandboxed processes, locked login sessions), and
// by the time Set runs after a device login the server has already activated
// the grant — failing would strand a live credential the user just approved.
func Open(configDir string, warn io.Writer) Store {
	file := fileStore{dir: filepath.Join(configDir, "credentials")}
	_, err := keyringGet(service, "tinycld-probe")
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return &systemStore{file: file, warn: warn}
	}
	fmt.Fprintf(warn, "warning: OS keychain unavailable (%v); storing credentials in %s (mode 0600)\n", err, file.dir)
	return file
}

// systemStore prefers the OS keychain and falls back to the file store when
// the keychain refuses a write. Reads consult both places, so a credential
// keeps working regardless of where an earlier Set landed.
type systemStore struct {
	file   fileStore
	warn   io.Writer
	warned bool
}

func (s *systemStore) Get(account string) (string, error) {
	v, err := keyringGet(service, account)
	if err == nil {
		return v, nil
	}
	if fv, ferr := s.file.Get(account); ferr == nil {
		return fv, nil
	}
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return "", err
}

func (s *systemStore) Set(account, value string) error {
	err := keyringSet(service, account, value)
	if err == nil {
		// A keychain write supersedes any stale file copy from a degraded
		// earlier session; drop it so Get cannot resurrect old credentials.
		s.file.Delete(account)
		return nil
	}
	if !s.warned {
		s.warned = true
		fmt.Fprintf(s.warn,
			"warning: OS keychain write failed (%v); storing credentials in %s (mode 0600)\n",
			err, s.file.dir)
	}
	return s.file.Set(account, value)
}

func (s *systemStore) Delete(account string) error {
	kerr := keyringDelete(service, account)
	if errors.Is(kerr, keyring.ErrNotFound) {
		kerr = nil
	}
	ferr := s.file.Delete(account)
	if kerr != nil {
		return kerr
	}
	return ferr
}

type fileStore struct{ dir string }

func (f fileStore) path(account string) string {
	// Context names carry ':' (host:port), which Windows filenames reject.
	return filepath.Join(f.dir, url.PathEscape(account)+".json")
}

func (f fileStore) Get(account string) (string, error) {
	data, err := os.ReadFile(f.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (f fileStore) Set(account, value string) error {
	if err := os.MkdirAll(f.dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.dir, "cred-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path(account))
}

func (f fileStore) Delete(account string) error {
	err := os.Remove(f.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// MemStore is an in-memory Store for tests.
type MemStore struct{ m map[string]string }

func NewMemStore() *MemStore { return &MemStore{m: map[string]string{}} }

func (s *MemStore) Get(account string) (string, error) {
	v, ok := s.m[account]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *MemStore) Set(account, value string) error {
	s.m[account] = value
	return nil
}

func (s *MemStore) Delete(account string) error {
	delete(s.m, account)
	return nil
}

// GetJSON / SetJSON are conveniences for the token payloads every caller
// stores — one JSON document per context.
func GetJSON(s Store, account string, out any) error {
	v, err := s.Get(account)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(v), out)
}

func SetJSON(s Store, account string, in any) error {
	data, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return s.Set(account, string(data))
}
