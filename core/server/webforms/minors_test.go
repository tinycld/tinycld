package webforms

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// m1: the accept list is checked against the sniffed type, not the client's.
func TestAcceptUsesTheSniffedType(t *testing.T) {
	f := newFixture(t, contactDef) // cv accepts application/pdf
	submit(t, f, "exe labelled as pdf", base(t, f, "sniff"), 400, []string{`"values.cv"`},
		upload{"files.cv", "cv.pdf", "application/pdf", []byte("MZ\x90\x00\x03\x00\x00\x00")})
	submit(t, f, "real pdf", base(t, f, "sniff2"), 200, []string{`"ok":true`},
		upload{"files.cv", "cv.pdf", "application/octet-stream", []byte("%PDF-1.4\n%%EOF\n")})
}

// m2: a long file name is cut, keeping its extension, not a failed save.
func TestLongFileNameIsShortened(t *testing.T) {
	if got := safeFileName(strings.Repeat("a", 300) + ".pdf"); len(got) > 255 || !strings.HasSuffix(got, ".pdf") {
		t.Fatalf("got %d bytes %q", len(got), got[len(got)-8:])
	}
	f := newFixture(t, contactDef)
	submit(t, f, "long name", base(t, f, "long"), 200, []string{`"ok":true`},
		upload{"files.cv", strings.Repeat("b", 300) + ".pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n")})
}

// m3: the user agent is cut on a character boundary.
func TestTruncateRunes(t *testing.T) {
	s := strings.Repeat("é", 200) // 400 bytes
	got := truncateRunes(s, 300)
	if !utf8ValidString(got) || len([]rune(got)) > 300 {
		t.Fatalf("got %q", got)
	}
}

// m4: one app's traffic does not spend another app's budget.
func TestLimitersArePerApp(t *testing.T) {
	a := newFixture(t, contactDef)
	for i := 0; i < 5; i++ {
		submit(t, a, "a", base(t, a, "pa"+string(rune('0'+i))), 200, []string{`"ok":true`})
	}
	b := newFixture(t, contactDef)
	submit(t, b, "b is not limited by a", base(t, b, "pb"), 200, []string{`"ok":true`})
}

// m5: X-Forwarded-Host "a, b" counts as its first entry.
func TestForwardedHostCommaList(t *testing.T) {
	f := newFixture(t, contactDef)
	f.app.Settings().TrustedProxy.Headers = []string{"X-Forwarded-Host"}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/x", nil)
	r.Header.Set("Origin", "https://forms.example")
	r.Header.Set("X-Forwarded-Host", "forms.example, internal.proxy")
	if !sameOrigin(f.app, r) {
		t.Fatal("the first forwarded host is the public host")
	}
}

// m6: deleting a publication that has submissions is refused, not a silent
// delete of every submission.
func TestPublicationWithSubmissionsCannotBeDeleted(t *testing.T) {
	f := newFixture(t, contactDef)
	submit(t, f, "one", base(t, f, "keep"), 200, []string{`"ok":true`})
	if err := f.app.Delete(f.pub); err == nil {
		t.Fatal("a publication with submissions must not be deleted")
	}
}

// m7: the public form response is not cached.
func TestPublicFormIsNotCached(t *testing.T) {
	f := newFixture(t, contactDef)
	(&tests.ApiScenario{Name: "no-store", Method: http.MethodGet, URL: "/api/forms/public/" + f.token,
		ExpectedStatus: 200, ExpectedContent: []string{`"stamp"`},
		AfterTestFunc: func(t testing.TB, _ *tests.TestApp, res *http.Response) {
			if res.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q", res.Header.Get("Cache-Control"))
			}
		},
		TestAppFactory: func(testing.TB) *tests.TestApp { return f.app }, DisableTestAppCleanup: true}).Test(t)
}

// m12: with no web form host bound, nothing is served or stored.
func TestNoHostNoForm(t *testing.T) {
	f := newFixtureWithoutHost(t, contactDef)
	submit(t, f, "unvalidated", base(t, f, "nohost"), 404, []string{`"status":404`})
}

// m13: a required file field is filled by its upload.
func TestRequiredFileField(t *testing.T) {
	def := `{"version":1,"pages":[{"key":"main","title":"Main"}],"fields":[
{"key":"cv","page":"main","label":"CV","kind":"file","required":true}]}`
	f := newFixture(t, def)
	empty := base(t, f, "nofile")
	empty["values"] = `{}`
	submit(t, f, "missing", empty, 400, []string{`"values.cv"`, `"validation_required"`})
	withFile := base(t, f, "file")
	withFile["values"] = `{}`
	submit(t, f, "present", withFile, 200, []string{`"ok":true`},
		upload{"files.cv", "cv.pdf", "application/pdf", []byte("%PDF-1.4\n%%EOF\n")})
}
