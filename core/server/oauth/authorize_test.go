package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// pendingUserCodeGrant seeds a pending device grant identified by its
// user_code, for exercising the browser-side approval flow. It is distinct
// from token_test.go's pendingDeviceGrant, which is keyed by device_code and
// exercises the CLI-side poll — same "pending device grant" concept, two
// different lookup keys, so both helpers are kept.
func pendingUserCodeGrant(t *testing.T, app *tests.TestApp) (userCode string, userID string) {
	t.Helper()
	uid, clientRecID := seedUserAndClient(t, app)

	code, err := newUserCode()
	if err != nil {
		t.Fatalf("newUserCode: %v", err)
	}
	col, _ := app.FindCollectionByNameOrId(grantsCollection)
	jti, _ := randomToken(24)
	g := core.NewRecord(col)
	g.Set("client", clientRecID)
	g.Set("jti", jti)
	g.Set("scopes", ScopeMailRead)
	g.Set("status", "pending")
	g.Set("user_code", code)
	dc, _ := randomToken(32)
	g.Set("device_code", hashSecret(dc))
	g.Set("expires_at", time.Now().Add(DeviceCodeTTL))
	if err := app.Save(g); err != nil {
		t.Fatalf("save pending grant: %v", err)
	}
	return code, uid
}

func TestApproveDeviceBindsUserAndActivates(t *testing.T) {
	app := newSchemaApp(t)
	userCode, userID := pendingUserCodeGrant(t, app)

	form := url.Values{}
	form.Set("user_code", userCode)
	form.Set("device_label", "Nathan's laptop")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user // the consent screen runs inside an authenticated session

	if err := handleApproveDevice(app, re); err != nil {
		t.Fatalf("handleApproveDevice: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	grant, err := FindGrantByUserCode(app, userCode)
	if err == nil && grant != nil && grant.GetString("user_code") != "" {
		// user_code should still be present until the token exchange consumes it
		if grant.GetString("status") != "active" {
			t.Fatalf("status = %q, want active", grant.GetString("status"))
		}
		if grant.GetString("user") != userID {
			t.Fatalf("grant user = %q, want %q", grant.GetString("user"), userID)
		}
		if grant.GetString("device_label") != "Nathan's laptop" {
			t.Errorf("device_label not stored")
		}
	} else {
		t.Fatalf("grant not found after approval: %v", err)
	}
}

func TestApproveDeviceRequiresAuthentication(t *testing.T) {
	app := newSchemaApp(t)
	userCode, _ := pendingUserCodeGrant(t, app)

	form := url.Values{}
	form.Set("user_code", userCode)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = nil // anonymous

	// handleApproveDevice reports failure via a returned *router.ApiError
	// (re.UnauthorizedError), which — called directly, with no router in
	// front of it — never touches rec.Code. That write only happens in
	// router error-handling middleware, which does not run in this harness.
	// So the real signal here is the returned error's status, not rec.Code.
	err := handleApproveDevice(app, re)
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("an anonymous caller must not be able to approve a device (status = %d, err = %v)", status, err)
	}
}

func TestApproveDeviceRejectsUnknownUserCode(t *testing.T) {
	app := newSchemaApp(t)
	userID, _ := seedUserAndClient(t, app)
	user, _ := app.FindRecordById("users", userID)

	form := url.Values{}
	form.Set("user_code", "ZZZZ-ZZZZ")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/authorize",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = user

	// Same signal choice as TestApproveDeviceRequiresAuthentication above:
	// handleApproveDevice's rejection paths return a *router.ApiError that
	// only the router's error middleware (absent here) would translate into
	// rec.Code, so rec.Code is not a real signal for this call shape.
	err := handleApproveDevice(app, re)
	if status := apiStatus(err); status != http.StatusNotFound {
		t.Fatalf("an unknown user_code must not approve anything (status = %d, err = %v)", status, err)
	}
}

func TestRevokeMarksGrantRevoked(t *testing.T) {
	app := newSchemaApp(t)
	deviceCode, _ := approvedDeviceGrant(t, app)

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", deviceCode)
	form.Set("client_id", "tinycld-cli")
	_, issued := postToken(t, app, form)

	revokeForm := url.Values{}
	revokeForm.Set("token", issued.RefreshToken)
	revokeForm.Set("client_id", "tinycld-cli")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke",
		strings.NewReader(revokeForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec

	if err := handleRevoke(app, re); err != nil {
		t.Fatalf("handleRevoke: %v", err)
	}
	// RFC 7009 §2.2: always 200, even for an unknown token.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	if _, err := VerifyGrant(app, grantIDFromToken(issued.AccessToken)); err == nil {
		t.Fatal("the grant must not verify after revocation")
	}
}

func TestRevokeUnknownTokenStillReturns200(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	form := url.Values{}
	form.Set("token", "never-issued")
	form.Set("client_id", "tinycld-cli")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/oauth/revoke",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec

	_ = handleRevoke(app, re)
	// Per RFC 7009 an unknown token is not an error — answering otherwise
	// turns the endpoint into a token oracle.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an unknown token", rec.Code)
	}
}

func TestMetadataAdvertisesSupportedGrants(t *testing.T) {
	app := newSchemaApp(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet,
		"/.well-known/oauth-authorization-server", nil)
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec

	if err := handleMetadata(app, re); err != nil {
		t.Fatalf("handleMetadata: %v", err)
	}

	var md MetadataResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &md); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if md.TokenEndpoint == "" || md.DeviceAuthorizationEndpoint == "" {
		t.Fatal("metadata must advertise the token and device endpoints")
	}
	if len(md.CodeChallengeMethodsSupported) != 1 ||
		md.CodeChallengeMethodsSupported[0] != MethodS256 {
		t.Fatalf("must advertise S256 only, got %v", md.CodeChallengeMethodsSupported)
	}
	for _, want := range []string{grantTypeDevice, grantTypeAuthCode, grantTypeRefresh} {
		var found bool
		for _, g := range md.GrantTypesSupported {
			if g == want {
				found = true
			}
		}
		if !found {
			t.Errorf("metadata omits grant type %q", want)
		}
	}
	if len(md.ScopesSupported) == 0 {
		t.Error("metadata must advertise the scope catalog")
	}
}
