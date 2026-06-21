// Package textpreview renders a deliberately lightweight, NOT pixel-perfect,
// approximation of a document's first page to a JPEG. It is pure Go (no CGo,
// no mupdf) so it can run anywhere the binary runs without the native MuPDF
// stack. The output is a best-effort "what's roughly on page 1" preview —
// plain text, an optional header image, and an optional small table grid —
// not a faithful re-layout of the source document.
package textpreview

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"

	"github.com/disintegration/imaging"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// PageModel describes the content to draw onto a single preview page. Every
// field is optional: a zero PageModel yields a blank white page.
type PageModel struct {
	Title string
	Lines []string
	Grid  *GridModel
	Image image.Image
}

// GridModel describes a small table to draw beneath the text. Cells is indexed
// [row][col]; Rows/Cols are the intended dimensions (capped when rendered).
type GridModel struct {
	Rows, Cols int
	Cells      [][]string
}

const (
	margin     = 16
	lineHeight = 16
)

// RenderJPEG draws model onto a width x height white canvas and writes it to
// outputPath as a quality-85 JPEG. It is a deliberately lightweight, NOT
// pixel-perfect, approximation of page 1: text is drawn with a single fixed
// 7x13 bitmap face, the Title reuses that face with extra spacing (basicfont
// has no bold/large variant), images are scaled to fit a header band, and any
// grid is clipped to a handful of rows/cols. An empty model renders a valid
// blank white JPEG and never returns an error for being empty.
func RenderJPEG(model PageModel, outputPath string, width, height int) error {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), image.White, image.Point{}, draw.Src)

	yStart := margin

	if model.Image != nil {
		fitted := imaging.Fit(model.Image, width-2*margin, 140, imaging.Lanczos)
		draw.Draw(
			img,
			image.Rect(margin, margin, margin+fitted.Bounds().Dx(), margin+fitted.Bounds().Dy()),
			fitted,
			image.Point{},
			draw.Src,
		)
		yStart = margin + fitted.Bounds().Dy() + 8
	}

	y := drawText(img, model, yStart, width, height)

	if model.Grid != nil {
		drawGrid(img, model.Grid, y, width, height)
	}

	out, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("textpreview: create output: %w", err)
	}
	defer out.Close()

	if err := jpeg.Encode(out, img, &jpeg.Options{Quality: 85}); err != nil {
		return fmt.Errorf("textpreview: encode jpeg: %w", err)
	}

	return nil
}

// drawText renders Title (if any) then Lines, clipping vertically to the
// canvas. It returns the y position after the last drawn line.
func drawText(img *image.RGBA, model PageModel, yStart, width, height int) int {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.Black,
		Face: basicfont.Face7x13,
	}

	y := yStart

	if model.Title != "" {
		if y > height-margin {
			return y
		}
		d.Dot = fixed.P(margin, y)
		d.DrawString(model.Title)
		// basicfont has no bold/large face — the Title uses the same face
		// with an extra gap below to set it apart from the body lines.
		y += lineHeight + 8
	}

	for _, line := range model.Lines {
		if y > height-margin {
			break
		}
		d.Dot = fixed.P(margin, y)
		d.DrawString(line)
		y += lineHeight
	}

	return y
}

// drawGrid draws a capped table (<=12 rows, <=6 cols) with light-gray
// gridlines and rune-truncated cell text, clipped to the canvas.
func drawGrid(img *image.RGBA, grid *GridModel, yStart, width, height int) {
	rows := min(grid.Rows, 12)
	cols := min(grid.Cols, 6)
	if rows <= 0 || cols <= 0 {
		return
	}

	cellW := (width - 2*margin) / cols
	cellH := min(20, (height-2*margin)/rows)
	if cellW <= 0 || cellH <= 0 {
		return
	}

	gridColor := image.NewUniform(color.RGBA{220, 220, 220, 255})

	d := &font.Drawer{
		Dst:  img,
		Src:  image.Black,
		Face: basicfont.Face7x13,
	}

	maxChars := cellW / 7
	top := yStart + 4

	for r := 0; r < rows; r++ {
		cellTop := top + r*cellH
		cellBottom := cellTop + cellH
		if cellTop > height-margin {
			break
		}

		for c := 0; c < cols; c++ {
			cellLeft := margin + c*cellW
			cellRight := cellLeft + cellW

			// Top and left border for each cell (1px rects).
			draw.Draw(img, image.Rect(cellLeft, cellTop, cellRight, cellTop+1), gridColor, image.Point{}, draw.Src)
			draw.Draw(img, image.Rect(cellLeft, cellTop, cellLeft+1, cellBottom), gridColor, image.Point{}, draw.Src)

			text := cellText(grid, r, c)
			if text == "" {
				continue
			}
			text = truncateRunes(text, maxChars)

			baseline := cellTop + cellH - 6
			if baseline > height-margin {
				continue
			}
			d.Dot = fixed.P(cellLeft+3, baseline)
			d.DrawString(text)
		}
	}
}

func cellText(grid *GridModel, r, c int) string {
	if r >= len(grid.Cells) {
		return ""
	}
	row := grid.Cells[r]
	if c >= len(row) {
		return ""
	}
	return row[c]
}

func truncateRunes(s string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= maxChars {
		return s
	}
	return string(runes[:maxChars])
}
