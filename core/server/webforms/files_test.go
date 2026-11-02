package webforms

import (
	"bytes"
	"mime/multipart"
	"net/textproto"
	"testing"

	"tinycld.org/core/forms"
)

// sniffedAs holds content that sniffs as each type; checkFiles reads the
// bytes, not the Content-Type the client sent.
var sniffedAs = map[string][]byte{
	"application/pdf": []byte("%PDF-1.4\n%%EOF\n"),
	"image/png":       []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\x0dIHDR"),
}

// header builds a real upload (so Open works) whose content sniffs as ctype,
// and reports size as its size.
func header(name, ctype string, size int64) *multipart.FileHeader {
	body, ok := sniffedAs[ctype]
	if !ok {
		body = []byte("MZ\x90\x00\x03\x00\x00\x00")
	}
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="f"; filename="`+name+`"`)
	h.Set("Content-Type", ctype)
	p, _ := w.CreatePart(h)
	_, _ = p.Write(body)
	_ = w.Close()
	form, err := multipart.NewReader(&buf, w.Boundary()).ReadForm(1 << 20)
	if err != nil {
		panic(err)
	}
	fh := form.File["f"][0]
	fh.Size = size
	return fh
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
	if !acceptMatches([]string{"image/*"}, header("a.png", "image/png", 1), "image/png") {
		t.Error("image/* must match image/png")
	}
	if !acceptMatches([]string{".docx"}, header("CV.DOCX", "application/zip", 1), "application/zip") {
		t.Error(".docx must match by extension, any case")
	}
}
