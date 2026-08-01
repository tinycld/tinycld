package thumbnails

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/nathanstitt/doctaculous/pkg/doctaculous"
)

// DefaultWidth is the default thumbnail width.
const DefaultWidth = 480

// DefaultHeight is the default thumbnail height.
const DefaultHeight = 360

// MaxInputBytes caps how much of a document is buffered for rendering.
// doctaculous parses the whole input in memory (the old mupdf path worked
// from a file on disk), so this bounds worst-case memory per render.
const MaxInputBytes = 50 << 20

// thumbFormats lists the doctaculous input formats we render document
// thumbnails for. Deliberately narrower than Format.ValidInput(): plain
// text / CSV / Markdown uploads have no useful page render, and PNG/JPEG
// images are handled by PocketBase's built-in ?thumb= parameter.
var thumbFormats = []doctaculous.Format{
	doctaculous.FormatPDF,
	doctaculous.FormatDOCX,
	doctaculous.FormatXLSX,
	doctaculous.FormatPPTX,
	doctaculous.FormatEPUB,
	doctaculous.FormatRTF,
}

// heifMimeTypes lists the HEIF MIME types we render ourselves. iPhone photo
// library emits image/heic for HEVC-encoded stills; image/heif covers the
// container. They stay a named list (rather than riding thumbFormats) because
// the reason to claim them differs from the reason to claim documents:
// PocketBase's ?thumb= parameter covers PNG/JPEG but cannot decode HEIF, so
// these are the one image family we thumbnail through doctaculous, exactly
// like pdf → image.
//
// The `-sequence` variants are deliberately absent: doctaculous decodes HEIF
// STILLS only (image/heic-sequence maps to FormatUnknown there), so claiming
// them would fail every render. If sequence support ever lands, add them here.
var heifMimeTypes = []string{
	"image/heic",
	"image/heif",
}

// CanGenerate reports whether a thumbnail can be generated for the given MIME type.
// Images are handled by PocketBase's built-in ?thumb= parameter, except HEIC/HEIF
// which Go's stdlib can't decode — we render those through doctaculous.
func CanGenerate(mimeType string) bool {
	mt := normalizeMime(mimeType)
	if slices.Contains(heifMimeTypes, mt) {
		return true
	}
	return slices.Contains(thumbFormats, doctaculous.FormatFromMIME(mt))
}

// Generate renders the document in r as a JPEG thumbnail written to w, fitted
// within width x height while preserving aspect ratio. The decoder is chosen
// from the document's MIME type — HEIF included, same path as every document
// format. Reads at most MaxInputBytes from r.
func Generate(ctx context.Context, w io.Writer, r io.Reader, mimeType string, width, height int) error {
	return generateFromDoc(ctx, w, r, mimeType, width, height)
}

// readCapped buffers at most MaxInputBytes from r, erroring when the input
// exceeds the cap.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxInputBytes+1))
	if err != nil {
		return nil, fmt.Errorf("thumbnails: failed to read document: %w", err)
	}
	if len(data) > MaxInputBytes {
		return nil, fmt.Errorf("thumbnails: document exceeds %d bytes", MaxInputBytes)
	}
	return data, nil
}

func generateFromDoc(ctx context.Context, w io.Writer, r io.Reader, mimeType string, width, height int) error {
	data, err := readCapped(r)
	if err != nil {
		return err
	}

	format := doctaculous.FormatFromMIME(mimeType)
	doc, err := doctaculous.OpenBytesAs(format, data)
	if err != nil {
		return fmt.Errorf("thumbnails: failed to open document: %w", err)
	}

	// Under a Max box, DPI is a resolution CEILING (DPI/72× the page's native
	// size). 300 lets documents render crisply before fitting — the old
	// render-at-native-resolution-then-Fit behavior. Photographic input (HEIC)
	// pins the ceiling at 72 = 1:1, so a photo smaller than the box is never
	// upscaled into blur — the downscale-only behavior the pre-doctaculous
	// imaging.Fit path had.
	dpi := 300.0
	if format == doctaculous.FormatHEIC {
		dpi = 72
	}

	err = doc.WriteImage(ctx, w, 0, doctaculous.ImageOptions{
		Format:  doctaculous.FormatJPEG,
		Quality: 85,
		Raster: doctaculous.RasterOptions{
			DPI:         dpi,
			MaxWidthPx:  width,
			MaxHeightPx: height,
		},
	})
	if err != nil {
		return fmt.Errorf("thumbnails: failed to render page: %w", err)
	}
	return nil
}

func normalizeMime(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(mimeType))
}
