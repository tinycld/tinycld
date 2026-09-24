package format

import (
	"archive/tar"
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

var (
	ErrChecksum = errors.New("backup: checksum mismatch")
	ErrFormat   = errors.New("backup: not a tinycld backup")
)

type Reader struct {
	zstdR    *zstd.Decoder
	tarR     *tar.Reader
	sums     map[string]string // computed as members are read
	current  *memberReader
	done     bool
	recorded map[string]string // parsed from checksums.txt
}

type memberReader struct {
	name string
	r    io.Reader
	h    hash.Hash
}

func (m *memberReader) Read(p []byte) (int, error) { return m.r.Read(p) }

func NewReader(r io.Reader, identity age.Identity) (*Reader, error) {
	ageR, err := age.Decrypt(r, identity)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	zr, err := zstd.NewReader(ageR)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	return &Reader{zstdR: zr, tarR: tar.NewReader(zr), sums: map[string]string{}}, nil
}

// ReadManifest reads only the first member. Calling it consumes nothing past
// the manifest, so a caller can inspect and stop.
func (r *Reader) ReadManifest() (Manifest, error) {
	hdr, body, err := r.Next()
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	if hdr.Name != MemberManifest {
		return Manifest{}, fmt.Errorf("%w: first member is %q", ErrFormat, hdr.Name)
	}
	var m Manifest
	if err := json.NewDecoder(body).Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("%w: manifest: %v", ErrFormat, err)
	}
	if m.Format != FormatV1 {
		return Manifest{}, fmt.Errorf("%w: format %q", ErrFormat, m.Format)
	}
	return m, nil
}

// Next advances to the next data member. checksums.txt is consumed
// internally and reported as io.EOF.
func (r *Reader) Next() (*tar.Header, io.Reader, error) {
	if r.done {
		return nil, nil, io.EOF
	}
	r.finishCurrent()
	hdr, err := r.tarR.Next()
	if err != nil {
		if err == io.EOF {
			return nil, nil, fmt.Errorf("%w: stream ended before checksums", ErrFormat)
		}
		return nil, nil, err
	}
	if hdr.Name == MemberChecksums {
		if err := r.parseChecksums(); err != nil {
			return nil, nil, err
		}
		r.done = true
		return nil, nil, io.EOF
	}
	h := sha256.New()
	r.current = &memberReader{name: hdr.Name, r: io.TeeReader(r.tarR, h), h: h}
	return hdr, r.current, nil
}

// finishCurrent drains an unread remainder so the hash covers the whole member.
func (r *Reader) finishCurrent() {
	if r.current == nil {
		return
	}
	_, _ = io.Copy(io.Discard, r.current)
	r.sums[r.current.name] = hex.EncodeToString(r.current.h.Sum(nil))
	r.current = nil
}

// sha256HexLen is the fixed width of a hex-encoded sha256 digest. Parsing
// checksums.txt by this fixed prefix (rather than cutting on the first
// "  ") keeps a storage/ key containing two consecutive spaces from being
// misparsed into the hash.
const sha256HexLen = 64

func (r *Reader) parseChecksums() error {
	r.recorded = map[string]string{}
	sc := bufio.NewScanner(r.tarR)
	for sc.Scan() {
		line := sc.Text()
		if len(line) < sha256HexLen+2 || line[sha256HexLen:sha256HexLen+2] != "  " {
			return fmt.Errorf("%w: bad checksum line", ErrFormat)
		}
		r.recorded[line[sha256HexLen+2:]] = line[:sha256HexLen]
	}
	return sc.Err()
}

// Verify compares every member read against checksums.txt in both
// directions: every recorded hash must match what was actually read, and
// every member actually read must have a recorded line — a checksums.txt
// silently missing a member's entry is a failure, not a pass. Valid only
// after Next returned io.EOF.
func (r *Reader) Verify() error {
	if !r.done {
		return fmt.Errorf("backup: verify called before end of stream")
	}
	for name, want := range r.recorded {
		got, ok := r.sums[name]
		if !ok {
			return fmt.Errorf("%w: %s not read", ErrChecksum, name)
		}
		if got != want {
			return fmt.Errorf("%w: %s", ErrChecksum, name)
		}
	}
	for name := range r.sums {
		if _, ok := r.recorded[name]; !ok {
			return fmt.Errorf("%w: %s not in checksums", ErrChecksum, name)
		}
	}
	return nil
}

func (r *Reader) Close() { r.zstdR.Close() }
