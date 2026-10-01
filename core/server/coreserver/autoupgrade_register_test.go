package coreserver

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
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
	// The "no delegate" scenario depends on the process global; do not trust
	// whatever an earlier test left installed.
	autoupgrade.SetDelegate(nil)
	t.Cleanup(func() { autoupgrade.SetDelegate(nil) })
	app := adminConsoleTestApp(t)
	registerAutoUpgradeOn(app)
	ownerTok, err := newUser(t, app, "owner@x.test", "owner", false).NewAuthToken()
	mustNil(t, err)
	adminTok, err := newUser(t, app, "admin@x.test", "admin", false).NewAuthToken()
	mustNil(t, err)
	seed, err := app.FindFirstRecordByFilter("system_settings", "key = 'autoupgrade.enabled'")
	mustNil(t, err)

	settingsCol, err := app.FindCollectionByNameOrId("system_settings")
	mustNil(t, err)
	otherRow := core.NewRecord(settingsCol)
	otherRow.Set("key", "mail.postmark_server_token")
	otherRow.Set("value", "whatever")
	mustNil(t, app.Save(otherRow))

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
		{
			// Renaming the key off the autoupgrade.* prefix must not let the
			// guard see only the new, unprotected key — it is still the row
			// that controls the policy, by virtue of its ORIGINAL key.
			Name:                  "an admin cannot rename the flag's key out of the guard",
			Method:                http.MethodPatch,
			URL:                   "/api/collections/system_settings/records/" + seed.Id,
			Body:                  strings.NewReader(`{"key":"x.enabled"}`),
			Headers:               map[string]string{"Authorization": adminTok},
			ExpectedStatus:        http.StatusForbidden,
			ExpectedContent:       []string{`"status":403`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
			AfterTestFunc: func(t testing.TB, app *tests.TestApp, _ *http.Response) {
				row, err := app.FindRecordById("system_settings", seed.Id)
				if err != nil {
					t.Fatalf("FindRecordById: %v", err)
				}
				if row.GetString("key") != "autoupgrade.enabled" {
					t.Errorf("key should be unchanged, got %q", row.GetString("key"))
				}
			},
		},
		{
			Name:                  "an admin cannot create a new autoupgrade row",
			Method:                http.MethodPost,
			URL:                   "/api/collections/system_settings/records",
			Body:                  strings.NewReader(`{"key":"autoupgrade.window","value":"00:00-01:00"}`),
			Headers:               map[string]string{"Authorization": adminTok},
			ExpectedStatus:        http.StatusForbidden,
			ExpectedContent:       []string{`"status":403`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			Name:                  "an admin cannot delete the flag",
			Method:                http.MethodDelete,
			URL:                   "/api/collections/system_settings/records/" + seed.Id,
			Headers:               map[string]string{"Authorization": adminTok},
			ExpectedStatus:        http.StatusForbidden,
			ExpectedContent:       []string{`"status":403`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
		{
			// The guard must not narrow non-autoupgrade settings: an admin
			// still has the admin console's normal system_settings access.
			Name:                  "an admin can still edit an unrelated system setting",
			Method:                http.MethodPatch,
			URL:                   "/api/collections/system_settings/records/" + otherRow.Id,
			Body:                  strings.NewReader(`{"value":"updated"}`),
			Headers:               map[string]string{"Authorization": adminTok},
			ExpectedStatus:        http.StatusOK,
			ExpectedContent:       []string{`"value":"updated"`},
			TestAppFactory:        factory,
			DisableTestAppCleanup: true,
		},
	}
	for _, sc := range scenarios {
		sc.Test(t)
	}
}
