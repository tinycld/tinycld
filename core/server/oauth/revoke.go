package oauth

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("oauth")

// handleRevoke implements RFC 7009 token revocation.
//
// Per §2.2 the response is 200 whether or not the token existed: answering
// differently would turn this into an oracle telling an attacker which tokens
// are real.
func handleRevoke(app core.App, re *core.RequestEvent) error {
	if err := re.Request.ParseForm(); err != nil {
		return re.BadRequestError("Malformed form body", err)
	}
	token := re.Request.FormValue("token")
	if token == "" {
		return re.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	// The token may be either an access token (carries the grant claim) or a
	// refresh token (matches a stored hash). Try both.
	if jti := grantIDFromToken(token); jti != "" {
		if grant, err := FindGrantByJTI(app, jti); err == nil {
			if err := RevokeGrant(app, grant.Id); err != nil {
				log.WarnContext(re.Request.Context(), "revoke by jti failed", "grantID", grant.Id, "err", err)
			}
		}
		return re.JSON(http.StatusOK, map[string]string{"status": "ok"})
	}

	grant, err := app.FindFirstRecordByFilter(
		grantsCollection, "refresh_token_hash = {:h}",
		map[string]any{"h": hashSecret(token)},
	)
	if err == nil && grant != nil {
		if err := RevokeGrant(app, grant.Id); err != nil {
			log.WarnContext(re.Request.Context(), "revoke by refresh token failed", "grantID", grant.Id, "err", err)
		}
	}
	return re.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
