// Package branding gives server renderers (printed documents, PDF files) the
// organization's identity: its name, logo and brand color. It reads the one
// org_branding row the Organization branding settings write.
package branding

import (
	"database/sql"
	"errors"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
	"tinycld.org/core/mailer"
)

// DefaultColor is the brand color when none is set: the accent of the
// transactional emails, so a deployment's documents and emails agree.
const DefaultColor = mailer.BrandColor

// maxLogoBytes is the org_branding logo field's maxSize (2 MB).
const maxLogoBytes = 2 << 20

var (
	log      = logging.ForPackage("branding")
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	// logoTypes are the logo field's allowed types, the images a renderer can embed.
	logoTypes = []string{"image/png", "image/jpeg", "image/webp"}
)

// IsValidColor reports whether s is a #rrggbb hex color, in either case.
func IsValidColor(s string) bool { return hexColor.MatchString(s) }

// IsLogoType reports whether mime is a logo type a document can embed.
func IsLogoType(mime string) bool { return slices.Contains(logoTypes, mime) }

// Org is the organization's identity as a renderer prints it.
type Org struct {
	// Name is the organization name (Settings().Meta.AppName).
	Name string
	// Logo is the uploaded logo, nil when none is set or it cannot be read.
	Logo []byte
	// LogoType is Logo's MIME type (image/png, image/jpeg or image/webp); "" when Logo is nil.
	LogoType string
	// Color is the brand color as lowercase #rrggbb, DefaultColor when none is set.
	Color string
}

// Load reads the organization's branding. It never fails: with no branding
// row, no logo, or a logo file that cannot be read, a document prints with the
// name and the default color, as /api/org-info answers "no logo" in the same
// cases. A logo the row names but storage cannot serve is logged.
func Load(app core.App) Org {
	org := Org{Name: app.Settings().Meta.AppName, Color: DefaultColor}
	record, err := app.FindFirstRecordByFilter("org_branding", "id != ''")
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			log.Warn("read org_branding", "error", err)
		}
		return org
	}
	if c := record.GetString("brand_color"); IsValidColor(c) {
		org.Color = strings.ToLower(c)
	}
	if name := record.GetString("logo"); name != "" {
		org.Logo, org.LogoType = readLogo(app, record, name)
	}
	return org
}

// readLogo returns the logo's bytes and sniffed type, or nil and "" when the
// file cannot be read or is not one of logoTypes.
func readLogo(app core.App, record *core.Record, name string) ([]byte, string) {
	fsys, err := app.NewFilesystem()
	if err != nil {
		log.Warn("open file storage for the logo", "error", err)
		return nil, ""
	}
	defer fsys.Close()
	r, err := fsys.GetReader(record.BaseFilesPath() + "/" + name)
	if err != nil {
		log.Warn("read the logo file", "file", name, "error", err)
		return nil, ""
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, maxLogoBytes+1))
	if err != nil || len(data) > maxLogoBytes {
		log.Warn("read the logo file", "file", name, "error", err, "bytes", len(data))
		return nil, ""
	}
	kind := mimetype.Detect(data)
	if t := kind.String(); IsLogoType(t) {
		return data, t
	}
	log.Warn("the logo is not an image a document can embed", "file", name, "type", kind.String())
	return nil, ""
}
