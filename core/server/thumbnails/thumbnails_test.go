package thumbnails

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/nathanstitt/doctaculous/pkg/doctaculous"
)

const fixtureMarkdown = "# Fixture Heading\n\nHello doctaculous world.\n"

const fixtureCSV = "Name,Score\nHello doctaculous world,42\n"

// buildFixture converts a small Markdown (or CSV, for XLSX) source into the
// requested format via doctaculous's own writers, so no binary fixtures need
// checking in.
func buildFixture(t *testing.T, to doctaculous.Format) []byte {
	t.Helper()

	src, from := fixtureMarkdown, doctaculous.FormatMarkdown
	if to == doctaculous.FormatXLSX {
		src, from = fixtureCSV, doctaculous.FormatCSV
	}

	doc, err := doctaculous.OpenBytesAs(from, []byte(src))
	if err != nil {
		t.Fatalf("open fixture source: %v", err)
	}
	var buf bytes.Buffer
	if err := doc.Write(context.Background(), &buf, to, doctaculous.ConvertOptions{}); err != nil {
		t.Fatalf("write %s fixture: %v", to, err)
	}
	return buf.Bytes()
}

func TestGenerateDocumentFormats(t *testing.T) {
	cases := []struct {
		mime   string
		format doctaculous.Format
	}{
		{"application/pdf", doctaculous.FormatPDF},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", doctaculous.FormatDOCX},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", doctaculous.FormatXLSX},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", doctaculous.FormatPPTX},
		{"application/epub+zip", doctaculous.FormatEPUB},
		{"application/rtf", doctaculous.FormatRTF},
	}
	for _, tc := range cases {
		t.Run(string(tc.format), func(t *testing.T) {
			fixture := buildFixture(t, tc.format)

			var out bytes.Buffer
			err := Generate(context.Background(), &out, bytes.NewReader(fixture), tc.mime, DefaultWidth, DefaultHeight)
			if err != nil {
				t.Fatalf("Generate: %v", err)
			}

			img, err := jpeg.Decode(&out)
			if err != nil {
				t.Fatalf("output is not a JPEG: %v", err)
			}
			b := img.Bounds()
			if b.Dx() <= 0 || b.Dy() <= 0 {
				t.Fatalf("empty image: %v", b)
			}
			if b.Dx() > DefaultWidth || b.Dy() > DefaultHeight {
				t.Fatalf("thumbnail %dx%d exceeds %dx%d", b.Dx(), b.Dy(), DefaultWidth, DefaultHeight)
			}
		})
	}
}

// The one binary fixture in this suite: doctaculous decodes HEIF but does not
// encode it, so a HEIC render can't build its fixture from Markdown the way
// the document formats above do. 532 bytes, from doctaculous's own decoder
// corpus (a 64x48 sips-encoded quad).
func TestGenerateHEIC(t *testing.T) {
	fixture, err := os.ReadFile("testdata/sips-quad-64x48.heic")
	if err != nil {
		t.Fatalf("read heic fixture: %v", err)
	}

	var out bytes.Buffer
	err = Generate(context.Background(), &out, bytes.NewReader(fixture), "image/heic", DefaultWidth, DefaultHeight)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	img, err := jpeg.Decode(&out)
	if err != nil {
		t.Fatalf("output is not a JPEG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 64 || b.Dy() != 48 {
		t.Fatalf("thumbnail = %dx%d, want the source's 64x48 (fits inside the default box, so no scaling)", b.Dx(), b.Dy())
	}
}

func TestCanGenerate(t *testing.T) {
	cases := []struct {
		mime string
		want bool
	}{
		{"application/pdf", true},
		{"application/epub+zip", true},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", true},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", true},
		{"application/rtf", true},
		{"text/rtf", true},
		{"image/heic", true},
		{"image/heif", true},
		{"IMAGE/HEIC ", true}, // normalizeMime lowercases + trims
		// Sequences are refused: doctaculous decodes HEIF stills only, so
		// claiming these would fail every render.
		{"image/heic-sequence", false},
		{"image/heif-sequence", false},
		// Legacy binary Office: deliberately unsupported since the doctaculous
		// migration (no thumbnail is rendered for these).
		{"application/msword", false},
		{"application/vnd.ms-excel", false},
		{"application/vnd.ms-powerpoint", false},
		// Plain images are served via PocketBase's ?thumb= parameter.
		{"image/png", false},
		{"image/jpeg", false},
		{"text/plain", false},
		{"text/csv", false},
		{"application/octet-stream", false},
	}
	for _, tc := range cases {
		if got := CanGenerate(tc.mime); got != tc.want {
			t.Errorf("CanGenerate(%q) = %v, want %v", tc.mime, got, tc.want)
		}
	}
}

func TestGenerateRejectsOversizeInput(t *testing.T) {
	// Both the document path and the HEIF path must enforce MaxInputBytes.
	for _, mime := range []string{"application/pdf", "image/heic"} {
		t.Run(mime, func(t *testing.T) {
			huge := bytes.NewReader(make([]byte, MaxInputBytes+1))
			var out bytes.Buffer
			err := Generate(context.Background(), &out, huge, mime, DefaultWidth, DefaultHeight)
			if err == nil {
				t.Fatal("expected error for oversize input")
			}
			if !strings.Contains(err.Error(), "exceeds") {
				t.Fatalf("expected size error, got: %v", err)
			}
		})
	}
}

func TestGenerateRejectsCorruptInput(t *testing.T) {
	garbage := bytes.NewReader([]byte("this is definitely not a PDF"))
	var out bytes.Buffer
	err := Generate(context.Background(), &out, garbage, "application/pdf", DefaultWidth, DefaultHeight)
	if err == nil {
		t.Fatal("expected error for corrupt input")
	}
}

// TestGenerateConcurrent exercises parallel rendering. The old MuPDF engine
// needed a global mutex (concurrent calls segfaulted); doctaculous must not.
func TestGenerateConcurrent(t *testing.T) {
	fixture := buildFixture(t, doctaculous.FormatPDF)

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var out bytes.Buffer
			errs[i] = Generate(context.Background(), &out, bytes.NewReader(fixture), "application/pdf", DefaultWidth, DefaultHeight)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}
