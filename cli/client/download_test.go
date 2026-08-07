package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

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
