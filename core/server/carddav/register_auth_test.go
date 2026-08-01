package carddav

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A bad Basic credential must produce a 401 WITH a WWW-Authenticate challenge.
// Before authentication moved to the route wrapper, it reached the backend,
// whose bare davauth.ErrUnauthorized go-webdav reports as a 500 — clients read
// that as a server fault and never re-prompt for a password. Found live by the
// router's cross-org CardDAV probe (an org's user probing another org's
// tenant got 500, not 401).

func davRequest(handler http.Handler, email, password string) *httptest.ResponseRecorder {
	body := `<?xml version="1.0" encoding="utf-8"?>
<C:addressbook-query xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:carddav">
  <D:prop><D:getetag/><C:address-data/></D:prop>
</C:addressbook-query>`
	req := httptest.NewRequest("REPORT", "/carddav/u/ab/default/", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Depth", "1")
	if email != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(email + ":" + password))
		req.Header.Set("Authorization", "Basic "+cred)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func TestHandlerFor_BadCredentialIs401WithChallenge(t *testing.T) {
	app, backend, _ := setupPutAuthzApp(t)
	handler := HandlerFor(app, backend.sources)

	cases := map[string]struct{ email, password string }{
		"wrong password": {"alice@example.com", "not-the-password"},
		"unknown user":   {"nobody@example.com", "Password123!"},
		"no credentials": {"", ""},
	}
	for name, c := range cases {
		rec := davRequest(handler, c.email, c.password)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s: status = %d, want 401", name, rec.Code)
		}
		if rec.Header().Get("WWW-Authenticate") == "" {
			t.Errorf("%s: 401 without a WWW-Authenticate challenge — clients cannot re-prompt", name)
		}
	}
}

func TestHandlerFor_GoodCredentialIsServed(t *testing.T) {
	app, backend, _ := setupPutAuthzApp(t)
	handler := HandlerFor(app, backend.sources)

	// Positive control for the denials above: the same request with the right
	// password must reach the protocol handler, not the challenge.
	rec := davRequest(handler, "alice@example.com", "Password123!")
	if rec.Code == http.StatusUnauthorized || rec.Code >= 500 {
		t.Fatalf("authenticated REPORT = %d, body:\n%s", rec.Code, rec.Body.String())
	}
}
