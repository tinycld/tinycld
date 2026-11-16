package pdfrender

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/nathanstitt/omnidoc/pkg/omnidoc"

	"tinycld.org/core/logging"
	"tinycld.org/core/readonly/readonlytest"
)

// twoPages forces a page break between its two paragraphs.
const twoPages = `<!doctype html><html><head><style>
@page { size: letter; margin: 1in; }
.next { break-before: page; page-break-before: always; }
</style></head><body>
<p>First page</p>
<p class="next">Second page</p>
</body></html>`

func TestHTMLToPDF_LaysTheHTMLOutOnPages(t *testing.T) {
	pdf, err := HTMLToPDF(context.Background(), []byte(twoPages), Fonts{Hermetic: true}, nil)
	if err != nil {
		t.Fatalf("HTMLToPDF: %v", err)
	}
	doc, err := omnidoc.OpenBytes(pdf)
	if err != nil {
		t.Fatalf("the bytes are not a PDF omnidoc reads: %v", err)
	}
	if got := doc.PageCount(); got != 2 {
		t.Errorf("PageCount() = %d, want 2", got)
	}
}

func TestHTMLToPDF_StopsAtACancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pdf, err := HTMLToPDF(ctx, []byte(twoPages), Fonts{Hermetic: true}, nil)
	if err == nil {
		t.Fatalf("a cancelled render gave %d bytes and no error", len(pdf))
	}
	if !strings.HasPrefix(err.Error(), "pdfrender:") {
		t.Errorf("error %q does not start with pdfrender:", err)
	}
}

func TestDiagnostics_LogsOnlyGlyphAndImageProblemsUnderTheCallersPackage(t *testing.T) {
	logs := readonlytest.CaptureLogs(t)
	logf := Diagnostics(context.Background(), logging.ForPackage("zoo"))
	logf("no glyph for %q in font %s", 'Z', "Sans")
	logf("image %s failed: %s", "data:image/png;base64,"+strings.Repeat("A", 400), "bad")
	logf("unsupported CSS property %s", "float")
	logf("image %s loaded", "logo")

	var got []readonlytest.Record
	for _, r := range logs() {
		if _, ok := r.Attrs["detail"]; ok {
			got = append(got, r)
		}
	}
	if len(got) != 2 {
		t.Fatalf("logged %d layout problems, want 2 (glyph, image): %+v", len(got), got)
	}
	glyph, image := got[0], got[1]
	if glyph.Level != slog.LevelWarn || glyph.Attrs["pkg"] != "zoo" ||
		!strings.HasPrefix(glyph.Attrs["detail"], "no glyph for") {
		t.Errorf("glyph record = %+v, want a warning of pkg zoo about the glyph", glyph)
	}
	if image.Attrs["pkg"] != "zoo" || !strings.HasPrefix(image.Attrs["detail"], "image data:") {
		t.Errorf("image record = %+v, want the image failure under pkg zoo", image)
	}
	if want := 300 + len("…"); len(image.Attrs["detail"]) != want {
		t.Errorf("image detail is %d bytes, want it cut to %d", len(image.Attrs["detail"]), want)
	}
}

func TestCutAtRune_NeverSplitsACharacter(t *testing.T) {
	if got := CutAtRune("short", 10); got != "short" {
		t.Errorf("CutAtRune(short, 10) = %q, want it unchanged", got)
	}
	// "é" is two bytes; the first one sits at byte 9, so a cut at 10 would split it.
	s := strings.Repeat("A", 9) + strings.Repeat("é", 4)
	got := CutAtRune(s, 10)
	if !utf8.ValidString(got) || got != strings.Repeat("A", 9)+"…" {
		t.Errorf("CutAtRune(%q, 10) = %q, want 9 A's and an ellipsis", s, got)
	}
	if got := CutAtRune(strings.Repeat("A", 8)+"éé", 10); got != strings.Repeat("A", 8)+"é…" {
		t.Errorf("a cut on a character boundary = %q, want 8 A's, é and an ellipsis", got)
	}
}
