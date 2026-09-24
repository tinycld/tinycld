package coreserver

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/offboard"
)

// offboard_handlers_test.go proves both offboard entry points run the
// package offboard handlers, with the right actor, and that a handler's plan
// refusal reaches the client as a 400. The handler belongs to a fictional
// "widgets" package: core must not name a real one.

type widgetsCall struct {
	leaverID    string
	plan        offboard.Plan
	actorUserID string
}

func setupOffboardHandlerApp(t *testing.T) (*tests.TestApp, *[]widgetsCall) {
	t.Helper()
	app := setupGuardTestApp(t)
	registerAccountDeleteCore(app)
	registerAdminOffboardCore(app)

	offboard.ResetHandlersForTesting()
	t.Cleanup(offboard.ResetHandlersForTesting)
	var calls []widgetsCall
	offboard.RegisterHandler("widgets", func(_ core.App, leaver *core.Record, plan offboard.Plan, actor string) error {
		calls = append(calls, widgetsCall{leaverID: leaver.Id, plan: plan, actorUserID: actor})
		return nil
	})
	return app, &calls
}

func authToken(t *testing.T, r *core.Record) string {
	t.Helper()
	token, err := r.NewAuthToken()
	if err != nil {
		t.Fatalf("NewAuthToken: %v", err)
	}
	return token
}

func TestAdminOffboard_RunsHandlersWithAdminAsActor(t *testing.T) {
	app, calls := setupOffboardHandlerApp(t)
	makeUserWithRole(t, app, "owner@test.local", "owner")
	admin := makeUserWithRole(t, app, "admin@test.local", "admin")
	leaver := makeUserWithRole(t, app, "leaver@test.local", "member")
	successor := makeUserWithRole(t, app, "successor@test.local", "member")

	scenario := &tests.ApiScenario{
		Name:   "admin offboard runs handlers",
		Method: http.MethodPost,
		URL:    "/api/admin/users/offboard",
		Body: strings.NewReader(fmt.Sprintf(
			`{"user_id":%q,"plan":{"mode":"reassign","successor_user_id":%q}}`, leaver.Id, successor.Id)),
		Headers:               map[string]string{"Authorization": authToken(t, admin)},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"user_anonymized":true`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)

	want := widgetsCall{
		leaverID:    leaver.Id,
		plan:        offboard.Plan{Mode: offboard.ModeReassign, SuccessorUserID: successor.Id},
		actorUserID: admin.Id,
	}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Errorf("handler calls = %+v, want [%+v]", *calls, want)
	}
}

func TestAccountDelete_RunsHandlersWithSelfAsActor(t *testing.T) {
	app, calls := setupOffboardHandlerApp(t)
	makeUserWithRole(t, app, "owner@test.local", "owner")
	leaver := makeUserWithRole(t, app, "leaver@test.local", "member")

	scenario := &tests.ApiScenario{
		Name:                  "self delete runs handlers",
		Method:                http.MethodPost,
		URL:                   "/api/account/delete",
		Body:                  strings.NewReader(`{"email":"leaver@test.local","plan":{"mode":"delete_my_data"}}`),
		Headers:               map[string]string{"Authorization": authToken(t, leaver)},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"user_anonymized":true`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)

	want := widgetsCall{
		leaverID:    leaver.Id,
		plan:        offboard.Plan{Mode: offboard.ModeDeleteMyData},
		actorUserID: leaver.Id,
	}
	if len(*calls) != 1 || (*calls)[0] != want {
		t.Errorf("handler calls = %+v, want [%+v]", *calls, want)
	}
}

// A handler refusal (wrapped ErrInvalidPlan) is a 400 with the handler's
// message, and the account is left as it was.
func TestAccountDelete_HandlerRefusalIsBadRequest(t *testing.T) {
	app, _ := setupOffboardHandlerApp(t)
	makeUserWithRole(t, app, "owner@test.local", "owner")
	leaver := makeUserWithRole(t, app, "leaver@test.local", "member")
	offboard.RegisterHandler("widgets-guard", func(core.App, *core.Record, offboard.Plan, string) error {
		return fmt.Errorf("%w: hand your widgets over first", offboard.ErrInvalidPlan)
	})

	scenario := &tests.ApiScenario{
		Name:                  "handler refusal",
		Method:                http.MethodPost,
		URL:                   "/api/account/delete",
		Body:                  strings.NewReader(`{"email":"leaver@test.local","plan":{"mode":"delete_my_data"}}`),
		Headers:               map[string]string{"Authorization": authToken(t, leaver)},
		ExpectedStatus:        http.StatusBadRequest,
		ExpectedContent:       []string{`hand your widgets over first`},
		TestAppFactory:        func(_ testing.TB) *tests.TestApp { return app },
		DisableTestAppCleanup: true,
	}
	scenario.Test(t)

	still, _ := app.FindRecordById("users", leaver.Id)
	if still.GetString("name") == "Deleted user" {
		t.Error("account anonymized despite the handler refusal")
	}
}
