package oauth

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// handleRevokeGrantByID implements POST /oauth/grants/{id}/revoke.
//
// This is deliberately separate from RFC 7009's /oauth/revoke: that endpoint
// authenticates by presenting a token (a JWT with a tcg claim, or a value
// hashing to refresh_token_hash) and reports 200 for an unknown token per
// spec. The Connected apps screen has neither shape — it has a grant ROW id
// read from the browser session — so it needs its own, session-authenticated
// route.
//
// The ownership check (grant.user != re.Auth.Id -> 403) is the security
// property of this endpoint: without it, any signed-in user could revoke
// another user's grant by guessing or enumerating ids.
func handleRevokeGrantByID(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Sign in to manage connected apps", nil)
	}
	if err := rejectOAuthToken(re); err != nil {
		return err
	}

	grantID := re.Request.PathValue("id")
	grant, err := app.FindRecordById(grantsCollection, grantID)
	if err != nil {
		return re.NotFoundError("Grant not found", err)
	}

	if grant.GetString("user") != re.Auth.Id {
		return re.ForbiddenError("You may only revoke your own grants", nil)
	}

	if err := RevokeGrant(app, grant.Id); err != nil {
		return re.InternalServerError("Failed to revoke grant", err)
	}
	return re.JSON(http.StatusOK, map[string]string{"status": "revoked"})
}
