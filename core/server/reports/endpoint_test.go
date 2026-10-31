package reports

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/oauth"
)

func (z *zoo) post(t *testing.T, name, token, body string, status int, want ...string) {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = token
	}
	sc := tests.ApiScenario{
		Name: name, Method: http.MethodPost, URL: "/api/reports/run",
		Body: strings.NewReader(body), Headers: headers,
		ExpectedStatus: status, ExpectedContent: want,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

func runBodyOf(spec string) string { return `{"spec":` + spec + `}` }

func TestRunEndpoint(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	ann := z.token(t, "ann")

	z.post(t, "a member runs a draft", ann, `{"spec":`+feedingsByDay+`,"params":{},"options":{"limit":10,"display":{"negative":"parens"}}}`,
		http.StatusOK, `"kind":"total"`, `"cost":4500`, `"topics":["collection/zoo_animals","collection/zoo_feedings","collection/zoo_keepers"]`)
	z.post(t, "signed out", "", runBodyOf(feedingsByDay), http.StatusUnauthorized, `"status":401`)
	z.post(t, "an invalid spec", ann, runBodyOf(`{"version":1,"source":"zoo.feedings","columns":[{"id":"1x","path":[{"field":"cost"}]}]}`),
		http.StatusBadRequest, `"message":"The report is not valid."`, `"columns.0.id":{"code":"invalid"`)
	z.post(t, "an unknown column", ann, runBodyOf(`{"version":1,"source":"zoo.feedings","columns":[{"id":"a","path":[{"field":"cost"}]}],"sort":[{"col":"b"}]}`),
		http.StatusUnprocessableEntity, `"sort.0.col":{"code":"unknown_column"`)
	z.post(t, "an unknown source", ann, runBodyOf(`{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`),
		http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	z.post(t, "SQL is not a field", ann, `{"spec":`+feedingsByDay+`,"sql":"DELETE FROM zoo_feedings"}`,
		http.StatusBadRequest, `unknown field`)
	z.post(t, "a second object", ann, `{"spec":`+feedingsByDay+`}{"sql":"x"}`, http.StatusBadRequest, `after the first object`)
	z.post(t, "trailing text", ann, `{"spec":`+feedingsByDay+`} trailing`, http.StatusBadRequest, `after the first object`)
	z.post(t, "no spec", ann, `{"params":{}}`, http.StatusBadRequest, `no spec`)
	z.post(t, "a body too large", ann, `{"spec":{"version":1,"columns":[],"x":"`+strings.Repeat("x", maxRunBody)+`"}}`,
		http.StatusRequestEntityTooLarge, `"status":413`)
	if n, _ := z.app.CountRecords("zoo_feedings"); n != 6 {
		t.Fatalf("the endpoint wrote: %d feedings", n)
	}
}

func TestRunEndpointRefusesASuperuser(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	superusers, err := z.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(superusers)
	su.SetEmail("root@zoo.test")
	su.SetPassword("password123")
	if err := z.app.Save(su); err != nil {
		t.Fatal(err)
	}
	token, err := su.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	z.post(t, "a superuser", token, runBodyOf(feedingsByDay), http.StatusForbidden, `"status":403`)
}

func TestRunEndpointErrors(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	ann := z.token(t, "ann")
	t.Run("the time limit", func(t *testing.T) {
		withLimits(t, func(l *limits) { l.timeout = time.Nanosecond })
		z.post(t, "the time limit", ann, runBodyOf(feedingsByDay), http.StatusRequestTimeout, `took too long`)
	})
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 9223372036854775807 WHERE note IN ('f1', 'f2')").Execute(); err != nil {
		t.Fatal(err)
	}
	z.post(t, "an amount too large", ann, runBodyOf(feedingsByDay), http.StatusUnprocessableEntity, `too large to total`)
}

// An internal error and an amount too large answer a fixed message: the
// statement and the driver's text stay in the server log.
func TestRunEndpointLeaksNoSQL(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	ann := z.token(t, "ann")
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 9223372036854775807 WHERE note IN ('f1', 'f2')").Execute(); err != nil {
		t.Fatal(err)
	}
	for _, leak := range []string{"SELECT", "integer overflow", "zoo_feedings", "sqlite"} {
		sc := tests.ApiScenario{
			Name: "no " + leak, Method: http.MethodPost, URL: "/api/reports/run",
			Body: strings.NewReader(runBodyOf(feedingsByDay)), Headers: map[string]string{"Authorization": ann},
			ExpectedStatus:        http.StatusUnprocessableEntity,
			ExpectedContent:       []string{`"message":"An amount is too large to total."`, `"spec":{"code":"amount_too_large"`},
			NotExpectedContent:    []string{leak},
			TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
			DisableTestAppCleanup: true,
		}
		sc.Test(t)
	}
}

