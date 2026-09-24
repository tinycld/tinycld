package format

import (
	"bytes"
	"crypto/rand"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/klauspost/compress/zstd"
)

func testRecipient(t *testing.T) (age.Recipient, age.Identity) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	return id.Recipient(), id
}

func sampleManifest() Manifest {
	return Manifest{
		Format: FormatV1, Created: time.Unix(1_700_000_000, 0).UTC(), Instance: "inst", Source: "docker",
		Kind: "manual", Core: "1.2.3", Lockfile: Lockfile{"tinycld": "1.2.3", "mail": "github:tinycld/mail#v1.0.0"},
		Packages: map[string]string{"mail": "1.0.0"},
		Counts:   Counts{Collections: map[string]int{"users": 2}, Files: 1, Bytes: 5},
	}
}

func buildArchive(t *testing.T, rcpt age.Recipient) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := NewWriter(&buf, rcpt, zstd.SpeedDefault)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteManifest(sampleManifest()); err != nil {
		t.Fatal(err)
	}
	// A large, incompressible data.db member defeats zstd/age read-ahead
	// buffering, so TestReadManifestStopsEarly can prove ReadManifest
	// doesn't consume the whole stream. Repeated bytes compress away to
	// almost nothing, so use random data instead.
	big := make([]byte, 4<<20)
	if _, err := rand.Read(big); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("data.db", int64(len(big)), bytes.NewReader(big)); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("storage/col/rec/file.txt", 3, strings.NewReader("abc")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestRoundTrip(t *testing.T) {
	rcpt, id := testRecipient(t)
	data := buildArchive(t, rcpt)

	r, err := NewReader(bytes.NewReader(data), id)
	if err != nil {
		t.Fatal(err)
	}
	m, err := r.ReadManifest()
	if err != nil || m.Core != "1.2.3" || m.Lockfile["mail"] == "" {
		t.Fatalf("manifest: %+v %v", m, err)
	}
	var names []string
	var sizes []int
	for {
		hdr, body, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(body)
		names = append(names, hdr.Name)
		sizes = append(sizes, len(b))
	}
	if strings.Join(names, ",") != "data.db,storage/col/rec/file.txt" {
		t.Fatalf("members %v", names)
	}
	if sizes[0] != 4<<20 || sizes[1] != 3 {
		t.Fatalf("sizes %v", sizes)
	}
	if err := r.Verify(); err != nil {
		t.Fatal(err)
	}
}

func TestReadManifestStopsEarly(t *testing.T) {
	rcpt, id := testRecipient(t)
	data := buildArchive(t, rcpt)
	counting := &countingReader{r: bytes.NewReader(data)}
	r, err := NewReader(counting, id)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReadManifest(); err != nil {
		t.Fatal(err)
	}
	if n := counting.count(); n >= len(data) {
		t.Fatalf("read whole stream (%d of %d) just for the manifest", n, len(data))
	}
}

func TestTamperedMemberFailsVerify(t *testing.T) {
	rcpt, id := testRecipient(t)
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, rcpt, zstd.SpeedDefault)
	_ = w.WriteManifest(sampleManifest())
	_ = w.WriteFile("data.db", 5, strings.NewReader("hello"))
	w.tamperChecksum("data.db") // test hook: corrupt the recorded hash before Close
	_ = w.Close()

	r, _ := NewReader(bytes.NewReader(buf.Bytes()), id)
	_, _ = r.ReadManifest()
	for {
		_, body, err := r.Next()
		if err == io.EOF {
			break
		}
		_, _ = io.Copy(io.Discard, body)
	}
	if err := r.Verify(); err == nil || !errors.Is(err, ErrChecksum) {
		t.Fatalf("want ErrChecksum, got %v", err)
	}
}

