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

// TestValidateClientScopesEnforcesClientCeiling is the mutation target for
// Finding 3: oauth_clients.scopes must be an actual ceiling, not a written-
// but-never-read column. A client registered for `profile` only must not be
// able to obtain mail:send/drive:write merely because those scopes exist in
// the global AllScopes catalog.
func TestValidateClientScopesEnforcesClientCeiling(t *testing.T) {
	app := newSchemaApp(t)
	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", "narrow-client")
	c.Set("name", "Narrow Client")
	c.Set("type", "public")
	c.Set("scopes", ScopeProfile+" "+ScopeMailRead)
	if err := app.Save(c); err != nil {
		t.Fatalf("save narrow client: %v", err)
	}

	if err := ValidateClientScopes(c, []string{ScopeMailRead}); err != nil {
		t.Fatalf("a registered scope must be allowed: %v", err)
	}
	if err := ValidateClientScopes(c, []string{ScopeMailSend, ScopeDriveWrite}); err == nil {
		t.Fatal("a client must not be able to obtain a scope outside its own registration, " +
			"even though both scopes are in the global catalog")
	}
}

// TestValidateClientScopesEmptyRegistrationDenies is the explicit test of
// the empty/unset decision documented on ValidateClientScopes: no
// registered scopes means no NON-baseline scopes are grantable, not "every
// scope in the catalog."
func TestValidateClientScopesEmptyRegistrationDenies(t *testing.T) {
	app := newSchemaApp(t)
	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", "unregistered-scopes-client")
	c.Set("name", "No Scopes Registered")
	c.Set("type", "public")
	// scopes intentionally left unset.
	if err := app.Save(c); err != nil {
		t.Fatalf("save client: %v", err)
	}

	if err := ValidateClientScopes(c, []string{ScopeMailRead}); err == nil {
		t.Fatal("an empty client.scopes must deny every non-baseline scope, not allow every scope")
	}
	// The profile default (what both handlers fall back to for an empty
	// request) must still work even with nothing registered.
	if err := ValidateClientScopes(c, []string{ScopeProfile}); err != nil {
		t.Fatalf("ScopeProfile must always be allowed regardless of client registration: %v", err)
	}
	if err := ValidateClientScopes(c, nil); err != nil {
		t.Fatalf("an empty request must still validate (the profile default is applied by the caller): %v", err)
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

// TestDeviceAuthorizationRejectsScopeOutsideClientCeiling is the endpoint-
// level half of Finding 3's fix: a client registered for `profile` only must
// not be able to obtain mail:read through the actual device flow, even
// though mail:read is a perfectly valid scope in the global catalog.
func TestDeviceAuthorizationRejectsScopeOutsideClientCeiling(t *testing.T) {
	app := newSchemaApp(t)
	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", "narrow-device-client")
	c.Set("name", "Narrow Device Client")
	c.Set("type", "public")
	c.Set("scopes", ScopeProfile)
	if err := app.Save(c); err != nil {
		t.Fatalf("save narrow client: %v", err)
	}

	form := url.Values{}
	form.Set("client_id", "narrow-device-client")
	form.Set("scope", "mail:read")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/device",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	err = serveDeviceForTest(app, rec, req)
	if status := apiStatus(err); status == 0 || status == http.StatusOK {
		t.Fatalf("a scope outside the client's own registration must be refused; err=%v", err)
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
