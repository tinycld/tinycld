package keychain

import (
	"errors"
	"os"
	"runtime"
	"testing"
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
