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

type Store interface {
	Get(account string) (string, error)
	Set(account, value string) error
	// Delete is idempotent: removing an absent credential is not an error.
	Delete(account string) error
}

// Open returns the OS keychain when one responds, else a file store with a
// one-time warning on warn. The probe read distinguishes "backend works but
// has no such item" (fine) from "no backend at all" (fall back).
func Open(configDir string, warn io.Writer) Store {
	_, err := keyring.Get(service, "tinycld-probe")
	if err == nil || errors.Is(err, keyring.ErrNotFound) {
		return systemStore{}
	}
	dir := filepath.Join(configDir, "credentials")
	fmt.Fprintf(warn, "warning: OS keychain unavailable (%v); storing credentials in %s (mode 0600)\n", err, dir)
	return fileStore{dir: dir}
}

type systemStore struct{}

func (systemStore) Get(account string) (string, error) {
	v, err := keyring.Get(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (systemStore) Set(account, value string) error {
	return keyring.Set(service, account, value)
}

func (systemStore) Delete(account string) error {
	err := keyring.Delete(service, account)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
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
