package userorg

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// TestEndpoint_LeaveOrg_SelfReassign — happy path through the HTTP layer.
// Alice posts /api/account/leave-org with a reassign plan; the server runs
// LeaveOrg and returns 200 with a result body.
func TestEndpoint_LeaveOrg_SelfReassign(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "owner")
	_ = makeEvent(t, app, "Standup", aliceUO.Id, org.Id)

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	body := fmt.Sprintf(`{"user_org_id":"%s","plan":{"mode":"reassign","successor_user_org_id":"%s"}}`,
		aliceUO.Id, bobUO.Id)

	scenario := &tests.ApiScenario{
		Name:           "alice leaves Acme, reassign to bob",
		Method:         http.MethodPost,
		URL:            "/api/account/leave-org",
		Body:           strings.NewReader(body),
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"records_reassigned":1`,
		},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_LeaveOrg_RejectsNonMemberRemover — alice (not a member of
// Acme) tries to remove bob from Acme. Must 403.
func TestEndpoint_LeaveOrg_RejectsNonMemberRemover(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local") // outsider
	bob := makeUser(t, app, "bob@test.local")
	carol := makeUser(t, app, "carol@test.local")
	org := makeOrg(t, app, "Acme")
	bobUO := makeUserOrg(t, app, bob, org, "owner")
	_ = makeUserOrg(t, app, carol, org, "member")

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	body := fmt.Sprintf(`{"user_org_id":"%s","plan":{"mode":"reassign"}}`, bobUO.Id)

	scenario := &tests.ApiScenario{
		Name:           "outsider can't remove bob from Acme",
		Method:         http.MethodPost,
		URL:            "/api/account/leave-org",
		Body:           strings.NewReader(body),
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusForbidden,
		ExpectedContent: []string{
			`"message"`,
		},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_LeaveOrg_AdminRemovesMember — admin Alice removes Bob (a
// member) and reassigns Bob's records to herself.
func TestEndpoint_LeaveOrg_AdminRemovesMember(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "member")
	_ = makeEvent(t, app, "Bob's event", bobUO.Id, org.Id)

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	body := fmt.Sprintf(`{"user_org_id":"%s","plan":{"mode":"reassign","successor_user_org_id":"%s"}}`,
		bobUO.Id, aliceUO.Id)

	scenario := &tests.ApiScenario{
		Name:           "admin alice removes bob from Acme",
		Method:         http.MethodPost,
		URL:            "/api/account/leave-org",
		Body:           strings.NewReader(body),
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"records_reassigned":1`,
			`"user_anonymized":false`, // admin removal doesn't anonymize bob
		},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_Preview returns the count summary + peer list for the UI.
func TestEndpoint_Preview(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	_ = makeUserOrg(t, app, bob, org, "member")
	_ = makeEvent(t, app, "e1", aliceUO.Id, org.Id)
	_ = makeEvent(t, app, "e2", aliceUO.Id, org.Id)

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	scenario := &tests.ApiScenario{
		Name:           "preview returns counts + peers",
		Method:         http.MethodGet,
		URL:            "/api/account/leave-org/preview?user_org_id=" + aliceUO.Id,
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"test_events.created_by":2`,
			`"sole_member":false`,
			`"sole_owner":true`, // bob is a member, not an owner — alice is sole owner
			`bob@test.local`,
		},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_LeaveOrg_CrossOrgAdminCannotRemove — Alice is an owner of
// org A and an outsider to org B. She tries to remove Bob (member of org B)
// via the endpoint. Must 403 — being an admin elsewhere doesn't grant her
// any authority over B's members.
func TestEndpoint_LeaveOrg_CrossOrgAdminCannotRemove(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	orgA := makeOrg(t, app, "A")
	orgB := makeOrg(t, app, "B")
	_ = makeUserOrg(t, app, alice, orgA, "owner") // alice owns A
	bobUO := makeUserOrg(t, app, bob, orgB, "owner")
	carol := makeUser(t, app, "carol@test.local")
	_ = makeUserOrg(t, app, carol, orgB, "member") // B needs a peer so it's not sole-member

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	body := fmt.Sprintf(`{"user_org_id":"%s","plan":{"mode":"reassign"}}`, bobUO.Id)
	scenario := &tests.ApiScenario{
		Name:                  "cross-org admin attack",
		Method:                http.MethodPost,
		URL:                   "/api/account/leave-org",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`"message"`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_LeaveOrg_AdminCannotRemoveOwner — this is the C1 fix:
// an admin (not owner) trying to remove an owner must 403. Otherwise an
// admin could chain remove-each-owner calls and end up as sole owner via
// pickPromotionTarget's auto-elevate. Owner-to-owner removal stays fine
// because the org keeps owners.
func TestEndpoint_LeaveOrg_AdminCannotRemoveOwner(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local") // admin
	bob := makeUser(t, app, "bob@test.local")     // owner — target
	carol := makeUser(t, app, "carol@test.local") // second owner (so bob isn't sole)
	org := makeOrg(t, app, "Acme")
	_ = makeUserOrg(t, app, alice, org, "admin")
	bobUO := makeUserOrg(t, app, bob, org, "owner")
	_ = makeUserOrg(t, app, carol, org, "owner")

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	body := fmt.Sprintf(`{"user_org_id":"%s","plan":{"mode":"reassign"}}`, bobUO.Id)
	scenario := &tests.ApiScenario{
		Name:                  "admin tries to remove owner",
		Method:                http.MethodPost,
		URL:                   "/api/account/leave-org",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusForbidden,
		ExpectedContent:       []string{`Admins cannot remove owners`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_LeaveOrg_OwnerCanRemoveAdmin — confirms the C1 fix doesn't
// over-rotate: an owner can still remove an admin (the demoted case).
func TestEndpoint_LeaveOrg_OwnerCanRemoveAdmin(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local") // owner
	bob := makeUser(t, app, "bob@test.local")     // admin — target
	org := makeOrg(t, app, "Acme")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")
	bobUO := makeUserOrg(t, app, bob, org, "admin")

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	body := fmt.Sprintf(`{"user_org_id":"%s","plan":{"mode":"reassign","successor_user_org_id":"%s"}}`,
		bobUO.Id, aliceUO.Id)
	scenario := &tests.ApiScenario{
		Name:                  "owner removes admin",
		Method:                http.MethodPost,
		URL:                   "/api/account/leave-org",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": token},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"user_anonymized":false`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

// TestEndpoint_Preview_SoleMember — when leaver is the only member of the
// org, preview must say sole_member=true and the UI shows the destructive
// "delete the org" confirmation instead of the reassign picker.
func TestEndpoint_Preview_SoleMember(t *testing.T) {
	app := setupTestApp(t)
	registerCore(app)

	alice := makeUser(t, app, "alice@test.local")
	org := makeOrg(t, app, "Solo")
	aliceUO := makeUserOrg(t, app, alice, org, "owner")

	token, err := alice.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}

	scenario := &tests.ApiScenario{
		Name:           "sole-member preview",
		Method:         http.MethodGet,
		URL:            "/api/account/leave-org/preview?user_org_id=" + aliceUO.Id,
		Headers:        map[string]string{"Authorization": token},
		ExpectedStatus: http.StatusOK,
		ExpectedContent: []string{
			`"sole_member":true`,
		},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}
