package textextract

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

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

func TestExtractDocumentFormats(t *testing.T) {
	cases := []struct {
		mime   string
		format doctaculous.Format
	}{
		{"application/pdf", doctaculous.FormatPDF},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", doctaculous.FormatDOCX},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", doctaculous.FormatXLSX},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", doctaculous.FormatPPTX},
		{"application/epub+zip", doctaculous.FormatEPUB},
	}
	for _, tc := range cases {
		t.Run(string(tc.format), func(t *testing.T) {
			fixture := buildFixture(t, tc.format)
			result, err := Extract(bytes.NewReader(fixture), tc.mime, MaxOutputBytes)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			if !strings.Contains(result, "Hello doctaculous world") {
				t.Errorf("expected fixture text in extraction, got %q", result)
			}
		})
	}
}

func TestExtractLegacyOfficeUnsupported(t *testing.T) {
	// Legacy binary Office is deliberately unsupported since the doctaculous
	// migration: no extraction, no error.
	for _, mime := range []string{
		"application/msword",
		"application/vnd.ms-excel",
		"application/vnd.ms-powerpoint",
	} {
		result, err := Extract(strings.NewReader("legacy binary blob"), mime, MaxOutputBytes)
		if err != nil {
			t.Fatalf("%s: %v", mime, err)
		}
		if result != "" {
			t.Errorf("%s: expected empty result, got %q", mime, result)
		}
	}
}

func TestExtractImageUnsupported(t *testing.T) {
	// Tiny valid 1x1 PNG header + data — images carry no text.
	png := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}
	result, err := Extract(bytes.NewReader(png), "image/png", MaxOutputBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Errorf("expected empty result for image, got %q", result)
	}
}

func TestExtractCorruptDocument(t *testing.T) {
	result, err := Extract(strings.NewReader("not a real pdf"), "application/pdf", MaxOutputBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result != "" {
		t.Errorf("expected empty result for corrupt input, got %q", result)
	}
}

func TestExtractMaxBytesTruncationIsValidUTF8(t *testing.T) {
	long := "# Heading\n\n" + strings.Repeat("héllo wörld ", 500)
	doc, err := doctaculous.OpenBytesAs(doctaculous.FormatMarkdown, []byte(long))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := doc.Write(context.Background(), &buf, doctaculous.FormatDOCX, doctaculous.ConvertOptions{}); err != nil {
		t.Fatal(err)
	}

	const maxBytes = 100
	result, err := Extract(&buf, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", maxBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) == 0 || len(result) > maxBytes {
		t.Fatalf("expected 1..%d bytes, got %d", maxBytes, len(result))
	}
	if !utf8.ValidString(result) {
		t.Errorf("truncated result is not valid UTF-8: %q", result)
	}
}

type fakeExtractor struct{ out string }

func (e *fakeExtractor) Extract(r io.Reader, limit int) (string, error) {
	return e.out, nil
}

// TestRegistryPrecedence proves a Register()ed extractor wins over the
// built-in doctaculous handling for the same MIME type — the hook packages
// use to support custom formats.
func TestRegistryPrecedence(t *testing.T) {
	Register("application/pdf", &fakeExtractor{out: "custom extractor output"})
	defer delete(registry, "application/pdf")

	fixture := buildFixture(t, doctaculous.FormatPDF)
	result, err := Extract(bytes.NewReader(fixture), "application/pdf", MaxOutputBytes)
	if err != nil {
		t.Fatal(err)
	}
	if result != "custom extractor output" {
		t.Errorf("expected registered extractor to win, got %q", result)
	}
}
