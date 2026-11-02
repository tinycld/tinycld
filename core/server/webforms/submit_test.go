package webforms

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
)

type upload struct {
	part, name, ctype string
	body              []byte
}

// postBody builds the multipart body the public page sends.
func postBody(t *testing.T, fields map[string]string, uploads ...upload) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, u := range uploads {
		h := textproto.MIMEHeader{}
		h.Set("Content-Disposition", `form-data; name="`+u.part+`"; filename="`+u.name+`"`)
		h.Set("Content-Type", u.ctype)
		p, err := w.CreatePart(h)
		if err != nil {
			t.Fatal(err)
		}
		p.Write(u.body)
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

// oldStamp is a stamp issued 5 seconds ago, so it passes the minimum age.
func oldStamp(t *testing.T, f *fixture) string {
	key, err := subkey(f.app, stampLabel)
	if err != nil {
		t.Fatal(err)
	}
	return issueStamp(key, f.pub.Id, time.Now().Add(-5*time.Second))
}

// sameSite is the Origin a real submit carries: httptest requests use Host example.com.
const sameSite = "http://example.com"

func submit(t *testing.T, f *fixture, name string, fields map[string]string, status int, content []string, uploads ...upload) {
	t.Helper()
	submitFrom(t, f, sameSite, name, fields, status, content, uploads...)
}

func submitFrom(t *testing.T, f *fixture, origin, name string, fields map[string]string, status int, content []string, uploads ...upload) {
	t.Helper()
	body, ctype := postBody(t, fields, uploads...)
	headers := map[string]string{"Content-Type": ctype, "Origin": origin}
	(&tests.ApiScenario{
		Name: name, Method: http.MethodPost, URL: "/api/forms/public/" + f.token + "/submit",
		Body: body, Headers: headers, ExpectedStatus: status, ExpectedContent: content,
		TestAppFactory: func(testing.TB) *tests.TestApp { return f.app }, DisableTestAppCleanup: true,
	}).Test(t)
}

func base(t *testing.T, f *fixture, clientID string) map[string]string {
	return map[string]string{"values": `{"name":"Ada"}`, "client_id": clientID, "stamp": oldStamp(t, f), "_hp": ""}
}

func countSubmissions(t *testing.T, f *fixture) int {
	n, err := f.app.CountRecords("form_submissions")
	if err != nil {
		t.Fatal(err)
	}
	return int(n)
}

func TestSubmit_HappyPathWithFiles(t *testing.T) {
	f := newFixture(t, contactDef)
	submit(t, f, "ok", base(t, f, "c1"), 200, []string{`"ok":true`},
		upload{"files.cv", "cv.pdf", "application/pdf", []byte("%PDF")},
		upload{"files.docs", "a.txt", "text/plain", []byte("a")},
		upload{"files.docs", "b.txt", "text/plain", []byte("b")})

	sub, err := f.app.FindFirstRecordByData("form_submissions", "client_id", "c1")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := json.Unmarshal([]byte(sub.GetString("values")), &values); err != nil {
		t.Fatal(err)
	}
	if _, single := values["cv"].(string); !single {
		t.Fatalf("a single file field stores one id, got %#v", values["cv"])
	}
	if docs, many := values["docs"].([]any); !many || len(docs) != 2 {
		t.Fatalf("a multiple file field stores a list, got %#v", values["docs"])
	}
	files, _ := f.app.FindRecordsByFilter("form_submission_files", "submission = {:s}", "", 0, 0, map[string]any{"s": sub.Id})
	if len(files) != 3 {
		t.Fatalf("want 3 file rows, got %d", len(files))
	}
	if sub.GetString("ip_hash") == "" || sub.GetString("ip_hash") == "192.0.2.1" {
		t.Fatal("ip must be stored as a hash")
	}
}

func TestSubmit_Checks(t *testing.T) {
	f := newFixture(t, contactDef)

	submitFrom(t, f, "https://evil.example", "other origin", base(t, f, "o1"), 403, []string{`"status":403`})

	hp := base(t, f, "h1")
	hp["_hp"] = "http://spam.example"
	submit(t, f, "honeypot gets a fake ok", hp, 200, []string{`"ok":true`})
	if countSubmissions(t, f) != 0 {
		t.Fatal("a honeypot hit must store nothing")
	}

	key, _ := subkey(f.app, stampLabel)
	fast := base(t, f, "f1")
	fast["stamp"] = issueStamp(key, f.pub.Id, time.Now())
	submit(t, f, "too fast", fast, 400, []string{`"validation_too_fast"`})

	forged := base(t, f, "g1")
	forged["stamp"] = "x.1.y"
	submit(t, f, "bad stamp", forged, 400, []string{`"validation_invalid_stamp"`})

	missing := base(t, f, "m1")
	missing["values"] = `{}`
	submit(t, f, "required field", missing, 400, []string{`"values.name"`, `"validation_required"`})

	submit(t, f, "wrong file type", base(t, f, "t1"), 400, []string{`"values.cv"`},
		upload{"files.cv", "a.exe", "application/octet-stream", []byte("x")})

	pointed := base(t, f, "x1")
	pointed["values"] = `{"name":"Ada","cv":"someone-elses-file"}`
	submit(t, f, "client value for a file key is dropped", pointed, 200, []string{`"ok":true`})
	sub, err := f.app.FindFirstRecordByData("form_submissions", "client_id", "x1")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	json.Unmarshal([]byte(sub.GetString("values")), &values)
	if _, has := values["cv"]; has {
		t.Fatalf("a file key the visitor did not upload must not be stored, got %#v", values["cv"])
	}
}

func TestSubmit_Idempotent(t *testing.T) {
	f := newFixture(t, contactDef)
	submit(t, f, "first", base(t, f, "same"), 200, []string{`"ok":true`})
	submit(t, f, "retry", base(t, f, "same"), 200, []string{`"ok":true`})
	if countSubmissions(t, f) != 1 {
		t.Fatal("a retried client_id must not store a second submission")
	}
}

func TestSubmit_RateLimit(t *testing.T) {
	f := newFixture(t, contactDef)
	for i := 0; i < 5; i++ {
		submit(t, f, "within limit", base(t, f, "r"+strconv.Itoa(i)), 200, []string{`"ok":true`})
	}
	submit(t, f, "over limit", base(t, f, "rz"), 429, []string{`"status":429`})
}

func TestSubmit_ProofOfWork(t *testing.T) {
	f := newFixture(t, contactDef)
	f.pub.Set("require_pow", true)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	submit(t, f, "missing pow", base(t, f, "p1"), 400, []string{`"validation_invalid_pow"`})

	key, _ := subkey(f.app, powLabel)
	c, _ := newChallenge(key, f.pub.Id, time.Now())
	raw, _ := json.Marshal(solve(c))
	withPow := base(t, f, "p2")
	withPow["pow"] = base64.StdEncoding.EncodeToString(raw)
	submit(t, f, "valid pow", withPow, 200, []string{`"ok":true`})

	replay := base(t, f, "p3")
	replay["pow"] = withPow["pow"]
	submit(t, f, "replayed pow", replay, 400, []string{`"validation_invalid_pow"`})
}

func TestSubmit_NotLive(t *testing.T) {
	f := newFixture(t, contactDef)
	f.pub.Set("enabled", false)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	submit(t, f, "disabled", base(t, f, "d1"), 404, []string{`"status":404`})
}

func TestSameOrigin(t *testing.T) {
	f := newFixture(t, contactDef)
	r := httptest.NewRequest(http.MethodPost, "http://forms.example/x", nil)
	r.Header.Set("Origin", "https://forms.example")
	if !sameOrigin(f.app, r) {
		t.Fatal("same host must pass")
	}
	r.Header.Set("Origin", "https://evil.example")
	if sameOrigin(f.app, r) {
		t.Fatal("another host must fail")
	}
	r.Header.Del("Origin")
	if sameOrigin(f.app, r) {
		t.Fatal("a missing Origin must fail")
	}

	// Behind a proxy that rewrites Host, the public host arrives in
	// X-Forwarded-Host, honored only when the admin trusts that header.
	proxied := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8090/x", nil)
	proxied.Header.Set("Origin", "https://forms.example")
	proxied.Header.Set("X-Forwarded-Host", "forms.example")
	if sameOrigin(f.app, proxied) {
		t.Fatal("an untrusted X-Forwarded-Host must be ignored")
	}
	f.app.Settings().TrustedProxy.Headers = []string{"X-Forwarded-Host"}
	if !sameOrigin(f.app, proxied) {
		t.Fatal("a trusted X-Forwarded-Host must count as this server")
	}
}
