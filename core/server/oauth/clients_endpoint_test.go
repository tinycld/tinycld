package oauth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// addRoleField adds the `role` select field to users. Same situation as
// addDisabledField: the bundled test fixture's users collection does not carry
// it (it arrives via a core migration newSchemaApp does not replay), so any
// test exercising the admin gate must add it first.
func addRoleField(t *testing.T, app core.App) {
	t.Helper()
	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	if users.Fields.GetByName("role") != nil {
		return
	}
	users.Fields.Add(&core.SelectField{
		Name:      "role",
		MaxSelect: 1,
		Values:    []string{"owner", "admin", "member", "guest"},
	})
	if err := app.Save(users); err != nil {
		t.Fatalf("add users.role: %v", err)
	}
}

// seedUserWithRole creates a user holding the given role.
func seedUserWithRole(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	addRoleField(t, app)

	users, err := app.FindCollectionByNameOrId(usersCollection)
	if err != nil {
		t.Fatalf("find users: %v", err)
	}
	u := core.NewRecord(users)
	u.Set("email", email)
	u.Set("password", "s3cret-password")
	u.Set("role", role)
	if err := app.Save(u); err != nil {
		t.Fatalf("save %s user: %v", role, err)
	}
	return u
}

func newListClientsEvent(app core.App, auth *core.Record) *core.RequestEvent {
	re := &core.RequestEvent{App: app}
	re.Request = httptest.NewRequest(http.MethodGet, "/oauth/clients", nil)
	re.Response = httptest.NewRecorder()
	re.Auth = auth
	return re
}

// newSetDisabledEvent builds POST /oauth/clients/{id}/disabled as the handler
// reads it directly, bypassing the router — so PathValue must be set by hand,
// same convention as newRevokeRequestEvent.
func newSetDisabledEvent(
	app core.App, clientRecID string, disabled bool, auth *core.Record,
) (*core.RequestEvent, *httptest.ResponseRecorder) {
	body, err := json.Marshal(setClientDisabledRequest{Disabled: disabled})
	if err != nil {
		panic(err)
	}
	req := httptest.NewRequest(
		http.MethodPost, "/oauth/clients/"+clientRecID+"/disabled", strings.NewReader(string(body)),
	)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", clientRecID)

	rec := httptest.NewRecorder()
	re := &core.RequestEvent{App: app}
	re.Request = req
	re.Response = rec
	re.Auth = auth
	return re, rec
}

func TestListClientsRequiresAuthentication(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	err := handleListClients(app, newListClientsEvent(app, nil))
	if status := apiStatus(err); status != http.StatusUnauthorized {
		t.Fatalf("anonymous list = %d, want 401 (err = %v)", status, err)
	}
}

// TestListClientsRejectsNonAdmin: every signed-in user may manage their OWN
// grants, but the client registry is org-wide configuration. A member seeing
// which integrations exist is an information leak; a member disabling one is
// an outage.
func TestListClientsRejectsNonAdmin(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)
	member := seedUserWithRole(t, app, "member@example.com", "member")

	err := handleListClients(app, newListClientsEvent(app, member))
	if status := apiStatus(err); status != http.StatusForbidden {
		t.Fatalf("member list = %d, want 403 (err = %v)", status, err)
	}
}

// TestListClientsRejectsOAuthToken is the property that matters most on this
// endpoint. These routes ARE the kill switch: if a stolen access token could
// read and mutate the client registry, it could disable whatever would detect
// it, or switch itself back on after an admin killed it. Session only —
// enforced in the handler, independent of the scope table, so a future
// exemption cannot silently reopen it.
func TestListClientsRejectsOAuthToken(t *testing.T) {
	app := newSchemaApp(t)
	_, clientID := seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	// A real OAuth token belonging to the ADMIN — the strongest version of the
	// attack. Even full admin authority must not carry through a bearer token.
	grant, err := NewGrant(app, admin.Id, clientID, []string{ScopeProfile}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	token, err := MintAccessToken(app, admin, grant, AccessTokenTTL)
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}

	re := newListClientsEvent(app, admin)
	re.Request.Header.Set("Authorization", "Bearer "+token)

	if status := apiStatus(handleListClients(app, re)); status != http.StatusForbidden {
		t.Fatalf("admin's OAuth token list = %d, want 403", status)
	}

	// And the same for the mutating half.
	setRe, _ := newSetDisabledEvent(app, clientID, true, admin)
	setRe.Request.Header.Set("Authorization", "Bearer "+token)
	if status := apiStatus(handleSetClientDisabled(app, setRe)); status != http.StatusForbidden {
		t.Fatalf("admin's OAuth token disable = %d, want 403", status)
	}

	reloaded, err := app.FindRecordById(clientsCollection, clientID)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	if reloaded.GetBool("disabled") {
		t.Fatal("a rejected request must not have changed the client's disabled state")
	}
}

