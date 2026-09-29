// Package snapshot takes a consistent copy of one pb_data directory: the
// database through VACUUM INTO, the list of stored files, and a manifest read
// from the copy. It has no PocketBase import, so a process that is not the
// app can back up an app's data directory.
//
// A delete hold is held from before the copy until Release, so every file the
// copy refers to still exists when a repository reads it.
package snapshot

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/hold"
)

type StoredFile struct {
	Key  string
	Size int64
	Open func() (io.ReadCloser, error)
}

type Snapshot struct {
	Manifest format.Manifest
	DBPath   string
	Files    []StoredFile
	Release  func() error // removes the DB copy, closes the lister, releases the hold; idempotent
}

type Options struct {
	DataDir  string // pb_data
	TmpDir   string // DB copy goes here (created 0700)
	Holder   string // hold owner
	Kind     string
	Source   string
	Instance string
	Vacuum   func(dest string) error                    // nil ⇒ own read-only connection
	Files    func() ([]StoredFile, func() error, error) // nil ⇒ walk DataDir/storage
	Now      func() time.Time
}

func FromDataDir(opts Options) (snap *Snapshot, err error) {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	h, err := hold.Acquire(opts.DataDir, opts.Holder, now)
	if err != nil {
		return nil, err
	}
	var cleanups []func() error
	release := func() error {
		var errs []error
		for i := len(cleanups) - 1; i >= 0; i-- {
			errs = append(errs, cleanups[i]())
		}
		errs = append(errs, h.Release())
		return errors.Join(errs...)
	}
	defer func() {
		if err != nil {
			_ = release()
		}
	}()

	if err = os.MkdirAll(opts.TmpDir, 0o700); err != nil {
		return nil, err
	}
	dest := filepath.Join(opts.TmpDir, randomName()+".db")
	cleanups = append(cleanups, func() error {
		if rerr := os.Remove(dest); rerr != nil && !errors.Is(rerr, os.ErrNotExist) {
			return rerr
		}
		return nil
	})
	vacuum := opts.Vacuum
	if vacuum == nil {
		vacuum = func(d string) error { return VacuumReadOnly(filepath.Join(opts.DataDir, "data.db"), d) }
	}
	if err = vacuum(dest); err != nil {
		return nil, err
	}

	m := format.Manifest{
		Format:   format.FormatV1,
		Created:  now().UTC(),
		Instance: opts.Instance,
		Source:   opts.Source,
		Kind:     opts.Kind,
		Lockfile: format.Lockfile{},
		Packages: map[string]string{},
		Counts:   format.Counts{Collections: map[string]int{}},
	}
	db, err := sql.Open("sqlite", "file:"+dest+"?mode=ro")
	if err != nil {
		return nil, err
	}
	s3 := usesS3(db)
	err = fillManifest(db, &m)
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}

	lister := opts.Files
	if lister == nil {
		if s3 {
			return nil, ErrS3Storage
		}
		lister = func() ([]StoredFile, func() error, error) { return walkLocal(filepath.Join(opts.DataDir, "storage")) }
	}
	files, closeFiles, err := lister()
	if err != nil {
		return nil, err
	}
	if closeFiles != nil {
		cleanups = append(cleanups, closeFiles)
	}
	m.Counts.Files = len(files)
	for _, f := range files {
		m.Counts.Bytes += f.Size
	}

	var once sync.Once
	var releaseErr error
	return &Snapshot{
		Manifest: m,
		DBPath:   dest,
		Files:    files,
		Release: func() error {
			once.Do(func() { releaseErr = release() })
			return releaseErr
		},
	}, nil
}

// VacuumReadOnly copies a live database from another process through a
// read-only connection. SQLite may still create the -shm file when it is
// missing (the app is not running), owned by this process's user; a caller
// that runs as a different user than the app must stop the app from starting
// during the call and fix ownership before the app next starts.
func VacuumReadOnly(src, dest string) error {
	if strings.Contains(dest, "'") {
		return errors.New("backup: snapshot path must not contain a quote")
	}
	db, err := sql.Open("sqlite", "file:"+src+"?mode=ro&_pragma=busy_timeout(10000)")
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("VACUUM INTO '" + dest + "'")
	return err
}

// walkLocal lists every regular file under root. A root that does not exist
// (no storage directory, e.g. a fresh org with nothing uploaded) yields zero
// files rather than an error: WalkDir calls fn once for the root itself with
// its own Lstat error, and returning SkipDir there stops the walk without
// WalkDir turning that error into its own return value.
func walkLocal(root string) ([]StoredFile, func() error, error) {
	var out []StoredFile
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if path == root && errors.Is(err, os.ErrNotExist) {
				return filepath.SkipDir
			}
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		p := path
		out = append(out, StoredFile{
			Key:  filepath.ToSlash(rel),
			Size: info.Size(),
			Open: func() (io.ReadCloser, error) { return os.Open(p) },
		})
		return nil
	})
	return out, nil, err
}

func randomName() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
