package keychain

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestFileStoreRoundTrip(t *testing.T) {
	fs := fileStore{dir: t.TempDir()}

	if _, err := fs.Get("localhost:7110"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get err = %v, want ErrNotFound", err)
	}
	if err := fs.Set("localhost:7110", `{"access_token":"x"}`); err != nil {
		t.Fatal(err)
	}
	v, err := fs.Get("localhost:7110")
	if err != nil || v != `{"access_token":"x"}` {
		t.Fatalf("get = %q, %v", v, err)
	}
	if err := fs.Delete("localhost:7110"); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.Get("localhost:7110"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("post-delete get err = %v, want ErrNotFound", err)
	}
	// idempotent delete
	if err := fs.Delete("localhost:7110"); err != nil {
		t.Fatalf("second delete: %v", err)
	}
}

func TestFileStorePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permissions")
	}
	fs := fileStore{dir: t.TempDir()}
	if err := fs.Set("ctx", "secret"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(fs.path("ctx"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("credential mode = %o, want 600", perm)
	}
}

func TestFileStoreEscapesAccountNames(t *testing.T) {
	fs := fileStore{dir: t.TempDir()}
	// ':' is invalid in Windows filenames and '/' would escape the dir.
	for _, account := range []string{"localhost:7110", "weird/../name"} {
		if err := fs.Set(account, "v"); err != nil {
			t.Fatalf("set %q: %v", account, err)
		}
		v, err := fs.Get(account)
		if err != nil || v != "v" {
			t.Fatalf("get %q = %q, %v", account, v, err)
		}
	}
	entries, err := os.ReadDir(fs.dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 files inside the store dir, got %d", len(entries))
	}
}

func TestMemStore(t *testing.T) {
	s := NewMemStore()
	if _, err := s.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if err := s.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get("a"); v != "1" {
		t.Fatalf("v = %q", v)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestJSONHelpers(t *testing.T) {
	s := NewMemStore()
	type tok struct {
		Access string `json:"access_token"`
	}
	if err := SetJSON(s, "ctx", tok{Access: "abc"}); err != nil {
		t.Fatal(err)
	}
	var out tok
	if err := GetJSON(s, "ctx", &out); err != nil {
		t.Fatal(err)
	}
	if out.Access != "abc" {
		t.Fatalf("out = %+v", out)
	}
	if err := GetJSON(s, "missing", &out); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

// stubKeyring swaps the keyring seams for the test and restores them after.
func stubKeyring(
	t *testing.T,
	get func(service, account string) (string, error),
	set func(service, account, value string) error,
	del func(service, account string) error,
) {
	t.Helper()
	origGet, origSet, origDel := keyringGet, keyringSet, keyringDelete
	keyringGet, keyringSet, keyringDelete = get, set, del
	t.Cleanup(func() { keyringGet, keyringSet, keyringDelete = origGet, origSet, origDel })
}

func TestSystemStoreFallsBackWhenKeychainRefusesWrites(t *testing.T) {
	// The OS keychain can answer the read probe yet refuse writes (sandboxed
	// process, locked login session — macOS `security` exits 154). By the
	// time Set runs after a device login the server has already activated
	// the grant, so dropping the token would strand a live credential.
	kcMem := map[string]string{}
	stubKeyring(t,
		func(_, account string) (string, error) {
			v, ok := kcMem[account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return v, nil
		},
		func(_, _, _ string) error { return errors.New("exit status 154") },
		func(_, account string) error {
			delete(kcMem, account)
			return nil
		},
	)

	dir := t.TempDir()
	var warn bytes.Buffer
	s := Open(dir, &warn)

	// The probe read succeeded, so Open picked the system store — the write
	// failure must degrade to the file store, not surface as an error.
	if err := s.Set("ctx1", `{"t":"v1"}`); err != nil {
		t.Fatalf("Set must fall back to the file store, got %v", err)
	}
	if !strings.Contains(warn.String(), "keychain write failed") {
		t.Fatalf("expected a one-time warning, got %q", warn.String())
	}
	v, err := s.Get("ctx1")
	if err != nil || v != `{"t":"v1"}` {
		t.Fatalf("Get after degraded Set = %q, %v", v, err)
	}

	// Location must name the FILE, not the keychain. `auth login` prints this,
	// and it used to say "keychain" unconditionally — directly under the
	// warning above saying the keychain write had just failed.
	loc := s.Location("ctx1")
	if strings.Contains(loc, "keychain") {
		t.Errorf("Location after a degraded write = %q, must not claim the keychain", loc)
	}
	if !strings.Contains(loc, dir) {
		t.Errorf("Location = %q, want the credential file under %s", loc, dir)
	}

	// Second write must not warn again.
	warn.Reset()
	if err := s.Set("ctx1", `{"t":"v2"}`); err != nil {
		t.Fatal(err)
	}
	if warn.String() != "" {
		t.Fatalf("warning must be one-time, got %q", warn.String())
	}

	// Delete clears the file copy too.
	if err := s.Delete("ctx1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("ctx1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after Delete, err = %v", err)
	}
}

func TestSystemStoreKeychainWriteClearsStaleFileCopy(t *testing.T) {
	// A session where the keychain works again must not leave an older file
	// credential around for Get to resurrect after the keychain entry is
	// later removed.
	kcMem := map[string]string{}
	stubKeyring(t,
		func(_, account string) (string, error) {
			v, ok := kcMem[account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return v, nil
		},
		func(_, account, value string) error {
			kcMem[account] = value
			return nil
		},
		func(_, account string) error {
			delete(kcMem, account)
			return nil
		},
	)

	dir := t.TempDir()
	var warn bytes.Buffer
	s := Open(dir, &warn)

	// Simulate a stale file credential from an earlier degraded session.
	stale := fileStore{dir: filepath.Join(dir, "credentials")}
	if err := stale.Set("ctx1", `{"t":"old"}`); err != nil {
		t.Fatal(err)
	}

	if err := s.Set("ctx1", `{"t":"new"}`); err != nil {
		t.Fatal(err)
	}

	// A healthy write really is in the keychain, so Location must say so —
	// otherwise reporting the truth on the degraded path would just have
	// traded one wrong message for another.
	if loc := s.Location("ctx1"); loc != "keychain" {
		t.Errorf("Location after a successful keychain write = %q, want \"keychain\"", loc)
	}

	delete(kcMem, "ctx1")
	if _, err := s.Get("ctx1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale file copy resurrected: err = %v", err)
	}
}

// One context degrading must not mislabel another that succeeded — the reason
// the flag is per-account rather than a single bool on the store.
func TestSystemStoreLocationIsPerAccount(t *testing.T) {
	kcMem := map[string]string{}
	stubKeyring(t,
		func(_, account string) (string, error) {
			v, ok := kcMem[account]
			if !ok {
				return "", keyring.ErrNotFound
			}
			return v, nil
		},
		func(_, account, value string) error {
			// Only this one context is refused.
			if account == "refused" {
				return errors.New("exit status 154")
			}
			kcMem[account] = value
			return nil
		},
		func(_, account string) error {
			delete(kcMem, account)
			return nil
		},
	)

	var warn bytes.Buffer
	s := Open(t.TempDir(), &warn)
	if err := s.Set("refused", "a"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("accepted", "b"); err != nil {
		t.Fatal(err)
	}

	if loc := s.Location("accepted"); loc != "keychain" {
		t.Errorf("accepted context Location = %q, want \"keychain\"", loc)
	}
	if loc := s.Location("refused"); loc == "keychain" {
		t.Errorf("refused context Location = %q, must not claim the keychain", loc)
	}
}
