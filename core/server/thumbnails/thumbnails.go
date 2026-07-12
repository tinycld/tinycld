package thumbnails

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"slices"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/jdeng/goheif"
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
}

// heifMimeTypes lists MIME types we decode via goheif. iPhone photo library
// emits image/heic for HEVC-encoded stills; image/heif covers the container.
// goheif (CGo) stays until doctaculous grows HEIF support; when that lands,
// this list and the goheif dependency go away.
var heifMimeTypes = []string{
	"image/heic",
	"image/heif",
	"image/heic-sequence",
	"image/heif-sequence",
}

// CanGenerate reports whether a thumbnail can be generated for the given MIME type.
// Images are handled by PocketBase's built-in ?thumb= parameter, except HEIC/HEIF
// which Go's stdlib can't decode — we render those ourselves.
func CanGenerate(mimeType string) bool {
	mt := normalizeMime(mimeType)
	if slices.Contains(heifMimeTypes, mt) {
		return true
	}
	return slices.Contains(thumbFormats, doctaculous.FormatFromMIME(mt))
}

// Generate renders the document in r as a JPEG thumbnail written to w, fitted
// within width x height while preserving aspect ratio. The decoder is chosen
// from the document's MIME type. Reads at most MaxInputBytes from r.
func Generate(ctx context.Context, w io.Writer, r io.Reader, mimeType string, width, height int) error {
	if slices.Contains(heifMimeTypes, normalizeMime(mimeType)) {
		return generateFromHeif(w, r, width, height)
	}
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

	doc, err := doctaculous.OpenBytesAs(doctaculous.FormatFromMIME(mimeType), data)
	if err != nil {
		return fmt.Errorf("thumbnails: failed to open document: %w", err)
	}

	err = doc.WriteImage(ctx, w, 0, doctaculous.ImageOptions{
		Format:  doctaculous.FormatJPEG,
		Quality: 85,
		Raster: doctaculous.RasterOptions{
			// A positive DPI makes the Max box a downscale-only ceiling,
			// matching the old render-at-native-resolution-then-Fit behavior.
			DPI:         300,
			MaxWidthPx:  width,
			MaxHeightPx: height,
		},
	})
	if err != nil {
		return fmt.Errorf("thumbnails: failed to render page: %w", err)
	}
	return nil
}

func generateFromHeif(w io.Writer, r io.Reader, width, height int) error {
	// Buffering also hands goheif an io.ReaderAt, sidestepping its internal
	// ReadAll of the whole stream.
	data, err := readCapped(r)
	if err != nil {
		return err
	}
	img, err := goheif.Decode(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("thumbnails: failed to decode heif: %w", err)
	}
	return writeJpegThumb(w, img, width, height)
}

func writeJpegThumb(w io.Writer, img image.Image, width, height int) error {
	thumb := imaging.Fit(img, width, height, imaging.Lanczos)
	if err := jpeg.Encode(w, thumb, &jpeg.Options{Quality: 85}); err != nil {
		return fmt.Errorf("thumbnails: failed to encode JPEG: %w", err)
	}
	return nil
}

func normalizeMime(mimeType string) string {
	return strings.ToLower(strings.TrimSpace(mimeType))
}
