package coreserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/offboard"
	"tinycld.org/core/quota"
)

// The seat refusal reaches the client with its own message, not "Failed to
// invite member".
func TestInviteMember_SeatLimitMessageReachesTheClient(t *testing.T) {
	app := setupInviteTestApp(t)
	owner := mustCreateUser(t, app, "owner-seat@test.local", false)
	setUserRole(t, app, owner, "owner")
	used, err := quota.CountSeats(app)
	if err != nil {
		t.Fatal(err)
	}
	quota.SetSeatLimit(app, func(core.App) int { return used })
	quota.BindSeats(app)
	registerInviteEndpointCore(app)

	tok, err := tokenForUser(app, owner)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "newperson", "role": "member"})
	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/invite-member",
		Body:                  strings.NewReader(string(body)),
		Headers:               map[string]string{"Authorization": tok},
		ExpectedStatus:        http.StatusPaymentRequired,
		ExpectedContent:       []string{"User limit reached"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// A row deleted before tombstones freed the username (old data) still holds
// it. Inviting that username is refused; it must never re-send the deleted
// account's invite.
func TestInviteMember_UsernameOfAnOldDeletedUserIsRefused(t *testing.T) {
	app := setupInviteTestApp(t)
	users, _ := app.FindCollectionByNameOrId("users")
	users.Fields.Add(&core.DateField{Name: "deleted"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}
	owner := mustCreateUser(t, app, "owner-old@test.local", false)
	setUserRole(t, app, owner, "owner")

	// The pre-tombstone shape: sentinel email, unverified, username kept.
	old := core.NewRecord(users)
	old.Set("username", "holiday")
	old.SetEmail("placeholder@test.local")
	old.SetPassword("Password123!")
	old.Set("role", "owner")
	if err := app.Save(old); err != nil {
		t.Fatal(err)
	}
	old.SetEmail("deleted-" + old.Id + "@deleted.tinycld.org")
	old.SetVerified(false)
	if err := app.Save(old); err != nil {
		t.Fatal(err)
	}
	registerInviteEndpointCore(app)

	tok, err := tokenForUser(app, owner)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "holiday", "role": "member"})
	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/invite-member",
		Body:                  strings.NewReader(string(body)),
		Headers:               map[string]string{"Authorization": tok},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"removed account"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
		AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
			gone, err := app.FindRecordById("users", old.Id)
			if err != nil {
				t.Fatal(err)
			}
			if gone.GetString("username") != "holiday" || gone.GetBool("verified") {
				t.Fatalf("old row changed: %q verified=%v", gone.GetString("username"), gone.GetBool("verified"))
			}
			if live, _ := liveTokenForUser(app, gone); live != nil {
				t.Fatal("the removed account got an invite token")
			}
		},
	}
	scenario.Test(t)
}

// An invite link sent before the member was removed must not bring the
// deleted account back.
func TestAcceptInvite_RefusesADeletedUser(t *testing.T) {
	app := setupInviteTestApp(t)
	users, _ := app.FindCollectionByNameOrId("users")
	users.Fields.Add(&core.DateField{Name: "deleted"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}
	pending := mustCreateUser(t, app, "pending@test.local", false)
	setUserRole(t, app, pending, "member")
	token, err := mintInviteToken(app, pending, "member")
	if err != nil {
		t.Fatal(err)
	}
	if err := offboard.AnonymizeUser(app, pending.Id); err != nil {
		t.Fatal(err)
	}
	registerInviteEndpointCore(app)

	for _, method := range []string{http.MethodGet, http.MethodPost} {
		scenario := &tests.ApiScenario{
			Name:                  method,
			Method:                method,
			URL:                   "/api/accept-invite/" + token,
			Body:                  strings.NewReader(`{"password":"Password123!","name":"Back"}`),
			ExpectedStatus:        http.StatusGone,
			ExpectedContent:       []string{"withdrawn"},
			TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
			DisableTestAppCleanup: true,
		}
		scenario.Test(t)
	}
}

// An admin cannot mint or send a new link for a removed account.
func TestInviteLink_RefusesADeletedUser(t *testing.T) {
	app := setupInviteTestApp(t)
	users, _ := app.FindCollectionByNameOrId("users")
	users.Fields.Add(&core.DateField{Name: "deleted"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}
	owner := mustCreateUser(t, app, "owner-link@test.local", false)
	setUserRole(t, app, owner, "owner")
	pending := mustCreateUser(t, app, "pending-link@test.local", false)
	setUserRole(t, app, pending, "member")
	if err := offboard.AnonymizeUser(app, pending.Id); err != nil {
		t.Fatal(err)
	}
	RegisterInviteLinkEndpoints(app)
	tok, err := tokenForUser(app, owner)
	if err != nil {
		t.Fatal(err)
	}
	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/invite-link/" + pending.Id + "/rotate",
		Headers:               map[string]string{"Authorization": tok},
		ExpectedStatus:        http.StatusNotFound,
		ExpectedContent:       []string{"User not found"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// The deleted- prefix belongs to removed accounts; a new member cannot take it.
func TestInviteMember_RefusesTheTombstonePrefix(t *testing.T) {
	app := setupInviteTestApp(t)
	owner := mustCreateUser(t, app, "owner-prefix@test.local", false)
	setUserRole(t, app, owner, "owner")
	registerInviteEndpointCore(app)
	tok, err := tokenForUser(app, owner)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"username": "deleted-foo", "role": "member"})
	scenario := &tests.ApiScenario{
		Method:                http.MethodPost,
		URL:                   "/api/invite-member",
		Body:                  strings.NewReader(string(body)),
		Headers:               map[string]string{"Authorization": tok},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{"reserved"},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}
