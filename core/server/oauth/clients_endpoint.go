package oauth

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// AdminClientView is the projection the admin screen receives. Built by hand
// rather than returning the record, because oauth_clients has every API rule
// null (superuser-only) and PublicExport is therefore never exercised on this
// collection — there is no `hidden: true` flag doing the redaction for us the
// way there is on oauth_grants. client_secret_hash must not leave the server:
// it is the verifier for a confidential client, and an admin UI has no use for
// it. Listing the fields explicitly means a column added to the collection
// later is invisible here until someone deliberately adds it.
type AdminClientView struct {
	ID           string `json:"id"`
	ClientID     string `json:"client_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Scopes       string `json:"scopes"`
	IsFirstParty bool   `json:"is_first_party"`
	Disabled     bool   `json:"disabled"`
	// ActiveGrants is how many live grants the client currently holds — the
	// number that tells an admin what disabling it will actually cut off.
	ActiveGrants int `json:"active_grants"`
}

// requireAdminSession is the authorization gate every client-administration
// endpoint runs first.
//
// Three checks, none redundant:
//
//   - re.Auth == nil: signed in at all.
//   - rejectOAuthToken: a genuine browser session, never an OAuth access
//     token. This is the important one. Client administration IS the kill
//     switch, so a compromised integration reaching it could disable the
//     clients that would notice it, or re-enable itself after being switched
//     off. A stolen token must not be able to touch the control that exists to
//     contain a stolen token.
//   - role: only owner/admin. Every user can manage their OWN grants
//     (Connected apps); the client REGISTRY is org-wide configuration.
//
// Superusers are not special-cased: they administer through the PocketBase
// admin UI, which talks to the collection directly and bypasses this route.
func requireAdminSession(app core.App, re *core.RequestEvent) error {
	if re.Auth == nil {
		return re.UnauthorizedError("Sign in to manage OAuth clients", nil)
	}
	if err := rejectOAuthToken(re); err != nil {
		return err
	}

	user, err := app.FindRecordById(usersCollection, re.Auth.Id)
	if err != nil {
		// Cannot confirm the caller's role — refuse rather than assume.
		return re.ForbiddenError("Admin access required", err)
	}
	if role := user.GetString("role"); role != "owner" && role != "admin" {
		return re.ForbiddenError("Admin access required", nil)
	}
	return nil
}

// handleListClients implements GET /oauth/clients.
//
// Disabled clients are included, not filtered out: the whole point of the
// screen is to see that a client is switched off and to switch it back on.
func handleListClients(app core.App, re *core.RequestEvent) error {
	if err := requireAdminSession(app, re); err != nil {
		return err
	}

	clients, err := app.FindAllRecords(clientsCollection)
	if err != nil {
		return re.InternalServerError("Failed to load OAuth clients", err)
	}

	views := make([]AdminClientView, 0, len(clients))
	for _, c := range clients {
		active, err := countActiveGrantsForClient(app, c.Id)
		if err != nil {
			return re.InternalServerError("Failed to count client grants", err)
		}
		views = append(views, AdminClientView{
			ID:           c.Id,
			ClientID:     c.GetString("client_id"),
			Name:         c.GetString("name"),
			Type:         c.GetString("type"),
			Scopes:       c.GetString("scopes"),
			IsFirstParty: c.GetBool("is_first_party"),
			Disabled:     c.GetBool("disabled"),
			ActiveGrants: active,
		})
	}

	return re.JSON(http.StatusOK, map[string]any{"clients": views})
}

// countActiveGrantsForClient counts the live grants a client holds. Only
// "active" counts: pending grants are unapproved device logins that may never
// complete, and revoked ones are already dead, so including either would
// overstate what disabling the client cuts off.
func countActiveGrantsForClient(app core.App, clientRecordID string) (int, error) {
	grants, err := app.FindAllRecords(
		grantsCollection,
		dbx.HashExp{"client": clientRecordID, "status": "active"},
	)
	if err != nil {
		return 0, err
	}
	return len(grants), nil
}

// setClientDisabledRequest is the body of POST /oauth/clients/{id}/disabled.
//
// An explicit desired state rather than a toggle: a toggle derives the new
// value from what the client last read, so two admins acting on a stale list
// can each flip it and land on the opposite of what both intended. Sending the
// target state makes the request idempotent and safe to retry.
type setClientDisabledRequest struct {
	Disabled bool `json:"disabled"`
}

// handleSetClientDisabled implements POST /oauth/clients/{id}/disabled — the
// operator surface for the kill switch (see oauth_clients.disabled in the
// migration, and its two enforcement points in FindClientByClientID and
// VerifyGrant).
//
// Disabling does NOT revoke the client's grants. The grant rows stay exactly
// as they are, and VerifyGrant refuses them for as long as the client is
// disabled — so re-enabling restores the integration rather than forcing every
// user to reconnect. That is what makes this safe to use decisively during an
// incident: it is a reversible switch, not a destructive one. An admin who
// wants the grants genuinely gone deletes the client, which cascades.
func handleSetClientDisabled(app core.App, re *core.RequestEvent) error {
	if err := requireAdminSession(app, re); err != nil {
		return err
	}

	var body setClientDisabledRequest
	if err := re.BindBody(&body); err != nil {
		return re.BadRequestError("Invalid request body", err)
	}

	client, err := app.FindRecordById(clientsCollection, re.Request.PathValue("id"))
	if err != nil {
		return re.NotFoundError("Client not found", err)
	}

	client.Set("disabled", body.Disabled)
	if err := app.Save(client); err != nil {
		return re.InternalServerError("Failed to update client", err)
	}

	return re.JSON(http.StatusOK, map[string]any{
		"id":       client.Id,
		"disabled": client.GetBool("disabled"),
	})
}
