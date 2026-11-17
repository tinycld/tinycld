package coreserver

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/quota"
)

func TestSeatsEndpoint_ReportsUseAndLimit(t *testing.T) {
	app := setupInviteTestApp(t)
	owner := mustCreateUser(t, app, "owner-seats@test.local", false)
	setUserRole(t, app, owner, "owner")
	quota.SetSeatLimit(app, func(core.App) int { return 3 })
	registerSeatsEndpointCore(app)

	tok, err := tokenForUser(app, owner)
	if err != nil {
		t.Fatal(err)
	}
	used, err := quota.CountSeats(app)
	if err != nil {
		t.Fatal(err)
	}
	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/seats",
		Headers:               map[string]string{"Authorization": tok},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{fmt.Sprintf(`"used":%d`, used), `"limit":3`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}

func TestSeatsEndpoint_RequiresSignIn(t *testing.T) {
	app := setupInviteTestApp(t)
	registerSeatsEndpointCore(app)
	scenario := &tests.ApiScenario{
		Method:                http.MethodGet,
		URL:                   "/api/seats",
		ExpectedStatus:        http.StatusUnauthorized,
		ExpectedContent:       []string{`"data":{}`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)
}
