// Package pbs stores backups in a Proxmox Backup Server datastore. Every run
// is a full snapshot; PBS keeps only the chunks it does not already have, so a
// daily backup costs about the size of what changed.
package pbs

import (
	"archive/tar"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gopbs "github.com/osshield/gopbs/pbs"

	"tinycld.org/core/backup/arm"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/backup/snapshot"
)

const (
	Kind         = "pbs"
	backupType   = "host"
	fileManifest = "manifest.blob"
	fileDB       = "data.db"
	fileStorage  = "storage.tar"
)

// Register adds the pbs kind unless it is already there. It checks the
// registry rather than using sync.Once, so a test that resets the registry
// can register it again.
func Register() {
	if !slices.Contains(repo.Kinds(), Kind) {
		repo.Register(Kind, Open)
	}
}

// putState is the progress state for one in-flight Put. It is swapped in
// atomically so the OnUploadProgress callback (which may run on the client's
// upload goroutines concurrently with a caller starting or finishing a Put)
// never observes a torn or reset sync.Map.
type putState struct {
	progress func(int64)
	sizes    sync.Map // archive name → uint64 bytes indexed so far
}

type Repository struct {
	cfg     Config
	client  *gopbs.Client
	current atomic.Pointer[putState]
}

func Open(raw json.RawMessage) (repo.Repository, error) {
	cfg, err := ParseConfig(raw)
	if err != nil {
		return nil, err
	}
	crypt, err := cfg.crypt()
	if err != nil {
		return nil, err
	}
	r := &Repository{cfg: cfg}
	client, err := gopbs.NewClient(gopbs.Config{
		BaseURL:     cfg.baseURL(),
		Auth:        gopbs.TokenAuth{AuthID: cfg.AuthID, Secret: cfg.Secret},
		Fingerprint: cfg.Fingerprint,
		Datastore:   cfg.Datastore,
		Namespace:   cfg.Namespace,
		Crypt:       crypt,
		// Progress reports the logical bytes indexed so far: the sum of
		// UploadStats.Size across the two upload streams (data.db and the
		// storage tar), not bytes actually sent over the wire — a
		// deduplicated chunk still advances this total.
		OnUploadProgress: func(name string, st gopbs.UploadStats, _ bool) {
			st2 := r.current.Load()
			if st2 == nil {
				return
			}
			st2.sizes.Store(name, st.Size)
			if st2.progress == nil {
				return
			}
			var total uint64
			st2.sizes.Range(func(_, v any) bool { total += v.(uint64); return true })
			st2.progress(int64(total))
		},
	})
	if err != nil {
		return nil, err
	}
	r.client = client
	return r, nil
}

func (r *Repository) Kind() string { return Kind }

func refFor(id string, t time.Time) repo.Ref {
	return repo.Ref(backupType + "/" + id + "/" + t.UTC().Format(time.RFC3339))
}

func parseRef(ref repo.Ref) (gopbs.SnapshotRef, error) {
	parts := strings.SplitN(string(ref), "/", 3)
	if len(parts) != 3 {
		return gopbs.SnapshotRef{}, fmt.Errorf("backup: %q is not a PBS snapshot reference", ref)
	}
	t, err := time.Parse(time.RFC3339, parts[2])
	if err != nil {
		return gopbs.SnapshotRef{}, fmt.Errorf("backup: %q is not a PBS snapshot reference", ref)
	}
	return gopbs.SnapshotRef{Type: parts[0], ID: parts[1], Time: t}, nil
}