// TestListClientsNeverReturnsSecretHash: oauth_clients has every API rule null,
// so PublicExport is never exercised on this collection and no `hidden: true`
// flag is doing the redaction. The hand-built projection IS the protection, so
// it needs its own test.
func TestListClientsNeverReturnsSecretHash(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)
	seedConfidentialClient(t, app, "zapier", "super-secret-value")
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	re := newListClientsEvent(app, admin)
	rec := httptest.NewRecorder()
	re.Response = rec

	if err := handleListClients(app, re); err != nil {
		t.Fatalf("handleListClients: %v", err)
	}

	// Assert on the raw wire bytes, not a decoded struct: decoding into
	// AdminClientView would drop an unexpected field silently, which is
	// exactly the leak this guards against.
	raw := rec.Body.String()
	for _, forbidden := range []string{"client_secret_hash", hashSecret("super-secret-value")} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("the client list response must not contain %q; got %s", forbidden, raw)
		}
	}
}

func TestListClientsReturnsClientsWithGrantCounts(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	// Two active grants, plus one revoked and one pending that must NOT count:
	// counting them would overstate what disabling the client cuts off.
	for i := 0; i < 2; i++ {
		if _, err := NewGrant(app, userID, clientID, []string{ScopeProfile}, "active"); err != nil {
			t.Fatalf("NewGrant active %d: %v", i, err)
		}
	}
	if _, err := NewGrant(app, userID, clientID, []string{ScopeProfile}, "revoked"); err != nil {
		t.Fatalf("NewGrant revoked: %v", err)
	}
	if _, err := NewGrant(app, userID, clientID, []string{ScopeProfile}, "pending"); err != nil {
		t.Fatalf("NewGrant pending: %v", err)
	}

	re := newListClientsEvent(app, admin)
	rec := httptest.NewRecorder()
	re.Response = rec

	if err := handleListClients(app, re); err != nil {
		t.Fatalf("handleListClients: %v", err)
	}

	var payload struct {
		Clients []AdminClientView `json:"clients"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	var cli *AdminClientView
	for i := range payload.Clients {
		if payload.Clients[i].ClientID == "tinycld-cli" {
			cli = &payload.Clients[i]
		}
	}
	if cli == nil {
		t.Fatalf("tinycld-cli missing from the list: %s", rec.Body.String())
	}
	if cli.ActiveGrants != 2 {
		t.Errorf("ActiveGrants = %d, want 2 (revoked and pending must not count)", cli.ActiveGrants)
	}
	if !cli.IsFirstParty || cli.Type != "public" {
		t.Errorf("client metadata wrong: %+v", cli)
	}
}

// TestListClientsIncludesDisabledClients: the screen exists to show that a
// client is switched off and to switch it back on, so filtering disabled ones
// out would strand them — invisible and un-restorable from the UI.
func TestListClientsIncludesDisabledClients(t *testing.T) {
	app := newSchemaApp(t)
	_, clientID := seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	client, err := app.FindRecordById(clientsCollection, clientID)
	if err != nil {
		t.Fatalf("find client: %v", err)
	}
	client.Set("disabled", true)
	if err := app.Save(client); err != nil {
		t.Fatalf("save disabled client: %v", err)
	}

	re := newListClientsEvent(app, admin)
	rec := httptest.NewRecorder()
	re.Response = rec
	if err := handleListClients(app, re); err != nil {
		t.Fatalf("handleListClients: %v", err)
	}

	var payload struct {
		Clients []AdminClientView `json:"clients"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Clients) != 1 || !payload.Clients[0].Disabled {
		t.Fatalf("a disabled client must still be listed, and marked disabled: %s", rec.Body.String())
	}
}

