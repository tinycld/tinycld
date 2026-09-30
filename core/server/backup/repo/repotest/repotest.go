// Package repotest is the contract every Repository must meet.
package repotest

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
)

type Options struct{ Dedup bool }

var bulk = func() []byte {
	b := make([]byte, 3<<20)
	_, _ = rand.Read(b)
	return b
}()

// Fixture is a snapshot a repository treats as bytes: data.db need not be a
// real database at this layer.
func Fixture(t *testing.T) *snapshot.Snapshot {
	t.Helper()
	dir := t.TempDir()
	db := filepath.Join(dir, "data.db")
	if err := os.WriteFile(db, append([]byte("SQLite format 3\x00"), bulk[:4096]...), 0o600); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{"c1/r1/a.txt": []byte("hello"), "c1/r2/bulk.bin": bulk}
	var stored []snapshot.StoredFile
	var total int64
	for _, key := range []string{"c1/r1/a.txt", "c1/r2/bulk.bin"} {
		key := key
		body := files[key]
		stored = append(stored, snapshot.StoredFile{
			Key: key, Size: int64(len(body)),
			Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil },
		})
		total += int64(len(body))
	}
	return &snapshot.Snapshot{
		Manifest: format.Manifest{
			Format: format.FormatV1, Created: time.Now().UTC().Truncate(time.Second),
			Instance: "https://acme.example", Source: "docker", Kind: "scheduled", Core: "1.2.3",
			Lockfile: format.Lockfile{"tinycld": "tinycld@1.2.3"}, Packages: map[string]string{},
			Counts: format.Counts{Collections: map[string]int{"notes": 3}, Files: 2, Bytes: total},
		},
		DBPath:  db,
		Files:   stored,
		Release: func() error { return nil },
	}
}

func Run(t *testing.T, open func(t *testing.T) repo.Repository, opts Options) {
	ctx := context.Background()

	t.Run("round trip", func(t *testing.T) {
		r := open(t)
		snap := Fixture(t)
		var sent atomic.Int64
		res, err := r.Put(ctx, snap, func(n int64) { sent.Store(n) })
		if err != nil {
			t.Fatal(err)
		}
		if res.Ref == "" || res.Bytes <= 0 || sent.Load() <= 0 {
			t.Fatalf("result = %+v sent = %d", res, sent.Load())
		}
		m, err := r.Manifest(ctx, res.Ref)
		if err != nil {
			t.Fatal(err)
		}
		if !m.Created.Equal(snap.Manifest.Created) || m.Core != "1.2.3" || m.Counts.Collections["notes"] != 3 {
			t.Fatalf("manifest = %+v", m)
		}
		dir := t.TempDir()
		if err := r.Fetch(ctx, res.Ref, dir); err != nil {
			t.Fatal(err)
		}
		wantDB, _ := os.ReadFile(snap.DBPath)
		gotDB, _ := os.ReadFile(filepath.Join(dir, "data.db"))
		if !bytes.Equal(wantDB, gotDB) {
			t.Fatal("data.db differs after fetch")
		}
		for _, f := range snap.Files {
			rc, _ := f.Open()
			want, _ := io.ReadAll(rc)
			rc.Close()
			got, err := os.ReadFile(filepath.Join(dir, "storage", filepath.FromSlash(f.Key)))
			if err != nil || !bytes.Equal(want, got) {
				t.Fatalf("%s differs after fetch: %v", f.Key, err)
			}
		}
	})

	t.Run("list", func(t *testing.T) {
		r := open(t)
		res, err := r.Put(ctx, Fixture(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		list, err := r.List(ctx)
		if errors.Is(err, repo.ErrNotSupported) {
			t.Skip("repository does not list")
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range list {
			if s.Ref == res.Ref {
				return
			}
		}
		t.Fatalf("ref %s not in %v", res.Ref, list)
	})

	t.Run("dedup", func(t *testing.T) {
		if !opts.Dedup {
			t.Skip("repository does not deduplicate")
		}
		r := open(t)
		first, err := r.Put(ctx, Fixture(t), nil)
		if err != nil {
			t.Fatal(err)
		}
		if first.UploadedBytes <= 0 {
			t.Fatalf("first put uploaded %d bytes", first.UploadedBytes)
		}
		second := Fixture(t)
		// Two snapshots in one group need distinct second-precision times.
		second.Manifest.Created = time.Now().UTC().Truncate(time.Second).Add(2 * time.Second)
		res, err := r.Put(ctx, second, nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.UploadedBytes >= res.Bytes/2 {
			t.Fatalf("second put uploaded %d of %d bytes", res.UploadedBytes, res.Bytes)
		}
	})
}
