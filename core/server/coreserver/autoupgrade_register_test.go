package coreserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/autoupgrade"
)

type recordingDelegate struct{ calls []bool }

func (d *recordingDelegate) PolicyChanged(_ context.Context, enabled bool) error {
	d.calls = append(d.calls, enabled)
	return nil
}
func (d *recordingDelegate) Status(context.Context) (autoupgrade.Status, error) {
	return autoupgrade.Status{Available: true, LastResult: "no updates"}, nil
}

func TestPolicyHookCallsDelegate(t *testing.T) {
	app := adminConsoleTestApp(t)
	d := &recordingDelegate{}
	autoupgrade.SetDelegate(d)
	t.Cleanup(func() { autoupgrade.SetDelegate(nil) })
	app.OnRecordAfterUpdateSuccess("system_settings").BindFunc(notifyPolicy)

	row, err := app.FindFirstRecordByFilter("system_settings", "key = 'autoupgrade.enabled'")
	mustNil(t, err)
	row.Set("value", "false")
	mustNil(t, app.Save(row))
	if len(d.calls) != 1 || d.calls[0] != false {
		t.Fatalf("calls %v", d.calls)
	}
}

func TestStatusRouteAndWriteGuard(t *testing.T) {
	app := adminConsoleTestApp(t)
	registerAutoUpgradeOn(app)
	ownerTok, err := newUser(t, app, "owner@x.test", "owner", false).NewAuthToken()
	mustNil(t, err)
	adminTok, err := newUser(t, app, "admin@x.test", "admin", false).NewAuthToken()
	mustNil(t, err)
	seed, err := app.FindFirstRecordByFilter("system_settings", "key = 'autoupgrade.enabled'")
	mustNil(t, err)
	factory := func(testing.TB) *tests.TestApp { return app }

	scenarios := []tests.ApiScenario{
		{
			// requireOwner answers every unauthorized case with ForbiddenError
			// (403), including a missing token — there is no 401 path.
			Name:                  "status needs auth",
			Method:                http.MethodGet,
			URL:                   "/api/admin/packages/auto-upgrade/status",
			ExpectedStatus:        http.StatusForbidden,
			ExpectedContent:       []string{`"status":403`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:                  "status for the owner, no delegate",
			Method:                http.MethodGet,
			URL:                   "/api/admin/packages/auto-upgrade/status",
			Headers:               map[string]string{"Authorization": ownerTok},
			ExpectedStatus:        http.StatusOK,
			ExpectedContent:       []string{`"available":false`, `"windowManaged":false`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:                  "an admin cannot change the flag",
			Method:                http.MethodPatch,
			URL:                   "/api/collections/system_settings/records/" + seed.Id,
			Body:                  strings.NewReader(`{"value":"false"}`),
			Headers:               map[string]string{"Authorization": adminTok},
			ExpectedStatus:        http.StatusForbidden,
			ExpectedContent:       []string{`"status":403`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:                  "the owner can change the flag",
			Method:                http.MethodPatch,
			URL:                   "/api/collections/system_settings/records/" + seed.Id,
			Body:                  strings.NewReader(`{"value":"false"}`),
			Headers:               map[string]string{"Authorization": ownerTok},
			ExpectedStatus:        http.StatusOK,
			ExpectedContent:       []string{`"value":"false"`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
	}
	for _, sc := range scenarios {
		sc.Test(t)
	}
}