func (r *Repository) Put(ctx context.Context, s *snapshot.Snapshot, progress func(int64)) (res repo.PutResult, err error) {
	r.current.Store(&putState{progress: progress})
	defer r.current.Store(nil)

	ref := gopbs.SnapshotRef{Type: backupType, ID: r.cfg.BackupID, Time: s.Manifest.Created}
	sess, err := r.client.StartBackup(ctx, ref)
	if err != nil {
		return res, err
	}
	finished := false
	defer func() {
		if !finished {
			_ = sess.Abort()
		}
	}()

	manifest, err := json.Marshal(s.Manifest)
	if err != nil {
		return res, err
	}
	if err = sess.UploadBlob(ctx, gopbs.NewBlobEncoder(), fileManifest, manifest, true); err != nil {
		return res, err
	}

	db, err := os.Open(s.DBPath)
	if err != nil {
		return res, err
	}
	dbStats, err := sess.UploadStream(ctx, fileDB, db)
	_ = db.Close()
	if err != nil {
		return res, err
	}

	pr, pw := io.Pipe()
	go func() { _ = pw.CloseWithError(writeTar(pw, s.Files)) }()
	stStats, err := sess.UploadStream(ctx, fileStorage, pr)
	_ = pr.Close()
	if err != nil {
		return res, err
	}

	if err = sess.Finish(ctx); err != nil {
		return res, err
	}
	finished = true
	return repo.PutResult{
		Ref:           refFor(r.cfg.BackupID, s.Manifest.Created),
		Bytes:         int64(dbStats.Size + stStats.Size),
		UploadedBytes: int64(dbStats.NewBytes + stStats.NewBytes),
	}, nil
}

// writeTar streams the stored files as a plain tar. Names are storage keys, so
// ExtractTar puts them back under storage/.
func writeTar(w io.Writer, files []snapshot.StoredFile) error {
	tw := tar.NewWriter(w)
	for _, f := range files {
		hdr := &tar.Header{Name: f.Key, Mode: 0o644, Size: f.Size, Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("read %s: %w", f.Key, err)
		}
		n, err := io.Copy(tw, rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		if n != f.Size {
			return fmt.Errorf("backup: %s: read %d bytes, expected %d", f.Key, n, f.Size)
		}
	}
	return tw.Close()
}

func (r *Repository) reader(ctx context.Context, ref repo.Ref) (*gopbs.ReaderSession, error) {
	sref, err := parseRef(ref)
	if err != nil {
		return nil, err
	}
	return r.client.StartReader(ctx, sref)
}

func (r *Repository) Manifest(ctx context.Context, ref repo.Ref) (format.Manifest, error) {
	rs, err := r.reader(ctx, ref)
	if err != nil {
		return format.Manifest{}, err
	}
	defer rs.Close()
	raw, err := rs.DownloadBlob(ctx, fileManifest)
	if err != nil {
		return format.Manifest{}, err
	}
	var m format.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return m, fmt.Errorf("%w: manifest: %v", format.ErrFormat, err)
	}
	if m.Format != format.FormatV1 {
		return m, fmt.Errorf("%w: format %q", format.ErrFormat, m.Format)
	}
	return m, nil
}

func (r *Repository) Fetch(ctx context.Context, ref repo.Ref, dir string) error {
	rs, err := r.reader(ctx, ref)
	if err != nil {
		return err
	}
	defer rs.Close()
	if err := os.MkdirAll(filepath.Join(dir, "storage"), 0o755); err != nil {
		return err
	}
	db, err := rs.OpenDynamicIndex(ctx, fileDB+".didx")
	if err != nil {
		return err
	}
	err = arm.WriteMember(filepath.Join(dir, format.MemberDB), db)
	_ = db.Close()
	if err != nil {
		return err
	}
	st, err := rs.OpenDynamicIndex(ctx, fileStorage+".didx")
	if err != nil {
		return err
	}
	defer st.Close()
	return arm.ExtractTar(st, filepath.Join(dir, "storage"))
}

func (r *Repository) List(ctx context.Context) ([]repo.SnapshotInfo, error) {
	snaps, err := r.client.ListSnapshots(ctx, backupType, r.cfg.BackupID)
	if err != nil {
		return nil, err
	}
	out := make([]repo.SnapshotInfo, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, repo.SnapshotInfo{Ref: refFor(s.Ref.ID, s.Ref.Time), Created: s.Ref.Time.UTC(), Bytes: int64(s.Size)})
	}
	return out, nil
}
