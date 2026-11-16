package reports

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
)

// sampleLogo is a 48x48 logo in two colors.
func sampleLogo(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 48, 48))
	for x := range 48 {
		for y := range 48 {
			c := color.RGBA{R: 11, G: 79, B: 74, A: 255}
			if x > 12 && x < 36 && y > 12 && y < 36 {
				c = color.RGBA{R: 255, G: 255, B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// sampleStatementRows are the statement-sized layout's rows of
// TestCellsOfAStatementSizedLayoutRun: rows within both sections, the section
// totals, a computed row of both sections and a later one that reads it.
func sampleStatementRows() string {
	f4 := `{"op":"eq","args":[{"path":[{"field":"note"}]},{"value":"f4"}]}`
	curF4, priF4 := sumCostTo(`{"param":"asOf"}`, f4), sumCostTo(yearBefore, f4)
	curAll, priAll := sumCostTo(`{"param":"asOf"}`, ""), sumCostTo(yearBefore, "")
	return `,{"row":"ca","label":"Cat adjust","within":"cats","cells":{"cur":` + addOf(curF4, priAll) + `,"pri":` + addOf(priF4, priF4) + `}}` +
		`,{"row":"re","label":"Retained","within":"stripes","cells":{"cur":` + addOf(curF4, curF4) + `,"pri":` + addOf(priF4, priF4) + `}}` +
		`,{"row":"ni","label":"Net","within":"stripes","cells":{"cur":{"op":"sub","args":[` + curAll + `,` + priAll + `]},"pri":` + addOf(priF4, priF4) + `}}` +
		`,{"row":"both","label":"Both","formula":` + addOf(`{"subtotal":"cats"}`, `{"subtotal":"stripes"}`) + `}` +
		`,{"row":"grand","label":"Grand","formula":` + addOf(`{"subtotal":"both"}`, `{"subtotal":"stripes"}`) + `}`
}

// TestWriteExportSamples writes, with a logo and a brand color, sample.pdf
// and sample.xlsx of the zoo statement layout (sections, section totals,
// computed rows) and sample-grouped.pdf and sample-grouped.xlsx of the
// grouped zoo report, into the directory EXPORT_SAMPLE_DIR names, for a
// person to look at. With the variable unset it does nothing.
func TestWriteExportSamples(t *testing.T) {
	dir := os.Getenv("EXPORT_SAMPLE_DIR")
	if dir == "" {
		t.Skip("EXPORT_SAMPLE_DIR is not set")
	}
	s := newSavedZoo(t)
	s.app.Settings().Meta.AppName = "Zoo Trust"
	col, err := s.app.FindCollectionByNameOrId("org_branding")
	if err != nil {
		t.Fatal(err)
	}
	branding := core.NewRecord(col)
	branding.Set("brand_color", "#0b4f4a")
	logo, err := filesystem.NewFileFromBytes(sampleLogo(t), "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	branding.Set("logo", logo)
	if err := s.app.Save(branding); err != nil {
		t.Fatal(err)
	}
	s.rec(t, "refund", "zoo_feedings", map[string]any{"animal": s.id("leo"), "cost": -123450, "currency": "USD", "fed": "2026-03-15 00:00:00.000Z", "note": "refund"})
	statement := s.saved(t, "statement", "ann", "Feeding Costs Statement", fmt.Sprintf(zooCells, sampleStatementRows()), false)
	grouped := s.saved(t, "sheet", "ann", "Feedings by Species", feedingsSheet, false)
	options := `{"expandAll":true,"display":{"negative":"parens","negativeRed":true},"today":"2026-11-16","timeZone":"America/Chicago"}`
	for _, sample := range []struct{ name, reportID, params string }{
		{"sample", statement.Id, `{"asOf":"2027-02-15"}`},
		{"sample-grouped", grouped.Id, `{"period":{"from":"2026-01-01","to":"2026-03-31"},"rows":"all"}`},
	} {
		for _, format := range []string{"pdf", "xlsx"} {
			file := buildExportAs(t, s, "ann", sample.reportID, format, sample.params, options)
			if err := os.WriteFile(filepath.Join(dir, sample.name+"."+format), file.Bytes, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
}
