package oauth

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// MetadataResponse is the RFC 8414 authorization server metadata document.
// A conforming client reads this to discover our endpoints instead of having
// them hard-coded, which is what lets Zapier point at any TinyCld host.
type MetadataResponse struct {
	Issuer                        string   `json:"issuer"`
	AuthorizationEndpoint         string   `json:"authorization_endpoint"`
	TokenEndpoint                 string   `json:"token_endpoint"`
	DeviceAuthorizationEndpoint   string   `json:"device_authorization_endpoint"`
	RevocationEndpoint            string   `json:"revocation_endpoint"`
	UserinfoEndpoint              string   `json:"userinfo_endpoint"`
	ScopesSupported               []string `json:"scopes_supported"`
	ResponseTypesSupported        []string `json:"response_types_supported"`
	GrantTypesSupported           []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethods      []string `json:"token_endpoint_auth_methods_supported"`
}

// handleMetadata serves GET /.well-known/oauth-authorization-server.
func handleMetadata(app core.App, re *core.RequestEvent) error {
	base := strings.TrimSuffix(app.Settings().Meta.AppURL, "/")
	return re.JSON(http.StatusOK, MetadataResponse{
		Issuer:                      base,
		AuthorizationEndpoint:       base + "/p/oauth/authorize",
		TokenEndpoint:               base + "/oauth/token",
		DeviceAuthorizationEndpoint: base + "/oauth/device",
		RevocationEndpoint:          base + "/oauth/revoke",
		UserinfoEndpoint:            base + "/oauth/userinfo",
		ScopesSupported:             AllScopes,
		ResponseTypesSupported:      []string{"code"},
		GrantTypesSupported: []string{
			grantTypeAuthCode, grantTypeRefresh, grantTypeDevice,
		},
		// S256 only — OAuth 2.1 removes `plain`.
		CodeChallengeMethodsSupported: []string{MethodS256},
		TokenEndpointAuthMethods: []string{
			"none", "client_secret_post",
		},
	})
}

// UserinfoResponse is the minimal identity document an integration needs.
type UserinfoResponse struct {
	Sub      string `json:"sub"`
	Email    string `json:"email"`
	Name     string `json:"name,omitempty"`
	Username string `json:"preferred_username,omitempty"`
}

// handleUserinfo serves GET /oauth/userinfo for the authenticated caller.
func handleUserinfo(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Authentication required", nil)
	}
	return re.JSON(http.StatusOK, UserinfoResponse{
		Sub:      re.Auth.Id,
		Email:    re.Auth.GetString("email"),
		Name:     re.Auth.GetString("name"),
		Username: re.Auth.GetString("username"),
	})
}
