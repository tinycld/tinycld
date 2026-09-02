package thumbnails

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/nathanstitt/omnidoc/pkg/omnidoc"
)

// DefaultWidth is the default thumbnail width.
const DefaultWidth = 480

// DefaultHeight is the default thumbnail height.
const DefaultHeight = 360

// MaxInputBytes caps how much of a document is buffered for rendering.
// omnidoc parses the whole input in memory (the old mupdf path worked
// from a file on disk), so this bounds worst-case memory per render.
const MaxInputBytes = 50 << 20

// thumbFormats lists the omnidoc input formats we render document
// thumbnails for. Deliberately narrower than Format.ValidInput(): plain
// text / CSV / Markdown uploads have no useful page render, and PNG/JPEG
// images are handled by PocketBase's built-in ?thumb= parameter.
//
// SVG rides this list rather than the image path below because it is a
// document, not a raster: it renders from its own vector geometry, so the
// 300-DPI ceiling produces a crisp thumbnail at any output size.
var thumbFormats = []omnidoc.Format{
	omnidoc.FormatPDF,
	omnidoc.FormatDOCX,
	omnidoc.FormatXLSX,
	omnidoc.FormatPPTX,
	omnidoc.FormatEPUB,
	omnidoc.FormatRTF,
	omnidoc.FormatSVG,
}

// photoMimeTypes lists the raster image MIME types we render ourselves. They
// stay a named list (rather than riding thumbFormats) because the reason to
// claim them differs from the reason to claim documents: PocketBase's ?thumb=
// parameter covers PNG/JPEG but cannot decode these, so they are the image
// family we thumbnail through omnidoc, exactly like pdf → image. Being raster
// is what puts them here — the DPI ceiling below pins them to 1:1 so a photo
// smaller than the thumbnail box is never upscaled into blur.
//
// iPhone photo library emits image/heic for HEVC-encoded stills; image/heif
// covers the container. The `-sequence` variants are deliberately absent:
// omnidoc decodes HEIF STILLS only (image/heic-sequence maps to FormatUnknown
// there), so claiming them would fail every render. If sequence support ever
// lands, add them here. Animated WebP is likewise refused by the decoder
// (ErrAnimatedImage); a still WebP renders fine.
var photoMimeTypes = []string{
	"image/heic",
	"image/heif",
	"image/webp",
}

// CanGenerate reports whether a thumbnail can be generated for the given MIME type.
// Images are handled by PocketBase's built-in ?thumb= parameter, except HEIC/HEIF
// which Go's stdlib can't decode — we render those through omnidoc.
func CanGenerate(mimeType string) bool {
	mt := normalizeMime(mimeType)
	if slices.Contains(photoMimeTypes, mt) {
		return true
	}
	return slices.Contains(thumbFormats, omnidoc.FormatFromMIME(mt))
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

	format := omnidoc.FormatFromMIME(mimeType)
	doc, err := omnidoc.OpenBytesAs(format, data)
	if err != nil {
		return fmt.Errorf("thumbnails: failed to open document: %w", err)
	}

	// Under a Max box, DPI is a resolution CEILING (DPI/72× the page's native
	// size). 300 lets documents render crisply before fitting — the old
	// render-at-native-resolution-then-Fit behavior. Raster input (HEIC, WebP)
	// pins the ceiling at 72 = 1:1, so a photo smaller than the box is never
	// upscaled into blur — the downscale-only behavior the pre-omnidoc
	// imaging.Fit path had. SVG is deliberately NOT pinned: it is vector, so
	// rendering above its nominal size sharpens rather than blurs.
	dpi := 300.0
	if format == omnidoc.FormatHEIC || format == omnidoc.FormatWebP {
		dpi = 72
	}

	err = doc.WriteImage(ctx, w, 0, omnidoc.ImageOptions{
		Format:  omnidoc.FormatJPEG,
		Quality: 85,
		Raster: omnidoc.RasterOptions{
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
