package branding

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/filesystem"

	"tinycld.org/core/rlstest"
)

// A 1x1 lossless WebP.
const webpBase64 = "UklGRhoAAABXRUJQVlA4TA0AAAAvAAAAEAcQERGIiP4HAA=="

func newApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
	app.Settings().Meta.AppName = "Acme Incorporated"
	return app
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.RGBA{200, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return b.Bytes()
}

func saveBranding(t *testing.T, app core.App, fields map[string]any, logo []byte, logoName string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("org_branding")
	if err != nil {
		t.Fatalf("find org_branding: %v", err)
	}
	r := core.NewRecord(col)
	for k, v := range fields {
		r.Set(k, v)
	}
	if logo != nil {
		f, err := filesystem.NewFileFromBytes(logo, logoName)
		if err != nil {
			t.Fatalf("file: %v", err)
		}
		r.Set("logo", f)
	}
	if err := app.Save(r); err != nil {
		t.Fatalf("save branding: %v", err)
	}
	return r
}

func TestLoad_NoBrandingRowGivesTheNameAndTheDefaultColor(t *testing.T) {
	org := Load(newApp(t))
	if org.Name != "Acme Incorporated" || org.Color != DefaultColor || org.Logo != nil || org.LogoType != "" {
		t.Fatalf("Load = %+v", org)
	}
	if DefaultColor != "#0d9488" {
		t.Errorf("DefaultColor = %q, want the email accent #0d9488", DefaultColor)
	}
}

func TestLoad_ReadsTheColorAndAPngLogo(t *testing.T) {
	app := newApp(t)
	logo := pngBytes(t)
	saveBranding(t, app, map[string]any{"brand_color": "#1E40AF"}, logo, "logo.png")
	org := Load(app)
	if org.Color != "#1e40af" {
		t.Errorf("Color = %q, want #1e40af", org.Color)
	}
	if org.LogoType != "image/png" || !bytes.Equal(org.Logo, logo) {
		t.Errorf("logo = %s, %d bytes", org.LogoType, len(org.Logo))
	}
}

func TestLoad_ReadsAWebpLogo(t *testing.T) {
	app := newApp(t)
	logo, _ := base64.StdEncoding.DecodeString(webpBase64)
	saveBranding(t, app, map[string]any{}, logo, "logo.webp")
	if org := Load(app); org.LogoType != "image/webp" || len(org.Logo) == 0 || org.Color != DefaultColor {
		t.Errorf("Load = %s, %d bytes, %s", org.LogoType, len(org.Logo), org.Color)
	}
}

// The row names a logo but its file is gone from storage: print without it.
func TestLoad_UnreadableLogoFileDegrades(t *testing.T) {
	app := newApp(t)
	r := saveBranding(t, app, map[string]any{"brand_color": "#1e40af"}, pngBytes(t), "logo.png")
	fsys, err := app.NewFilesystem()
	if err != nil {
		t.Fatalf("filesystem: %v", err)
	}
	if err := fsys.Delete(r.BaseFilesPath() + "/" + r.GetString("logo")); err != nil {
		t.Fatalf("delete logo file: %v", err)
	}
	fsys.Close()
	org := Load(app)
	if org.Logo != nil || org.LogoType != "" || org.Color != "#1e40af" {
		t.Errorf("Load = %s, %d bytes, %s", org.LogoType, len(org.Logo), org.Color)
	}
}

// A color that bypassed validation (a raw SQL write) never reaches a renderer.
func TestLoad_IgnoresAStoredColorThatIsNotHex(t *testing.T) {
	app := newApp(t)
	col, _ := app.FindCollectionByNameOrId("org_branding")
	r := core.NewRecord(col)
	r.Set("brand_color", "red;}")
	if err := app.SaveNoValidate(r); err != nil {
		t.Fatalf("save: %v", err)
	}
	if org := Load(app); org.Color != DefaultColor {
		t.Errorf("Color = %q, want the default", org.Color)
	}
}

func TestBrandColorField_AcceptsHexOrEmptyOnly(t *testing.T) {
	app := newApp(t)
	col, _ := app.FindCollectionByNameOrId("org_branding")
	for value, ok := range map[string]bool{
		"": true, "#0d9488": true, "#ABCDEF": true,
		"red": false, "#12345": false, "#1234567": false, "0d9488": false, "#12345g": false,
	} {
		r := core.NewRecord(col)
		r.Set("brand_color", value)
		err := app.Save(r)
		if ok && err != nil {
			t.Errorf("brand_color %q refused: %v", value, err)
		}
		if !ok && err == nil {
			t.Errorf("brand_color %q accepted", value)
		}
	}
}

func TestIsValidColor(t *testing.T) {
	for value, ok := range map[string]bool{
		"#0d9488": true, "#ABCDEF": true, "#aBc123": true,
		"": false, "red": false, "#12345": false, "#1234567": false, "0d9488": false, "#12345g": false, "red;}": false,
	} {
		if got := IsValidColor(value); got != ok {
			t.Errorf("IsValidColor(%q) = %v, want %v", value, got, ok)
		}
	}
}

func TestIsLogoType(t *testing.T) {
	for value, ok := range map[string]bool{
		"image/png": true, "image/jpeg": true, "image/webp": true,
		"image/svg+xml": false, "image/gif": false, "text/plain": false, "": false,
	} {
		if got := IsLogoType(value); got != ok {
			t.Errorf("IsLogoType(%q) = %v, want %v", value, got, ok)
		}
	}
}