// A database failure answers 500 with a fixed message and no SQL text.
func TestRunEndpointInternalErrorLeaksNoSQL(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	ann := z.token(t, "ann")
	// Serve checks every source before it binds the route, so the table goes
	// after that check: the source is valid, the statement then fails.
	z.app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if _, err := e.App.DB().NewQuery("DROP TABLE IF EXISTS zoo_feedings").Execute(); err != nil {
			return err
		}
		return e.Next()
	})
	sc := tests.ApiScenario{
		Name: "an internal error", Method: http.MethodPost, URL: "/api/reports/run",
		Body: strings.NewReader(runBodyOf(feedingsByDay)), Headers: map[string]string{"Authorization": ann},
		ExpectedStatus:        http.StatusInternalServerError,
		ExpectedContent:       []string{`"message":"The report could not run."`},
		NotExpectedContent:    []string{"SELECT", "no such table", "zoo_feedings", "sqlite"},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

func TestServeRefusesABrokenSourceBeforeAnyRun(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.ghosts", Collection: "zoo_ghosts", Label: "Ghosts", Fields: []Field{{Key: "name", Label: "Name", Type: Text}}})
	Register(z.app)
	z.post(t, "a refused source", z.token(t, "ann"), runBodyOf(`{"version":1,"source":"zoo.ghosts","columns":[{"id":"a","path":[{"field":"name"}]}]}`),
		http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	if _, bad := Refused(z.app)["zoo.ghosts"]; !bad {
		t.Fatal("serve did not refuse zoo.ghosts")
	}
}

// A report reads across every package's collections, and an OAuth token's
// scopes cover only some: the route stays session-only.
func TestRunEndpointIsNotOpenToOAuthTokens(t *testing.T) {
	if got := oauth.ScopeForRoute(http.MethodPost, "/api/reports/run"); len(got) != 0 {
		t.Fatalf("POST /api/reports/run resolved to scopes %v, want default-deny", got)
	}
}

// An OAuth access token, even one holding every scope, meets the real grant
// middleware, which refuses the run route as a route no scope covers.
func TestRunEndpointRefusesAnOAuthTokenThroughTheMiddleware(t *testing.T) {
	z := newZoo(t)
	oauth.Register(z.app)
	Register(z.app)
	client, err := oauth.FindClientByClientID(z.app, "tinycld-cli")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := oauth.NewGrant(z.app, z.id("ann"), client.Id, oauth.AllScopes(), "active")
	if err != nil {
		t.Fatal(err)
	}
	token, err := oauth.MintAccessToken(z.app, z.recs["ann"], grant, oauth.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	z.post(t, "an OAuth token", "Bearer "+token, runBodyOf(feedingsByDay), http.StatusForbidden, `not available to API tokens`)
	// The same reader's session token runs the report.
	z.post(t, "a session token", z.token(t, "ann"), runBodyOf(feedingsByDay), http.StatusOK, `"kind":"total"`)
}

// levelsHandler keeps the level of every record logged through it.
type levelsHandler struct{ levels *[]slog.Level }

func (h levelsHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h levelsHandler) Handle(_ context.Context, r slog.Record) error {
	*h.levels = append(*h.levels, r.Level)
	return nil
}
func (h levelsHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h levelsHandler) WithGroup(string) slog.Handler      { return h }

// A client that went away is not a server error: nothing is logged above
// debug (warn and up reach Sentry) and the answer is not a 500.
func TestARunTheClientLeftIsNotAServerError(t *testing.T) {
	var levels []slog.Level
	saved := log
	log = slog.New(levelsHandler{&levels})
	t.Cleanup(func() { log = saved })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, err := range map[string]error{
		"canceled":           context.Canceled,
		"driver interrupted": errors.New("interrupted (9)"),
	} {
		t.Run(name, func(t *testing.T) {
			levels = nil
			re := &core.RequestEvent{}
			re.Request = httptest.NewRequest(http.MethodPost, "/api/reports/run", nil).WithContext(ctx)
			re.Response = httptest.NewRecorder()
			var apiErr *router.ApiError
			if got := runError(re, err); !errors.As(got, &apiErr) || apiErr.Status >= 500 {
				t.Fatalf("runError = %v, want a 4xx", got)
			}
			if slices.ContainsFunc(levels, func(l slog.Level) bool { return l > slog.LevelDebug }) {
				t.Fatalf("logged at %v", levels)
			}
		})
	}
}

func TestRuleHeadersReadLikePocketBase(t *testing.T) {
	got := ruleHeaders(http.Header{"X-Zoo-Gate": {"open", "shut"}, "Accept": {"application/json"}})
	want := map[string]string{"x_zoo_gate": "open", "accept": "application/json"}
	if len(got) != len(want) || got["x_zoo_gate"] != "open" || got["accept"] != "application/json" {
		t.Fatalf("ruleHeaders = %v", got)
	}
}