func TestSetClientDisabledRequiresAdmin(t *testing.T) {
	app := newSchemaApp(t)
	_, clientID := seedUserAndClient(t, app)
	member := seedUserWithRole(t, app, "member@example.com", "member")

	re, _ := newSetDisabledEvent(app, clientID, true, member)
	if status := apiStatus(handleSetClientDisabled(app, re)); status != http.StatusForbidden {
		t.Fatalf("member disable = %d, want 403", status)
	}

	re, _ = newSetDisabledEvent(app, clientID, true, nil)
	if status := apiStatus(handleSetClientDisabled(app, re)); status != http.StatusUnauthorized {
		t.Fatalf("anonymous disable = %d, want 401", status)
	}

	reloaded, err := app.FindRecordById(clientsCollection, clientID)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	if reloaded.GetBool("disabled") {
		t.Fatal("a rejected request must not have disabled the client")
	}
}

// TestSetClientDisabledRoundTrip drives the switch through the endpoint and
// confirms the effect reaches the actual enforcement point — disabling here
// must make FindClientByClientID refuse the client, and re-enabling must
// restore it. Asserting on the flag alone would pass even if the two halves
// were wired to different fields.
func TestSetClientDisabledRoundTrip(t *testing.T) {
	app := newSchemaApp(t)
	_, clientID := seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	re, _ := newSetDisabledEvent(app, clientID, true, admin)
	if err := handleSetClientDisabled(app, re); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := FindClientByClientID(app, "tinycld-cli"); err == nil {
		t.Fatal("after disabling, the client must not resolve")
	}

	re, _ = newSetDisabledEvent(app, clientID, false, admin)
	if err := handleSetClientDisabled(app, re); err != nil {
		t.Fatalf("re-enable: %v", err)
	}
	if _, err := FindClientByClientID(app, "tinycld-cli"); err != nil {
		t.Fatalf("after re-enabling, the client must resolve again: %v", err)
	}
}

// TestSetClientDisabledPreservesGrants pins the reversibility that makes this
// safe to use during an incident: disabling must not destroy grant rows, so
// re-enabling restores the integration instead of forcing every user to
// reconnect. If this ever starts revoking, that is a product decision that
// needs to be made deliberately, not acquired by accident.
func TestSetClientDisabledPreservesGrants(t *testing.T) {
	app := newSchemaApp(t)
	userID, clientID := seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	grant, err := NewGrant(app, userID, clientID, []string{ScopeProfile}, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}

	re, _ := newSetDisabledEvent(app, clientID, true, admin)
	if err := handleSetClientDisabled(app, re); err != nil {
		t.Fatalf("disable: %v", err)
	}

	reloaded, err := app.FindRecordById(grantsCollection, grant.Id)
	if err != nil {
		t.Fatalf("the grant row must survive disabling its client: %v", err)
	}
	if reloaded.GetString("status") != "active" {
		t.Errorf("grant status = %q, want it untouched at \"active\"", reloaded.GetString("status"))
	}
	// ...but the token it backs must stop working while the client is off.
	if _, err := VerifyGrant(app, grant.GetString("jti")); err == nil {
		t.Fatal("a grant belonging to a disabled client must not verify")
	}
}

func TestSetClientDisabledNotFound(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	re, _ := newSetDisabledEvent(app, "does-not-exist", true, admin)
	if status := apiStatus(handleSetClientDisabled(app, re)); status != http.StatusNotFound {
		t.Fatalf("unknown client id = %d, want 404", status)
	}
}

// TestSetClientDisabledIsIdempotent: the request carries the DESIRED state
// rather than a toggle, so a retry (or two admins acting on the same stale
// list) converges instead of flipping back and forth.
func TestSetClientDisabledIsIdempotent(t *testing.T) {
	app := newSchemaApp(t)
	_, clientID := seedUserAndClient(t, app)
	admin := seedUserWithRole(t, app, "admin@example.com", "admin")

	for i := 0; i < 3; i++ {
		re, _ := newSetDisabledEvent(app, clientID, true, admin)
		if err := handleSetClientDisabled(app, re); err != nil {
			t.Fatalf("disable %d: %v", i, err)
		}
	}

	reloaded, err := app.FindRecordById(clientsCollection, clientID)
	if err != nil {
		t.Fatalf("reload client: %v", err)
	}
	if !reloaded.GetBool("disabled") {
		t.Fatal("repeating the same desired state must converge on it")
	}
}
