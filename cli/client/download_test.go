package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestLocalFileName pins the sanitizer every download path relies on to keep a
// server-supplied name from steering a write out of the user's chosen directory.
func TestLocalFileName(t *testing.T) {
	cases := map[string]string{
		"report.pdf":                 "report.pdf",
		"../../escaped.txt":          "escaped.txt",
		"/etc/passwd":                "passwd",
		"..":                         "download",
		".":                          "download",
		"/":                          "download",
		"":                           "download",
		"a/b/c.txt":                  "c.txt",
		`..\..\windows\evil.txt`:     "evil.txt",
		"trailing/":                  "trailing",
		"weird name (1).tar.gz":      "weird name (1).tar.gz",
		"....//....//still-escaping": "still-escaping",
	}
	for in, want := range cases {
		if got := LocalFileName(in); got != want {
			t.Errorf("LocalFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDownloadToFile(t *testing.T) {
	content := strings.Repeat("x", 4096)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/drive_items/r1/report.pdf", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		// PocketBase file responses carry an explicit length; without the
		// header the httptest server streams chunked and ContentLength is -1.
		w.Header().Set("Content-Length", strconv.Itoa(len(content)))
		w.Write([]byte(content))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("access-1"), srv.Client())

	dest := filepath.Join(t.TempDir(), "out.pdf")
	var lastWritten, lastTotal int64
	err := DownloadToFile(context.Background(), c, "/api/files/drive_items/r1/report.pdf", dest,
		func(written, total int64) { lastWritten, lastTotal = written, total })
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("content mismatch: %d bytes", len(got))
	}
	if lastWritten != int64(len(content)) || lastTotal != int64(len(content)) {
		t.Fatalf("progress written=%d total=%d", lastWritten, lastTotal)
	}
	// No temp litter left beside the destination.
	entries, _ := os.ReadDir(filepath.Dir(dest))
	if len(entries) != 1 {
		t.Fatalf("dest dir has %d entries, want only the download", len(entries))
	}
}

func TestDownloadToFileErrorLeavesNoDest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Requires the \"drive:read\" scope"}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := New(srv.URL, validStore("t"), srv.Client())

	dest := filepath.Join(t.TempDir(), "out.bin")
	err := DownloadToFile(context.Background(), c, "/api/files/x/y/z", dest, nil)
	if err == nil || !strings.Contains(err.Error(), "drive:read") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(dest); !os.IsNotExist(statErr) {
		t.Fatal("failed download must not leave a destination file")
	}
}

func TestDownloadPublicSendsNoBearer(t *testing.T) {
	var sawAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/drive/download-folder", func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Write([]byte("zip-bytes"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "folder.zip")
	err := DownloadPublic(context.Background(), srv.Client(),
		srv.URL+"/api/drive/download-folder?token=abc", dest, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sawAuth != "" {
		t.Fatalf("public download sent Authorization %q — token URLs are credential-less by design", sawAuth)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != "zip-bytes" {
		t.Fatalf("content = %q", got)
	}
}

func TestStreamingOutlivesClientTimeout(t *testing.T) {
	// http.Client.Timeout covers the ENTIRE exchange including the body read,
	// so a transfer longer than the configured API timeout would abort
	// mid-stream if downloads went through Do. This server trickles a body
	// for ~6× the client's timeout; DownloadToFile must still complete.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/files/drive_items/r1/big.bin", func(w http.ResponseWriter, r *http.Request) {
		f := w.(http.Flusher)
		for range 6 {
			w.Write([]byte("chunk"))
			f.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	hc := &http.Client{Timeout: 50 * time.Millisecond}
	c := New(srv.URL, validStore("access-1"), hc)

	dest := filepath.Join(t.TempDir(), "big.bin")
	if err := DownloadToFile(context.Background(), c, "/api/files/drive_items/r1/big.bin", dest, nil); err != nil {
		t.Fatalf("streaming download died to the API timeout: %v", err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != strings.Repeat("chunk", 6) {
		t.Fatalf("content = %q", got)
	}
}

// A download is ordinary user data, not a credential. os.CreateTemp hardcodes
// 0600, so downloads used to land owner-only regardless of umask — a file the
// user could not share with their own group, unlike curl or scp.
func TestDownloadRespectsUmask(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no umask on Windows")
	}
	// dataFileMode samples the umask once per process, so this asserts against
	// whatever it actually sampled rather than re-setting it here — the point
	// is that the mode is umask-derived data, not the hardcoded 0600.
	want := dataFileMode().Perm()
	if want == 0o600 {
		t.Skip("umask makes the expected mode indistinguishable from the old hardcoded 0600")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("payload"))
	}))
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "report.csv")
	if err := DownloadPublic(t.Context(), srv.Client(), srv.URL, dest, nil); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("downloaded file mode = %04o, want %04o (0666 minus the umask)", got, want)
	}
}
