package automation

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func retryToken(t *testing.T, u *core.Record, role string, app core.App) string {
	t.Helper()
	u.Set("role", role)
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	tok, err := u.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func TestRetryEndpoint(t *testing.T) {
	app, eng, u, save := dynamicApp(t)
	tapp := app.(*tests.TestApp)
	registerEndpoints(tapp, eng)
	rule := dynamicRule(t, app, u.Id, "a", `{"match":"all","groups":[]}`, "x")
	mine := save("a", "y")
	other := save("b", "z")
	tickets, _ := app.FindCollectionByNameOrId("tickets")
	ticket := core.NewRecord(tickets)
	ticket.Set("title", "t")
	if err := app.Save(ticket); err != nil {
		t.Fatal(err)
	}
	admin := retryToken(t, u, "admin", app)
	factory := func(testing.TB) *tests.TestApp { return tapp }
	url := func(record string) string { return "/api/automation/rules/" + rule.Id + "/records/" + record + "/run" }
	run := func(name, record, token string, status int, content string) {
		(&tests.ApiScenario{Name: name, Method: http.MethodPost, URL: url(record),
			Headers: map[string]string{"Authorization": token}, ExpectedStatus: status,
			ExpectedContent: []string{content}, TestAppFactory: factory, DisableTestAppCleanup: true}).Test(t)
	}

	run("admin retries the rule's own record", mine.Id, admin, 200, `"queued":true`)
	run("a record of another collection is not found", ticket.Id, admin, 404, `"status":404`)
	run("a record the rule filter refuses is not found", other.Id, admin, 404, `This rule does not run for that record.`)

	member := retryToken(t, u, "member", app)
	run("a member may not run an org rule", mine.Id, member, 403, `"status":403`)

	u.Set("role", "admin")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	rule.Set("enabled", false)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	run("a disabled rule is not run", mine.Id, admin, 400, `Turn this rule on to run it again.`)
}