func TestDroppedChecksumFailsVerify(t *testing.T) {
	rcpt, id := testRecipient(t)
	var buf bytes.Buffer
	w, _ := NewWriter(&buf, rcpt, zstd.SpeedDefault)
	_ = w.WriteManifest(sampleManifest())
	_ = w.WriteFile("data.db", 5, strings.NewReader("hello"))
	w.dropChecksum("data.db") // test hook: omit this member's line from checksums.txt before Close
	_ = w.Close()

	r, _ := NewReader(bytes.NewReader(buf.Bytes()), id)
	_, _ = r.ReadManifest()
	for {
		_, body, err := r.Next()
		if err == io.EOF {
			break
		}
		_, _ = io.Copy(io.Discard, body)
	}
	if err := r.Verify(); err == nil || !errors.Is(err, ErrChecksum) {
		t.Fatalf("want ErrChecksum, got %v", err)
	}
}

// failAfterWriter fails every write once more than n bytes have been
// written in total, simulating a downstream write failure (e.g. a full
// disk) partway through Close.
type failAfterWriter struct {
	n int
}

func (f *failAfterWriter) Write(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, errors.New("simulated write failure")
	}
	if len(p) > f.n {
		w := f.n
		f.n = 0
		return w, errors.New("simulated write failure")
	}
	f.n -= len(p)
	return len(p), nil
}

func TestCloseIdempotentOnError(t *testing.T) {
	rcpt, _ := testRecipient(t)
	// n is large enough for WriteManifest/WriteFile to succeed (age's
	// header plus the buffered zstd frame for those two members) but too
	// small for Close's checksums.txt + frame-flush + age-close writes.
	w, err := NewWriter(&failAfterWriter{n: 300}, rcpt, zstd.SpeedDefault)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.WriteManifest(sampleManifest()); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFile("data.db", 5, strings.NewReader("hello")); err != nil {
		t.Fatal(err)
	}

	err1 := w.Close()
	if err1 == nil {
		t.Fatal("want Close to fail against a writer that errors partway through")
	}
	err2 := w.Close()
	if err2 == nil {
		t.Fatal("second Close silently reported success after the first Close failed")
	}
	if err2.Error() != err1.Error() {
		t.Fatalf("second Close returned a different error: %v vs %v", err2, err1)
	}
}

func TestTruncatedStreamFails(t *testing.T) {
	rcpt, id := testRecipient(t)
	data := buildArchive(t, rcpt)
	r, err := NewReader(bytes.NewReader(data[:len(data)/2]), id)
	if err != nil {
		return // failing at open is acceptable
	}
	_, _ = r.ReadManifest()
	for {
		_, body, err := r.Next()
		if err != nil {
			if err == io.EOF {
				t.Fatal("truncated stream reported clean EOF")
			}
			return
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			return
		}
	}
}

func TestWrongIdentityFails(t *testing.T) {
	rcpt, _ := testRecipient(t)
	_, other := testRecipient(t)
	data := buildArchive(t, rcpt)
	if _, err := NewReader(bytes.NewReader(data), other); err == nil {
		t.Fatal("wrong identity opened the archive")
	}
}

func TestNotABackup(t *testing.T) {
	_, id := testRecipient(t)
	if _, err := NewReader(strings.NewReader("garbage"), id); err == nil {
		t.Fatal("garbage opened")
	}
}

func TestInspect(t *testing.T) {
	rcpt, id := testRecipient(t)
	m, rep, err := Inspect(bytes.NewReader(buildArchive(t, rcpt)), id)
	if err != nil || !rep.OK || m.Kind != "manual" || len(rep.Members) != 2 {
		t.Fatalf("%+v %+v %v", m, rep, err)
	}
}

// countingReader tracks bytes read. zstd's decoder prefetches from the
// underlying reader on a background goroutine while the caller consumes
// already-decoded output on its own goroutine, so n needs synchronization
// even though nothing in this test looks concurrent.
type countingReader struct {
	mu sync.Mutex
	r  io.Reader
	n  int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.mu.Lock()
	c.n += n
	c.mu.Unlock()
	return n, err
}

func (c *countingReader) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}
