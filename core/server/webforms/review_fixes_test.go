package webforms

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/embedpolicy"
)

func powFor(t *testing.T, f *fixture) string {
	t.Helper()
	key, _ := subkey(f.app, powLabel)
	c, _ := newChallenge(key, f.pub.Id, time.Now())
	raw, _ := json.Marshal(solve(c))
	return base64.StdEncoding.EncodeToString(raw)
}

// C1: a retry of a stored submission (the response was lost) must get ok,
// even though its proof of work was spent by the first try.
func TestReview_RetryOfStoredPowSubmissionIsOk(t *testing.T) {
	f := newFixture(t, contactDef)
	f.pub.Set("require_pow", true)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	body := base(t, f, "lost-response")
	body["pow"] = powFor(t, f)
	submit(t, f, "first", body, 200, []string{`"ok":true`})
	submit(t, f, "retry", body, 200, []string{`"ok":true`})
	if countSubmissions(t, f) != 1 {
		t.Fatal("the retry must not store a second submission")
	}
}

// C1: a submit refused after the proof-of-work check must not spend it.
func TestReview_RefusedSubmitKeepsPowUsable(t *testing.T) {
	f := newFixture(t, contactDef)
	f.pub.Set("require_pow", true)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	pow := powFor(t, f)
	bad := base(t, f, "fix-me")
	bad["values"] = `{}`
	bad["pow"] = pow
	submit(t, f, "missing required field", bad, 400, []string{`"values.name"`})
	good := base(t, f, "fix-me")
	good["pow"] = pow
	submit(t, f, "fixed and sent again", good, 200, []string{`"ok":true`})
}

// I2: an old (regenerated) token still frames, so the page can say
// "not available" inside the client's site.
func TestReview_UnknownTokenStillFrames(t *testing.T) {
	f := newFixture(t, contactDef)
	frame := func(path string) string {
		return embedpolicy.FrameAncestors(f.app, httptest.NewRequest(http.MethodGet, path, nil))
	}
	if got := frame("/p/forms/regenerated-away"); got != "*" {
		t.Fatalf("an unknown token page frames anywhere, got %q", got)
	}
	for _, p := range []string{"/p/forms/x/y", "/p/forms/", "/a/settings"} {
		if got := frame(p); got != "'none'" {
			t.Fatalf("%s: want 'none', got %q", p, got)
		}
	}
}

// I3: keys that are not fields of this form are dropped, not validated
// against every other web form.
func TestReview_ForeignKeysAreDropped(t *testing.T) {
	f := newFixture(t, contactDef)
	body := base(t, f, "foreign")
	body["values"] = `{"name":"Ada","not_a_field":"x","other_form_key":1}`
	submit(t, f, "foreign keys", body, 200, []string{`"ok":true`})
	sub, err := f.app.FindFirstRecordByData("form_submissions", "client_id", "foreign")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	json.Unmarshal([]byte(sub.GetString("values")), &values)
	if len(values) != 1 || values["name"] != "Ada" {
		t.Fatalf("only this form's fields are stored, got %#v", values)
	}
}

// I3: an oversized values part is refused before it is decoded.
func TestReview_OversizedValuesRefused(t *testing.T) {
	f := newFixture(t, contactDef)
	body := base(t, f, "huge")
	body["values"] = `{"name":"` + strings.Repeat("a", maxValuesBytes+1) + `"}`
	submit(t, f, "huge values", body, 400, []string{`"validation_values_too_large"`})
}

// I4: a body over the publication's cap is refused before it is parsed.
func TestReview_BodyOverCapRefusedBeforeParse(t *testing.T) {
	f := newFixture(t, contactDef)
	f.pub.Set("max_upload_mb", 1)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	big := bytes.Repeat([]byte("x"), 3<<20)
	submit(t, f, "over the cap", base(t, f, "big"), 413, []string{`"status":413`},
		upload{"files.docs", "big.bin", "application/octet-stream", big})
}

// I4: a coarse per-IP limit applies before the body is read at all.
func TestReview_PreParseIPLimit(t *testing.T) {
	f := newFixture(t, contactDef)
	factory := func(testing.TB) *tests.TestApp { return f.app }
	for i := 0; i < preParseLimit; i++ {
		(&tests.ApiScenario{Name: "garbage " + strconv.Itoa(i), Method: http.MethodPost, URL: "/api/forms/public/" + f.token + "/submit",
			Body: strings.NewReader("x"), Headers: map[string]string{"Origin": sameSite, "Content-Type": "text/plain"},
			ExpectedStatus: 400, ExpectedContent: []string{`"status":400`}, TestAppFactory: factory, DisableTestAppCleanup: true}).Test(t)
	}
	(&tests.ApiScenario{Name: "over the pre-parse limit", Method: http.MethodPost, URL: "/api/forms/public/" + f.token + "/submit",
		Body: strings.NewReader("x"), Headers: map[string]string{"Origin": sameSite, "Content-Type": "text/plain"},
		ExpectedStatus: 429, ExpectedContent: []string{`"status":429`}, TestAppFactory: factory, DisableTestAppCleanup: true}).Test(t)
}
