package snapshot

import (
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"tinycld.org/core/backup/hold"
)

// fixture builds a pb_data with a real SQLite DB carrying the two tables the
// manifest reads, one user collection with rows, and one stored file.
func fixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "pb_data")
	if err := os.MkdirAll(filepath.Join(dir, "storage", "c1", "r1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "storage", "c1", "r1", "a.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, q := range []string{
		"PRAGMA journal_mode=WAL",
		"CREATE TABLE _collections (id TEXT, name TEXT, system BOOLEAN)",
		"CREATE TABLE _params (id TEXT, value TEXT)",
		"INSERT INTO _collections VALUES ('1','notes',0),('2','_superusers',1),('3','pkg_registry',0)",
		"CREATE TABLE notes (id TEXT)",
		"INSERT INTO notes VALUES ('a'),('b'),('c')",
		"CREATE TABLE pkg_registry (slug TEXT, version TEXT, npm_package TEXT, status TEXT)",
		"INSERT INTO pkg_registry VALUES ('core','1.2.3','tinycld@1.2.3','bundled'),('widgets','1.0.0','@x/widgets@1.0.0','installed'),('gone','0.1.0','@x/gone@0.1.0','available')",
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	return dir
}

func TestFromDataDirBuildsManifestFromTheCopy(t *testing.T) {
	dir := fixture(t)
	s, err := FromDataDir(Options{DataDir: dir, TmpDir: t.TempDir(), Holder: "test", Kind: "scheduled", Source: "docker", Instance: "https://acme.example"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Release()
	m := s.Manifest
	if m.Core != "1.2.3" || m.Lockfile["tinycld"] != "tinycld@1.2.3" {
		t.Fatalf("core = %q lockfile = %v", m.Core, m.Lockfile)
	}
	if m.Packages["widgets"] != "1.0.0" || m.Packages["gone"] != "" {
		t.Fatalf("packages = %v", m.Packages)
	}
	if m.Counts.Collections["notes"] != 3 {
		t.Fatalf("counts = %v", m.Counts.Collections)
	}
	if _, ok := m.Counts.Collections["_superusers"]; ok {
		t.Fatal("system collection counted")
	}
	if m.Counts.Files != 1 || m.Counts.Bytes != 5 || len(s.Files) != 1 || s.Files[0].Key != "c1/r1/a.txt" {
		t.Fatalf("files = %+v counts = %+v", s.Files, m.Counts)
	}
	r, err := s.Files[0].Open()
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(r)
	r.Close()
	if string(body) != "hello" {
		t.Fatalf("body = %q", body)
	}
}

func TestFromDataDirHoldsDeletesUntilRelease(t *testing.T) {
	dir := fixture(t)
	s, err := FromDataDir(Options{DataDir: dir, TmpDir: t.TempDir(), Holder: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if st, ok, _ := hold.Read(dir); !ok || st.Holder != "test" {
		t.Fatal("no hold while the snapshot is open")
	}
	db := s.DBPath
	if err := s.Release(); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := hold.Read(dir); ok {
		t.Fatal("hold survived Release")
	}
	if _, err := os.Stat(db); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("DB copy survived Release")
	}
	if err := s.Release(); err != nil {
		t.Fatalf("second Release: %v", err)
	}
}

func TestFromDataDirRefusesWhileAnotherHolderHolds(t *testing.T) {
	dir := fixture(t)
	h, _ := hold.Acquire(dir, "other", nil)
	defer h.Release()
	if _, err := FromDataDir(Options{DataDir: dir, TmpDir: t.TempDir(), Holder: "test"}); !errors.Is(err, hold.ErrHeld) {
		t.Fatalf("err = %v", err)
	}
}

func TestFromDataDirWithNoStorageDirYieldsZeroFiles(t *testing.T) {
	dir := fixture(t)
	if err := os.RemoveAll(filepath.Join(dir, "storage")); err != nil {
		t.Fatal(err)
	}
	s, err := FromDataDir(Options{DataDir: dir, TmpDir: t.TempDir(), Holder: "test"})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Release()
	if len(s.Files) != 0 || s.Manifest.Counts.Files != 0 {
		t.Fatalf("files = %+v counts = %+v", s.Files, s.Manifest.Counts)
	}
}

func TestFromDataDirRefusesS3StorageWithoutALister(t *testing.T) {
	dir := fixture(t)
	db, _ := sql.Open("sqlite", filepath.Join(dir, "data.db"))
	_, _ = db.Exec(`INSERT INTO _params VALUES ('settings','{"s3":{"enabled":true}}')`)
	db.Close()
	if _, err := FromDataDir(Options{DataDir: dir, TmpDir: t.TempDir(), Holder: "test"}); !errors.Is(err, ErrS3Storage) {
		t.Fatalf("err = %v", err)
	}
	if _, ok, _ := hold.Read(dir); ok {
		t.Fatal("a refused snapshot left its hold")
	}
}

func TestWalkLocalWithMissingRootYieldsNoFilesAndNoError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "does-not-exist")
	files, closeFn, err := walkLocal(root)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if closeFn != nil {
		t.Fatal("walkLocal returned a non-nil closer")
	}
	if len(files) != 0 {
		t.Fatalf("files = %+v", files)
	}
}
