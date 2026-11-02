package webforms

import (
	"mime/multipart"
	"net/textproto"
	"testing"

	"tinycld.org/core/forms"
)

func header(name, ctype string, size int64) *multipart.FileHeader {
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", ctype)
	return &multipart.FileHeader{Filename: name, Header: h, Size: size}
}

func TestCheckFiles(t *testing.T) {
	def, err := forms.ParseDefinition([]byte(contactDef))
	if err != nil {
		t.Fatal(err)
	}
	mb := int64(1 << 20)
	cases := []struct {
		name  string
		parts map[string][]*multipart.FileHeader
		max   int
		bad   string
	}{
		{"ok", map[string][]*multipart.FileHeader{"files.cv": {header("cv.pdf", "application/pdf", mb/2)}}, 25, ""},
		{"unknown key", map[string][]*multipart.FileHeader{"files.nope": {header("a.pdf", "application/pdf", 1)}}, 25, "values.nope"},
		{"not a file field", map[string][]*multipart.FileHeader{"files.name": {header("a.pdf", "application/pdf", 1)}}, 25, "values.name"},
		{"two on single", map[string][]*multipart.FileHeader{"files.cv": {header("a.pdf", "application/pdf", 1), header("b.pdf", "application/pdf", 1)}}, 25, "values.cv"},
		{"wrong type", map[string][]*multipart.FileHeader{"files.cv": {header("a.exe", "application/octet-stream", 1)}}, 25, "values.cv"},
		{"too big for field", map[string][]*multipart.FileHeader{"files.cv": {header("a.pdf", "application/pdf", 2*mb)}}, 25, "values.cv"},
		{"too big in total", map[string][]*multipart.FileHeader{"files.docs": {header("a.bin", "x/y", 6*mb), header("b.bin", "x/y", 6*mb)}}, 10, "values.docs"},
	}
	for _, c := range cases {
		_, errs := checkFiles(def, c.parts, c.max)
		if c.bad == "" && errs != nil {
			t.Errorf("%s: unexpected %v", c.name, errs)
		}
		if c.bad != "" {
			if _, ok := errs[c.bad]; !ok {
				t.Errorf("%s: want an error on %s, got %v", c.name, c.bad, errs)
			}
		}
	}
	if !acceptMatches([]string{"image/*"}, header("a.png", "image/png", 1)) {
		t.Error("image/* must match image/png")
	}
	if !acceptMatches([]string{".docx"}, header("CV.DOCX", "application/zip", 1)) {
		t.Error(".docx must match by extension, any case")
	}
}
