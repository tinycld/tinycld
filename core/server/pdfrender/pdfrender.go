// Package pdfrender lays an HTML page out on paper pages and writes it as a
// PDF, and names a downloaded file in its Content-Disposition header.
package pdfrender

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nathanstitt/omnidoc/pkg/omnidoc"
)

// RenderTimeout bounds one render; omnidoc stops at the context.
const RenderTimeout = 30 * time.Second

// Fonts chooses the fonts a render may use beyond the bundled ones.
type Fonts struct {
	// Dir is a directory of extra font files for characters the bundled fonts
	// lack (docfonts.Dir(), from TINYCLD_FONTS_DIR); "" for none.
	Dir string
	// Hermetic ignores the fonts installed on the machine, so a test proves
	// what the bundled fonts and Dir cover on every host. Production leaves it
	// false.
	Hermetic bool
}

// HTMLToPDF lays the page out on the pages its @page rule names and writes the
// PDF. No resource loader is set, so the page can load nothing from the network
// or disk; an image must be an inline data: URI. A character the bundled fonts
// lack comes from fonts.Dir, then (unless Hermetic) from the machine's fonts.
// logf receives omnidoc's layout messages; nil drops them.
func HTMLToPDF(
	ctx context.Context, page []byte, fonts Fonts, logf func(string, ...any),
) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, RenderTimeout)
	defer cancel()
	opts := []omnidoc.HTMLOption{
		omnidoc.WithDefaultPaged(), omnidoc.WithPrintMedia(), omnidoc.WithContext(ctx),
	}
	if fonts.Dir != "" {
		opts = append(opts, omnidoc.WithFallbackFontDir(fonts.Dir))
	}
	if fonts.Hermetic {
		opts = append(opts, omnidoc.WithBundledFonts())
	}
	if logf != nil {
		opts = append(opts, omnidoc.WithLogf(logf))
	}
	doc, err := omnidoc.OpenHTMLBytes(page, opts...)
	if err != nil {
		return nil, fmt.Errorf("pdfrender: lay out pdf: %w", err)
	}
	var out bytes.Buffer
	if err := doc.WritePDF(ctx, &out, omnidoc.PDFOptions{}); err != nil {
		return nil, fmt.Errorf("pdfrender: write pdf: %w", err)
	}
	return out.Bytes(), nil
}

// Diagnostics logs, through the caller's logger (so the line keeps the
// caller's pkg), the layout messages that mean the printed page is wrong: a
// character drawn as a box, an image that did not load. omnidoc's other
// messages are routine notes about CSS it degrades, and are dropped.
func Diagnostics(ctx context.Context, logger *slog.Logger) func(string, ...any) {
	return func(format string, args ...any) {
		msg := fmt.Sprintf(format, args...)
		imageFailed := strings.Contains(msg, "image") && strings.Contains(msg, "failed")
		if !strings.Contains(msg, "no glyph for") && !imageFailed {
			return
		}
		// An image message quotes its data: URI; keep the log line short.
		msg = CutAtRune(msg, 300)
		logger.WarnContext(ctx, "pdf layout problem", "detail", msg)
	}
}

// CutAtRune shortens s to at most limit bytes plus an ellipsis, cutting at a
// rune boundary so a multi-byte character is never split into invalid UTF-8.
func CutAtRune(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "…"
}
