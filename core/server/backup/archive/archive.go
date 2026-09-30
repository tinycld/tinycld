// Package archive is the tinycld-backup-v1 archive as a Repository: one
// encrypted, full-copy file per backup, written to a sink and read from a
// stream.
package archive

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
)

const Kind = "archive"

type Repository struct {
	Recipient age.Recipient
	Identity  age.Identity
	Target    func(ctx context.Context, created time.Time) (io.WriteCloser, repo.Ref, error)
	Open      func(ctx context.Context, ref repo.Ref) (io.ReadCloser, error)
	Level     zstd.EncoderLevel
}

// WriterClosedForTesting reports that the archive writer was closed. Only
// this package's own tests set it: an abandoned zstd encoder leaks worker
// goroutines silently, and nothing observable from the sink can show the
// close happened.
var WriterClosedForTesting func()

// ToSink adapts a single, already-open sink into a Target: the caller already
// knows where the archive goes and just wants Put to write to it.
func ToSink(sink io.WriteCloser, ref repo.Ref) func(context.Context, time.Time) (io.WriteCloser, repo.Ref, error) {
	return func(context.Context, time.Time) (io.WriteCloser, repo.Ref, error) { return sink, ref, nil }
}

func (r *Repository) Kind() string { return Kind }

func (r *Repository) Put(ctx context.Context, s *snapshot.Snapshot, progress func(int64)) (res repo.PutResult, err error) {
	sink, ref, err := r.Target(ctx, s.Manifest.Created)
	if err != nil {
		return res, err
	}
	sinkClosed := false
	defer func() {
		if !sinkClosed {
			if cerr := sink.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}
	}()
	level := r.Level
	if level == 0 {
		level = zstd.SpeedDefault
	}
	counter := &countingWriter{w: sink, progress: progress}
	w, err := format.NewWriter(counter, r.Recipient, level)
	if err != nil {
		return res, err
	}
	// The zstd encoder owns goroutines until closed, so an early return still
	// closes the writer; Close is idempotent.
	defer func() {
		cerr := w.Close()
		if WriterClosedForTesting != nil {
			WriterClosedForTesting()
		}
		if cerr != nil && err == nil {
			err = cerr
		}
	}()
	if err = w.WriteManifest(s.Manifest); err != nil {
		return res, err
	}
	if err = writePath(w, format.MemberDB, s.DBPath); err != nil {
		return res, err
	}
	for _, f := range s.Files {
		if err = writeStored(w, f); err != nil {
			return res, err
		}
	}
	if err = w.Close(); err != nil {
		return res, err
	}
	sinkClosed = true
	if err = sink.Close(); err != nil {
		return res, err
	}
	n := counter.total()
	return repo.PutResult{Ref: ref, Bytes: n, UploadedBytes: n, Sha256: w.Sha256()}, nil
}

func writePath(w *format.Writer, name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	return w.WriteFile(name, fi.Size(), f)
}

func writeStored(w *format.Writer, f snapshot.StoredFile) error {
	r, err := f.Open()
	if err != nil {
		return fmt.Errorf("read %s: %w", f.Key, err)
	}
	defer r.Close()
	return w.WriteFile(format.StoragePrefix+f.Key, f.Size, r)
}

func (r *Repository) Manifest(ctx context.Context, ref repo.Ref) (format.Manifest, error) {
	src, err := r.Open(ctx, ref)
	if err != nil {
		return format.Manifest{}, err
	}
	defer src.Close()
	rd, err := format.NewReader(src, r.Identity)
	if err != nil {
		return format.Manifest{}, err
	}
	defer rd.Close()
	return rd.ReadManifest()
}

func (r *Repository) Fetch(ctx context.Context, ref repo.Ref, dir string) error {
	src, err := r.Open(ctx, ref)
	if err != nil {
		return err
	}
	defer src.Close()
	rd, err := format.NewReader(src, r.Identity)
	if err != nil {
		return err
	}
	defer rd.Close()
	if _, err := rd.ReadManifest(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Clean(dir), 0o755); err != nil {
		return err
	}
	return Stage(rd, dir)
}

func (r *Repository) List(context.Context) ([]repo.SnapshotInfo, error) {
	return nil, repo.ErrNotSupported
}

type countingWriter struct {
	w        io.Writer
	progress func(int64)
	mu       sync.Mutex
	n        int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.mu.Lock()
	c.n += int64(n)
	total := c.n
	c.mu.Unlock()
	if c.progress != nil {
		c.progress(total)
	}
	return n, err
}

func (c *countingWriter) total() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
