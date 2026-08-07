package oauth

import (
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// AuthorizeInfoResponse describes a pending device request so the consent
// screen can name the client and list the scopes before the user approves.
type AuthorizeInfoResponse struct {
	ClientName string   `json:"client_name"`
	Scopes     []string `json:"scopes"`
	ExpiresAt  string   `json:"expires_at"`
}

// handleAuthorizeInfo implements GET /oauth/authorize?user_code=…
// It is what the consent screen calls to render "TinyCld CLI wants access to…".
func handleAuthorizeInfo(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Sign in to approve this request", nil)
	}
	if err := rejectOAuthToken(re); err != nil {
		return err
	}
	userCode := strings.ToUpper(strings.TrimSpace(re.Request.URL.Query().Get("user_code")))
	grant, err := FindGrantByUserCode(app, userCode)
	if err != nil {
		return re.NotFoundError("That code is not valid", err)
	}
	if grant.GetString("status") != "pending" {
		return re.BadRequestError("That code has already been used", nil)
	}
	if grantExpired(grant) {
		return re.BadRequestError("That code has expired", nil)
	}

	client, err := app.FindRecordById(clientsCollection, grant.GetString("client"))
	if err != nil {
		return re.InternalServerError("Failed to load client", err)
	}
	return re.JSON(http.StatusOK, AuthorizeInfoResponse{
		ClientName: client.GetString("name"),
		Scopes:     ParseScopes(grant.GetString("scopes")),
		ExpiresAt:  grant.GetDateTime("expires_at").String(),
	})
}

// handleApproveDevice implements POST /oauth/authorize for the device flow:
// the signed-in user binds themselves to a pending grant and activates it.
func handleApproveDevice(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Sign in to approve this request", nil)
	}
	if err := rejectOAuthToken(re); err != nil {
		return err
	}
	if err := re.Request.ParseForm(); err != nil {
		return re.BadRequestError("Malformed form body", err)
	}
	userCode := strings.ToUpper(strings.TrimSpace(re.Request.FormValue("user_code")))
	grant, err := FindGrantByUserCode(app, userCode)
	if err != nil {
		return re.NotFoundError("That code is not valid", err)
	}
	if grant.GetString("status") != "pending" {
		return re.BadRequestError("That code has already been used", nil)
	}
	if grantExpired(grant) {
		return re.BadRequestError("That code has expired", nil)
	}

	label := strings.TrimSpace(re.Request.FormValue("device_label"))
	if label == "" {
		label = "Unnamed device"
	}

	// Bind to re.Auth.Id — the authenticated caller — never to a user id
	// supplied in the request body. Anything else would let one user approve
	// a device login on another user's behalf.
	grant.Set("user", re.Auth.Id)
	grant.Set("status", "active")
	grant.Set("device_label", label)
	if err := app.Save(grant); err != nil {
		return re.InternalServerError("Failed to approve device", err)
	}
	return re.JSON(http.StatusOK, map[string]string{"status": "approved"})
}

// handleDenyDevice lets a user reject a device request outright.
//
// Authorization: identical to handleApproveDevice, deliberately. A pending
// grant has no `user` yet — approval is the step that assigns one — so there
// is no owner field to compare the caller against, for either verb. RFC 8628
// defines knowledge of the user_code as the entire binding mechanism: the
// code is shown on the device the legitimate user is sitting at, and typing
// it into an authenticated browser tab is what proves "this is my device
// login." That is exactly as true for rejecting a login as for accepting
// one, and every production device-flow implementation (GitHub, Google,
// Auth0) relies on the same signal for both — the RFC's own countermeasure
// against a wrong/hijacked approval is UX confirmation, not a session tie.
// So "signed in + correct, still-pending user_code" is the complete rule
// here, not a partial one pending some other ownership check.
func handleDenyDevice(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Sign in to manage this request", nil)
	}
	if err := rejectOAuthToken(re); err != nil {
		return err
	}
	if err := re.Request.ParseForm(); err != nil {
		return re.BadRequestError("Malformed form body", err)
	}
	userCode := strings.ToUpper(strings.TrimSpace(re.Request.FormValue("user_code")))
	grant, err := FindGrantByUserCode(app, userCode)
	if err != nil {
		return re.NotFoundError("That code is not valid", err)
	}
	// Mirrors handleApproveDevice's own status check. Without it, a caller
	// who still has the user_code (device_code/user_code are only cleared on
	// TOKEN EXCHANGE, not on approval — see issueTokens) could "deny" a grant
	// the user already approved, silently revoking an active connection the
	// user just authorized on the same screen.
	if grant.GetString("status") != "pending" {
		return re.BadRequestError("That code has already been used", nil)
	}
	// Route through RevokeGrant rather than hand-setting status: a denied
	// pending grant still carries a live device_code/user_code, and those
	// must be cleared the same way any other revocation clears credential
	// material — see RevokeGrant's comment in grants.go.
	if err := RevokeGrant(app, grant.Id); err != nil {
		return re.InternalServerError("Failed to deny request", err)
	}
	return re.JSON(http.StatusOK, map[string]string{"status": "denied"})
}

// handleAuthorize implements the Authorization Code + PKCE path used by
// third-party integrations. It issues a one-shot code bound to the client's
// PKCE challenge and redirect URI.
func handleAuthorize(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Sign in to approve this request", nil)
	}
	if err := rejectOAuthToken(re); err != nil {
		return err
	}
	if err := re.Request.ParseForm(); err != nil {
		return re.BadRequestError("Malformed form body", err)
	}
	q := re.Request.Form

	client, err := FindClientByClientID(app, q.Get("client_id"))
	if err != nil {
		return re.BadRequestError("Unknown client_id", err)
	}
	redirectURI := q.Get("redirect_uri")
	if !RedirectURIAllowed(client, redirectURI) {
		// Never redirect to an unregistered URI — that is an open redirect.
		return re.BadRequestError("redirect_uri is not registered for this client", nil)
	}
	challenge := q.Get("code_challenge")
	if challenge == "" || q.Get("code_challenge_method") != MethodS256 {
		return re.BadRequestError("code_challenge with method S256 is required", nil)
	}
	scopes := ParseScopes(q.Get("scope"))
	if err := ValidateClientScopes(client, scopes); err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	if len(scopes) == 0 {
		scopes = []string{ScopeProfile}
	}

	code, err := randomToken(32)
	if err != nil {
		return re.InternalServerError("Failed to generate code", err)
	}
	grant, err := NewGrant(app, re.Auth.Id, client.Id, scopes, "pending")
	if err != nil {
		return re.InternalServerError("Failed to create grant", err)
	}
	grant.Set("auth_code_hash", hashSecret(code))
	grant.Set("code_challenge", challenge)
	grant.Set("redirect_uri", redirectURI)
	grant.Set("expires_at", time.Now().Add(AuthCodeTTL))
	grant.Set("device_label", client.GetString("name"))
	if err := app.Save(grant); err != nil {
		return re.InternalServerError("Failed to store authorization code", err)
	}

	return re.JSON(http.StatusOK, map[string]string{
		"code":         code,
		"redirect_uri": redirectURI,
		"state":        q.Get("state"),
	})
}
