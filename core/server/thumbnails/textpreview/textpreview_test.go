package textpreview

import (
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// renderAndDecode renders model to a temp JPEG and decodes it back, asserting a
// valid JPEG at exactly width x height.
func renderAndDecode(t *testing.T, model PageModel, width, height int) {
	t.Helper()

	outputPath := filepath.Join(t.TempDir(), "preview.jpg")
	if err := RenderJPEG(model, outputPath, width, height); err != nil {
		t.Fatalf("RenderJPEG returned error: %v", err)
	}

	f, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("opening rendered file: %v", err)
	}
	defer f.Close()

	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatalf("decoding rendered JPEG: %v", err)
	}

	bounds := img.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Fatalf("rendered size = %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}
}

func TestRenderJPEGTextOnly(t *testing.T) {
	model := PageModel{Title: "Hello", Lines: []string{"a", "b", "c"}}
	renderAndDecode(t, model, 240, 320)
}

func TestRenderJPEGGrid(t *testing.T) {
	model := PageModel{
		Grid: &GridModel{
			Rows: 3,
			Cols: 3,
			Cells: [][]string{
				{"1", "2", "3"},
				{"4", "5", "6"},
				{"7", "8", "9"},
			},
		},
	}
	renderAndDecode(t, model, 240, 320)
}

func TestRenderJPEGImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 40))
	red := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 40; y++ {
		for x := 0; x < 40; x++ {
			src.Set(x, y, red)
		}
	}

	model := PageModel{Image: src, Lines: []string{"caption"}}
	renderAndDecode(t, model, 240, 320)
}

func TestRenderJPEGEmptyModel(t *testing.T) {
	renderAndDecode(t, PageModel{}, 240, 320)
}

func TestRenderJPEG480x360(t *testing.T) {
	model := PageModel{Title: "Hello", Lines: []string{"a", "b", "c"}}
	renderAndDecode(t, model, 480, 360)
}
