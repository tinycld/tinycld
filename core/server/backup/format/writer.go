package format

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

// Writer streams one archive. Members must be written in the fixed order:
// WriteManifest, then data.db and storage files via WriteFile, then Close.
type Writer struct {
	outerHash hash.Hash // sha256 of the encrypted bytes, reported to the ledger
	ageW      io.WriteCloser
	zstdW     *zstd.Encoder
	tarW      *tar.Writer
	sums      map[string]string
	order     []string
	started   bool
	closed    bool
	closeErr  error
}

func NewWriter(w io.Writer, recipient age.Recipient, level zstd.EncoderLevel) (*Writer, error) {
	h := sha256.New()
	ageW, err := age.Encrypt(io.MultiWriter(w, h), recipient)
	if err != nil {
		return nil, fmt.Errorf("backup: encrypt: %w", err)
	}
	zw, err := zstd.NewWriter(ageW, zstd.WithEncoderLevel(level))
	if err != nil {
		return nil, fmt.Errorf("backup: compress: %w", err)
	}
	return &Writer{outerHash: h, ageW: ageW, zstdW: zw, tarW: tar.NewWriter(zw), sums: map[string]string{}}, nil
}

func (w *Writer) WriteManifest(m Manifest) error {
	if w.started {
		return fmt.Errorf("backup: manifest must be the first member")
	}
	w.started = true
	m.Format = FormatV1
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return w.WriteFile(MemberManifest, int64(len(body)), strings.NewReader(string(body)))
}

func (w *Writer) WriteFile(name string, size int64, r io.Reader) error {
	if !w.started {
		return fmt.Errorf("backup: write the manifest first")
	}
	hdr := &tar.Header{Name: name, Mode: 0o600, Size: size, ModTime: time.Now().UTC(), Typeflag: tar.TypeReg}
	if err := w.tarW.WriteHeader(hdr); err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(w.tarW, h), r)
	if err != nil {
		return err
	}
	if n != size {
		return fmt.Errorf("backup: %s: wrote %d bytes, header said %d", name, n, size)
	}
	w.sums[name] = hex.EncodeToString(h.Sum(nil))
	w.order = append(w.order, name)
	return nil
}

// Close writes checksums.txt and closes the tar/zstd/age pipeline. It is
// idempotent: a second call returns the same error the first call produced
// (or nil, if the first call succeeded) rather than silently reporting
// success on a call that never re-runs the writes.
func (w *Writer) Close() error {
	if w.closed {
		return w.closeErr
	}
	w.closed = true
	w.closeErr = w.close()
	return w.closeErr
}

func (w *Writer) close() error {
	var b strings.Builder
	for _, name := range w.order {
		fmt.Fprintf(&b, "%s  %s\n", w.sums[name], name)
	}
	body := b.String()
	hdr := &tar.Header{Name: MemberChecksums, Mode: 0o600, Size: int64(len(body)), ModTime: time.Now().UTC(), Typeflag: tar.TypeReg}
	if err := w.tarW.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := io.WriteString(w.tarW, body); err != nil {
		return err
	}
	if err := w.tarW.Close(); err != nil {
		return err
	}
	if err := w.zstdW.Close(); err != nil {
		return err
	}
	return w.ageW.Close()
}

func (w *Writer) Sha256() string { return hex.EncodeToString(w.outerHash.Sum(nil)) }

// tamperChecksum corrupts a recorded hash. Test-only; keeps the tamper test
// honest without a second code path for writing archives.
func (w *Writer) tamperChecksum(name string) { w.sums[name] = strings.Repeat("0", 64) }

// dropChecksum removes a member's line from checksums.txt. Test-only; lets
// a test produce an archive whose checksums.txt is missing a member's
// entry without a second code path for writing archives.
func (w *Writer) dropChecksum(name string) {
	for i, n := range w.order {
		if n == name {
			w.order = append(w.order[:i], w.order[i+1:]...)
			break
		}
	}
}
