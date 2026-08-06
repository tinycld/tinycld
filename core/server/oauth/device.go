package oauth

import (
	"net/http"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// pollInterval is the seconds a client should wait between token polls
// (RFC 8628 §3.2). Five is the spec's own suggested floor.
const pollInterval = 5

// DeviceResponse is the RFC 8628 §3.2 device authorization response.
type DeviceResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	// VerificationURIComplete embeds the code so the CLI can open a browser
	// straight to an approved-looking screen (RFC 8628 §3.3.1).
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// handleDeviceAuthorization implements POST /oauth/device.
//
// It creates a PENDING grant with no user attached yet — the user is bound
// when they approve in the browser. The device_code is the secret the CLI
// polls with; the user_code is the short string they read off the terminal.
func handleDeviceAuthorization(app core.App, re *core.RequestEvent) error {
	if err := re.Request.ParseForm(); err != nil {
		return re.BadRequestError("Malformed form body", err)
	}
	clientID := re.Request.FormValue("client_id")
	client, err := FindClientByClientID(app, clientID)
	if err != nil {
		return re.BadRequestError("Unknown client_id", err)
	}

	scopes := ParseScopes(re.Request.FormValue("scope"))
	if err := ValidateClientScopes(client, scopes); err != nil {
		return re.BadRequestError(err.Error(), err)
	}
	if len(scopes) == 0 {
		scopes = []string{ScopeProfile}
	}

	deviceCode, err := randomToken(32)
	if err != nil {
		return re.InternalServerError("Failed to generate device code", err)
	}
	userCode, err := newUserCode()
	if err != nil {
		return re.InternalServerError("Failed to generate user code", err)
	}

	col, err := app.FindCollectionByNameOrId(grantsCollection)
	if err != nil {
		return re.InternalServerError("Failed to load grants", err)
	}
	jti, err := randomToken(24)
	if err != nil {
		return re.InternalServerError("Failed to generate grant id", err)
	}

	// user is intentionally unset until approval. The relation is required, so
	// the pending row carries an empty string until handleApprove fills it —
	// PocketBase permits that for a relation with no value.
	grant := core.NewRecord(col)
	grant.Set("client", client.Id)
	grant.Set("jti", jti)
	grant.Set("scopes", strings.Join(scopes, " "))
	grant.Set("status", "pending")
	grant.Set("device_code", hashSecret(deviceCode))
	grant.Set("user_code", userCode)
	grant.Set("expires_at", time.Now().Add(DeviceCodeTTL))
	if err := app.Save(grant); err != nil {
		return re.InternalServerError("Failed to create device grant", err)
	}

	base := strings.TrimSuffix(app.Settings().Meta.AppURL, "/")
	verifyURL := base + "/p/oauth/authorize"

	return re.JSON(http.StatusOK, DeviceResponse{
		DeviceCode:              deviceCode,
		UserCode:                userCode,
		VerificationURI:         verifyURL,
		VerificationURIComplete: verifyURL + "?user_code=" + userCode,
		ExpiresIn:               int(DeviceCodeTTL.Seconds()),
		Interval:                pollInterval,
	})
}
