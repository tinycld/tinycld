package webforms

import (
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"tinycld.org/core/forms"
)

const (
	filePartPrefix   = "files."
	defaultMaxFileMb = 10
)

// checkFiles maps file parts to their file fields and checks count, type and
// size. It returns the parts by field key.
func checkFiles(def *forms.Definition, parts map[string][]*multipart.FileHeader, maxUploadMb int) (map[string][]*multipart.FileHeader, validation.Errors) {
	errs := validation.Errors{}
	byKey := map[string][]*multipart.FileHeader{}
	var total int64
	for name, headers := range parts {
		key := strings.TrimPrefix(name, filePartPrefix)
		errKey := "values." + key
		f := def.Field(key)
		if !strings.HasPrefix(name, filePartPrefix) || f == nil || f.Kind != "file" {
			errs[errKey] = validation.NewError("validation_unknown_file_field", "Not a file field")
			continue
		}
		if len(headers) > 1 && !f.Multiple {
			errs[errKey] = validation.NewError("validation_one_file", f.Label+" takes one file")
			continue
		}
		limit := float64(defaultMaxFileMb)
		if f.MaxSizeMb != nil {
			limit = *f.MaxSizeMb
		}
		for _, h := range headers {
			total += h.Size
			if len(f.Accept) > 0 && !acceptMatches(f.Accept, h) {
				errs[errKey] = validation.NewError("validation_file_type", h.Filename+" is not an accepted type")
				break
			}
			if float64(h.Size) > limit*(1<<20) {
				errs[errKey] = validation.NewError("validation_file_size", fmt.Sprintf("%s is larger than %g MB", h.Filename, limit))
				break
			}
		}
		byKey[key] = headers
	}
	if len(errs) == 0 && total > int64(maxUploadMb)<<20 {
		for key := range byKey {
			errs["values."+key] = validation.NewError("validation_upload_total", fmt.Sprintf("The files together are larger than %d MB", maxUploadMb))
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return byKey, nil
}

// acceptMatches reads accept entries as the HTML accept attribute does: a MIME
// type, a type/* family, or a .extension.
func acceptMatches(accept []string, h *multipart.FileHeader) bool {
	ctype := strings.ToLower(h.Header.Get("Content-Type"))
	ext := strings.ToLower(filepath.Ext(h.Filename))
	for _, a := range accept {
		a = strings.ToLower(strings.TrimSpace(a))
		switch {
		case strings.HasPrefix(a, "."):
			if a == ext {
				return true
			}
		case strings.HasSuffix(a, "/*"):
			if strings.HasPrefix(ctype, strings.TrimSuffix(a, "*")) {
				return true
			}
		case a == ctype:
			return true
		}
	}
	return false
}
