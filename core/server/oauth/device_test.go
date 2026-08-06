package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestValidateScopesRejectsUnknown(t *testing.T) {
	if err := ValidateScopes([]string{ScopeMailRead}); err != nil {
		t.Fatalf("ValidateScopes on a known scope: %v", err)
	}
	if err := ValidateScopes([]string{"mail:read", "not-a-real-scope"}); err == nil {
		t.Fatal("ValidateScopes must reject an unknown scope")
	}
	// An empty request is fine — it defaults to `profile` at issue time.
	if err := ValidateScopes(nil); err != nil {
		t.Fatalf("ValidateScopes on empty: %v", err)
	}
}

func TestFindClientByClientID(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	c, err := FindClientByClientID(app, "tinycld-cli")
	if err != nil {
		t.Fatalf("FindClientByClientID: %v", err)
	}
	if c.GetString("name") != "TinyCld CLI" {
		t.Fatalf("wrong client resolved: %s", c.GetString("name"))
	}

	if _, err := FindClientByClientID(app, "unregistered-app"); err == nil {
		t.Fatal("an unregistered client_id must not resolve")
	}
}

func TestDeviceAuthorizationIssuesCodes(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("client_id", "tinycld-cli")
	form.Set("scope", "mail:read drive:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/device",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if err := serveDeviceForTest(app, rec, req); err != nil {
		t.Fatalf("device authorization: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	var resp DeviceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.DeviceCode == "" {
		t.Error("device_code must be present")
	}
	if resp.UserCode == "" {
		t.Error("user_code must be present")
	}
	if resp.VerificationURI == "" {
		t.Error("verification_uri must be present")
	}
	if resp.Interval <= 0 {
		t.Error("interval must be a positive number of seconds")
	}
	if resp.ExpiresIn <= 0 {
		t.Error("expires_in must be positive")
	}
	// The device code must never equal the user code: one is secret, the
	// other is read aloud.
	if resp.DeviceCode == resp.UserCode {
		t.Error("device_code and user_code must differ")
	}
}

func TestDeviceAuthorizationRejectsUnknownClient(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("client_id", "not-registered")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/device",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// handleDeviceAuthorization is called directly here, bypassing the router,
	// so an ApiError never reaches rec — the router's central error handler is
	// what writes rec.Code, and that layer isn't running in this test. The
	// returned error is the only signal available, same as apiStatus is used
	// for enforceGrant in middleware_test.go.
	err := serveDeviceForTest(app, rec, req)
	if status := apiStatus(err); status == 0 || status == http.StatusOK {
		t.Fatalf("an unregistered client must not receive device codes; err=%v", err)
	}
}

func TestDeviceAuthorizationRejectsUnknownScope(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("client_id", "tinycld-cli")
	form.Set("scope", "mail:read wat:everything")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/device",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err := serveDeviceForTest(app, rec, req)
	if status := apiStatus(err); status == 0 || status == http.StatusOK {
		t.Fatalf("an unknown scope must be rejected, not silently dropped; err=%v", err)
	}
}

// serveDeviceForTest drives handleDeviceAuthorization against a recorder
// without standing up the whole router.
func serveDeviceForTest(app core.App, rec *httptest.ResponseRecorder, req *http.Request) error {
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	return handleDeviceAuthorization(app, re)
}
