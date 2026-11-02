package webforms

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/embedpolicy"
)

func TestPublicForm(t *testing.T) {
	f := newFixture(t, contactDef)
	factory := func(testing.TB) *tests.TestApp { return f.app }
	for _, sc := range []tests.ApiScenario{
		{Name: "live form", Method: http.MethodGet, URL: "/api/forms/public/" + f.token, ExpectedStatus: 200,
			ExpectedContent: []string{`"name":"Contact"`, `"confirmation":"Thank you.`, `"slug":"contact"`, `"stamp":"`, `"minDelayMs":3000`, `"requirePow":false`, `"maxUploadMb":25`, `"key":"name"`},
			TestAppFactory:  factory, DisableTestAppCleanup: true},
		{Name: "unknown token", Method: http.MethodGet, URL: "/api/forms/public/nope", ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "no challenge when pow is off", Method: http.MethodGet, URL: "/api/forms/public/" + f.token + "/challenge", ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, TestAppFactory: factory, DisableTestAppCleanup: true},
	} {
		sc.Test(t)
	}

	f.pub.Set("enabled", false)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	(&tests.ApiScenario{Name: "disabled", Method: http.MethodGet, URL: "/api/forms/public/" + f.token, ExpectedStatus: 404, ExpectedContent: []string{`"status":404`}, TestAppFactory: factory, DisableTestAppCleanup: true}).Test(t)

	f.pub.Set("enabled", true)
	f.pub.Set("require_pow", true)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	(&tests.ApiScenario{Name: "challenge when pow is on", Method: http.MethodGet, URL: "/api/forms/public/" + f.token + "/challenge", ExpectedStatus: 200, ExpectedContent: []string{`"algorithm":"SHA-256"`, `"maxnumber":50000`, `"signature":"`}, TestAppFactory: factory, DisableTestAppCleanup: true}).Test(t)
}

func TestEmbedResolver(t *testing.T) {
	f := newFixture(t, contactDef)
	frame := func(path string) string {
		return embedpolicy.FrameAncestors(f.app, httptest.NewRequest(http.MethodGet, path, nil))
	}
	if got := frame("/p/forms/" + f.token); got != "*" {
		t.Fatalf("empty list frames anywhere, got %q", got)
	}
	f.pub.Set("allowed_domains", "https://client.example")
	f.pub.Set("enabled", false)
	if err := f.app.Save(f.pub); err != nil {
		t.Fatal(err)
	}
	// Disabled still frames, so the page can say "not available" inside the iframe.
	if got := frame("/p/forms/" + f.token); got != "https://client.example" {
		t.Fatalf("got %q", got)
	}
	for _, p := range []string{"/p/forms/nope", "/p/forms/" + f.token + "/x", "/p/forms/", "/a/settings"} {
		if got := frame(p); got != "'none'" {
			t.Fatalf("%s: want 'none', got %q", p, got)
		}
	}
}
