package classifier_test

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"tinycld.org/core/classifier"
	"tinycld.org/core/classifier/fakeprovider"
)

func statusUser(t *testing.T, app core.App, users *core.Collection, email, role string) string {
	t.Helper()
	u := core.NewRecord(users)
	u.SetEmail(email)
	u.SetPassword("password123")
	u.Set("role", role)
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	token, err := u.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func TestStatusRoute(t *testing.T) {
	app := classifyApp(t, fakeprovider.New(), nil)
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	users.Fields.Add(&core.TextField{Name: "role"})
	if err := app.Save(users); err != nil {
		t.Fatal(err)
	}
	member := statusUser(t, app, users, "member@test.local", "member")
	guest := statusUser(t, app, users, "guest@test.local", "guest")
	testApp := app.(*tests.TestApp)
	classifier.Register(testApp, superuserGuard)
	factory := func(testing.TB) *tests.TestApp { return testApp }
	url := "/api/classifier/status"
	for _, sc := range []tests.ApiScenario{
		{Name: "member reads", Method: http.MethodGet, URL: url, Headers: map[string]string{"Authorization": member}, ExpectedStatus: 200, ExpectedContent: []string{`"available":true`}},
		{Name: "guest refused", Method: http.MethodGet, URL: url, Headers: map[string]string{"Authorization": guest}, ExpectedStatus: 403, ExpectedContent: []string{`"status":403`}},
		{Name: "anonymous refused", Method: http.MethodGet, URL: url, ExpectedStatus: 401, ExpectedContent: []string{`"status":401`}},
	} {
		sc.TestAppFactory = factory
		sc.DisableTestAppCleanup = true
		sc.Test(t)
	}
}
