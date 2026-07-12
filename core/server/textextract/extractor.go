// Package textextract extracts plain text from uploaded files for search
// indexing. Built-in formats (PDF, Office, EPUB, RTF, HTML, text) are handled
// by doctaculous; packages can Register custom extractors for additional MIME
// types, and a registered extractor always takes precedence over the built-in
// handling.
package textextract

import (
	"bytes"
	"context"
	"io"
	"mime"
	"slices"
	"strings"

	"github.com/nathanstitt/doctaculous/pkg/doctaculous"
)

// MaxOutputBytes is the default maximum size of extracted text output.
const MaxOutputBytes = 50 * 1024

// maxInputBytes caps how much of an upload is buffered for parsing.
// doctaculous parses the whole input in memory.
const maxInputBytes = 10 << 20

// Extractor extracts plain text from a specific file format.
type Extractor interface {
	Extract(r io.Reader, limit int) (string, error)
}

var registry = make(map[string]Extractor)

// Register associates a MIME type with an Extractor, overriding the built-in
// doctaculous handling for that type. Called from handler init() functions.
func Register(mimeType string, e Extractor) {
	registry[mimeType] = e
}

// docFormats lists the doctaculous input formats extracted via a full
// document parse. Text-family formats (plain, Markdown, CSV) are cheaper
// and just as useful for search indexing as a raw passthrough.
var docFormats = []doctaculous.Format{
	doctaculous.FormatPDF,
	doctaculous.FormatDOCX,
	doctaculous.FormatXLSX,
	doctaculous.FormatPPTX,
	doctaculous.FormatEPUB,
	doctaculous.FormatRTF,
	doctaculous.FormatHTML,
}

// Extract extracts plain text from r for the given MIME type, dispatching to
// a registered handler first, then to the built-in doctaculous support.
// Falls back to plain-text passthrough for any text/* type.
// Returns ("", nil) for unsupported or corrupt files.
func Extract(r io.Reader, mimeType string, maxBytes int) (string, error) {
	if maxBytes <= 0 {
		maxBytes = MaxOutputBytes
	}

	// Normalize: strip parameters (e.g. "text/html; charset=utf-8" → "text/html")
	mt, _, _ := mime.ParseMediaType(mimeType)
	if mt == "" {
		mt = mimeType
	}
	mt = strings.ToLower(strings.TrimSpace(mt))

	if e, ok := registry[mt]; ok {
		return e.Extract(r, maxBytes)
	}

	switch f := doctaculous.FormatFromMIME(mt); {
	case slices.Contains(docFormats, f):
		return extractDocument(r, f, maxBytes)
	case strings.HasPrefix(mt, "text/"):
		return extractPlainText(r, maxBytes)
	default:
		return "", nil
	}
}

func extractDocument(r io.Reader, f doctaculous.Format, maxBytes int) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxInputBytes))
	if err != nil {
		return "", nil
	}

	doc, err := doctaculous.OpenBytesAs(f, data)
	if err != nil {
		// Corrupt or truncated files aren't indexing errors — skip them.
		return "", nil
	}

	var buf bytes.Buffer
	if err := doc.WriteText(context.Background(), &buf, doctaculous.MarkdownOptions{MaxBytes: maxBytes}); err != nil {
		return "", nil
	}
	return strings.TrimSpace(buf.String()), nil
}

func extractPlainText(r io.Reader, limit int) (string, error) {
	data, err := io.ReadAll(io.LimitReader(r, int64(limit)+1))
	if err != nil {
		return "", nil
	}
	if len(data) > limit {
		data = data[:limit]
	}
	return strings.TrimSpace(string(data)), nil
}
