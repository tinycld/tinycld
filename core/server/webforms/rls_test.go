package webforms

import (
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/rlstest"
)

func rlsUser(t *testing.T, app core.App, email, role string) string {
	t.Helper()
	users, _ := app.FindCollectionByNameOrId("users")
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword("password123")
	u.Set("role", role)
	u.Set("name", email)
	u.Set("username", strings.Split(email, "@")[0])
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	token, err := u.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestRLS_SubmissionsAreAdminReadOnly(t *testing.T) {
	app := rlstest.NewAssembledApp(t)
	admin := rlsUser(t, app, "admin@test.local", "admin")
	member := rlsUser(t, app, "member@test.local", "member")
	factory := func(testing.TB) *tests.TestApp { return app }
	create := `{"form":"x","publication":"y","values":{},"client_id":"c"}`
	for _, sc := range []tests.ApiScenario{
		{Name: "admin lists", Method: http.MethodGet, URL: "/api/collections/form_submissions/records", Headers: map[string]string{"Authorization": admin}, ExpectedStatus: 200, ExpectedContent: []string{`"totalItems":0`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "member list is empty by rule", Method: http.MethodGet, URL: "/api/collections/form_submissions/records", Headers: map[string]string{"Authorization": member}, ExpectedStatus: 200, ExpectedContent: []string{`"totalItems":0`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "admin cannot create through REST", Method: http.MethodPost, URL: "/api/collections/form_submissions/records", Body: strings.NewReader(create), Headers: map[string]string{"Authorization": admin}, ExpectedStatus: 403, ExpectedContent: []string{`"status":403`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "anonymous cannot list files", Method: http.MethodGet, URL: "/api/collections/form_submission_files/records", ExpectedStatus: 200, ExpectedContent: []string{`"totalItems":0`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "member cannot create a publication", Method: http.MethodPost, URL: "/api/collections/form_publications/records", Body: strings.NewReader(`{"form":"x"}`), Headers: map[string]string{"Authorization": member}, ExpectedStatus: 400, ExpectedContent: []string{`"Failed to create record."`}, TestAppFactory: factory, DisableTestAppCleanup: true},
	} {
		sc.Test(t)
	}
}
