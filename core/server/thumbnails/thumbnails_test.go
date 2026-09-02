package thumbnails

import (
	"bytes"
	"context"
	"image/jpeg"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/nathanstitt/omnidoc/pkg/omnidoc"
)

const fixtureMarkdown = "# Fixture Heading\n\nHello omnidoc world.\n"

const fixtureCSV = "Name,Score\nHello omnidoc world,42\n"

// buildFixture converts a small Markdown (or CSV, for XLSX) source into the
// requested format via omnidoc's own writers, so no binary fixtures need
// checking in.
func buildFixture(t *testing.T, to omnidoc.Format) []byte {
	t.Helper()

	src, from := fixtureMarkdown, omnidoc.FormatMarkdown
	if to == omnidoc.FormatXLSX {
		src, from = fixtureCSV, omnidoc.FormatCSV
	}

	doc, err := omnidoc.OpenBytesAs(from, []byte(src))
	if err != nil {
		t.Fatalf("open fixture source: %v", err)
	}
	var buf bytes.Buffer
	if err := doc.Write(context.Background(), &buf, to, omnidoc.ConvertOptions{}); err != nil {
		t.Fatalf("write %s fixture: %v", to, err)
	}
	return buf.Bytes()
}

func TestGenerateDocumentFormats(t *testing.T) {
	cases := []struct {
		mime   string
		format omnidoc.Format
	}{
		{"application/pdf", omnidoc.FormatPDF},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", omnidoc.FormatDOCX},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", omnidoc.FormatXLSX},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", omnidoc.FormatPPTX},
		{"application/epub+zip", omnidoc.FormatEPUB},
		{"application/rtf", omnidoc.FormatRTF},
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

// The one binary fixture in this suite: omnidoc decodes HEIF but does not
// encode it, so a HEIC render can't build its fixture from Markdown the way
// the document formats above do. 532 bytes, from omnidoc's own decoder
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

// SVG is vector, so unlike the HEIC fixture it can be written inline. It also
// exercises the one branch that must NOT pin DPI to 72: rendering a 200x120
// nominal SVG into the 480x360 box scales it UP, which is correct for vector
// input and would be wrong for a photo.
func TestGenerateSVG(t *testing.T) {
	const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="120">` +
		`<rect width="200" height="120" fill="#0B4F4A"/></svg>`

	var out bytes.Buffer
	err := Generate(context.Background(), &out, strings.NewReader(svg), "image/svg+xml", DefaultWidth, DefaultHeight)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	img, err := jpeg.Decode(&out)
	if err != nil {
		t.Fatalf("output is not a JPEG: %v", err)
	}
	// Aspect ratio is preserved and the render fills the box's constraining
	// dimension rather than staying at the source's 200x120.
	if b := img.Bounds(); b.Dx() <= 200 || b.Dy() <= 120 {
		t.Fatalf("svg thumbnail = %dx%d, want it scaled up into the %dx%d box",
			b.Dx(), b.Dy(), DefaultWidth, DefaultHeight)
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
		{"image/webp", true},
		{"image/svg+xml", true},
		{"IMAGE/HEIC ", true}, // normalizeMime lowercases + trims
		// Sequences are refused: omnidoc decodes HEIF stills only, so
		// claiming these would fail every render.
		{"image/heic-sequence", false},
		{"image/heif-sequence", false},
		// Legacy binary Office: deliberately unsupported since the omnidoc
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
// needed a global mutex (concurrent calls segfaulted); omnidoc must not.
func TestGenerateConcurrent(t *testing.T) {
	fixture := buildFixture(t, omnidoc.FormatPDF)

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
