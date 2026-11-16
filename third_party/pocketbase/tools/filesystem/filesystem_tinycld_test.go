package filesystem_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// The stored original name keeps at most 255 bytes. A longer name is cut back
// to the last whole rune, because the blob writer refuses metadata that is not
// valid UTF-8.
var originalNameCases = []struct {
	label string
	name  string
	want  string
}{
	{"short ASCII name is unchanged", "invoice.pdf", "invoice.pdf"},
	{"255-byte ASCII name is unchanged", strings.Repeat("a", 255), strings.Repeat("a", 255)},
	{"255-byte name of 3-byte runes is unchanged", strings.Repeat("€", 85), strings.Repeat("€", 85)},
	{"256-byte ASCII name is cut to 255 bytes", strings.Repeat("a", 256), strings.Repeat("a", 255)},
	// byte 255 falls inside the 128th "é" (2 bytes each)
	{"cut inside a 2-byte rune", strings.Repeat("é", 130) + ".txt", strings.Repeat("é", 127)},
	// byte 255 is the third byte of the 85th "€" (3 bytes each, after one "a")
	{"cut inside a 3-byte rune", "a" + strings.Repeat("€", 90), "a" + strings.Repeat("€", 84)},
	// byte 255 is the fourth byte of the 64th emoji (4 bytes each)
	{"cut inside a 4-byte rune", strings.Repeat("😀", 70), strings.Repeat("😀", 63)},
}

func assertStoredOriginalName(t *testing.T, fsys *filesystem.System, fileKey string, want string) {
	t.Helper()

	attrs, err := fsys.Attributes(fileKey)
	if err != nil {
		t.Fatalf("Failed to fetch file attributes: %v", err)
	}
	name, ok := attrs.Metadata["original-filename"]
	if !ok {
		t.Fatal("Expected an original-filename metadata entry")
	}
	if !utf8.ValidString(name) {
		t.Fatalf("Expected original-filename to be valid UTF-8, got %q", name)
	}
	if name != want {
		t.Fatalf("Expected original-filename to be %d bytes %q, got %d bytes %q", len(want), want, len(name), name)
	}
}

func TestFilesystemUploadFileCutsALongNameOnARuneBoundary(t *testing.T) {
	for _, c := range originalNameCases {
		t.Run(c.label, func(t *testing.T) {
			dir := createTestDir(t)
			defer os.RemoveAll(dir)

			fsys, err := filesystem.NewLocal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer fsys.Close()

			file, err := filesystem.NewFileFromBytes([]byte("demo"), "demo.txt")
			if err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}
			file.OriginalName = c.name

			fileKey := "newdir/name.txt"
			if err := fsys.UploadFile(file, fileKey); err != nil {
				t.Fatalf("Expected the upload to succeed, got %v", err)
			}

			assertStoredOriginalName(t, fsys, fileKey, c.want)
		})
	}
}

func TestFilesystemUploadMultipartCutsALongNameOnARuneBoundary(t *testing.T) {
	for _, c := range originalNameCases {
		t.Run(c.label, func(t *testing.T) {
			dir := createTestDir(t)
			defer os.RemoveAll(dir)

			body := new(bytes.Buffer)
			mp := multipart.NewWriter(body)
			w, err := mp.CreateFormFile("test", c.name)
			if err != nil {
				t.Fatalf("Failed creating form file: %v", err)
			}
			w.Write([]byte("demo"))
			mp.Close()

			req := httptest.NewRequest(http.MethodPost, "/", body)
			req.Header.Add("Content-Type", mp.FormDataContentType())

			file, fh, err := req.FormFile("test")
			if err != nil {
				t.Fatalf("Failed to fetch form file: %v", err)
			}
			defer file.Close()

			if fh.Filename != c.name {
				t.Fatalf("Expected the multipart filename to be %q, got %q", c.name, fh.Filename)
			}

			fsys, err := filesystem.NewLocal(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer fsys.Close()

			fileKey := "newdir/name.txt"
			if err := fsys.UploadMultipart(fh, fileKey); err != nil {
				t.Fatalf("Expected the upload to succeed, got %v", err)
			}

			assertStoredOriginalName(t, fsys, fileKey, c.want)
		})
	}
}
