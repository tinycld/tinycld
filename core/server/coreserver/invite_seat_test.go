package coreserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

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
