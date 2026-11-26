package reports

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/oauth"
)

// tokenZoo is the zoo behind the real grant middleware, with the scopes of
// registerZooScopes.
func tokenZoo(t *testing.T) *zoo {
	t.Helper()
	z := newZoo(t)
	oauth.Register(z.app)
	Register(z.app)
	registerZooScopes(t)
	return z
}

// oauthToken is an access token of user with exactly these scopes, as the
// device flow of the CLI mints one.
func (z *zoo) oauthToken(t *testing.T, user string, scopes ...string) string {
	t.Helper()
	client, err := oauth.FindClientByClientID(z.app, "tinycld-cli")
	if err != nil {
		t.Fatal(err)
	}
	grant, err := oauth.NewGrant(z.app, z.id(user), client.Id, scopes, "active")
	if err != nil {
		t.Fatal(err)
	}
	token, err := oauth.MintAccessToken(z.app, z.recs[user], grant, oauth.AccessTokenTTL)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + token
}

func (z *zoo) scenario(t *testing.T, name, url, token, body string, status int, want, notWant []string) {
	t.Helper()
	sc := tests.ApiScenario{
		Name: name, Method: http.MethodPost, URL: url,
		Body: strings.NewReader(body), Headers: map[string]string{"Authorization": token},
		ExpectedStatus: status, ExpectedContent: want, NotExpectedContent: notWant,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

// runByID is the run body of a saved report, as a token sends it.
func runByID(rec *core.Record, params string) string {
	return fmt.Sprintf(`{"reportId":%q,"params":%s}`, rec.Id, params)
}

// The reports scope opens the run route, and the token's other scopes decide
// what a run may read: every collection a statement of the run selects rows
// from. A collection that only a list rule reads needs no scope, as for a
// request to the records API.
func TestRunHoldsAnOAuthTokenToItsScopes(t *testing.T) {
	z := tokenZoo(t)
	const run = "/api/reports/run"
	body := runByID(z.saved(t, "by-day", "ann", "Feedings By Day", feedingsByDay, false), `{}`)
	all := z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals")

	z.scenario(t, "every collection is in the scopes", run, all, body, http.StatusOK,
		[]string{`"kind":"total"`, `"cost":4500`}, nil)
	z.scenario(t, "no reports scope", run, z.oauthToken(t, "ann", "zoo:read", "zoo:animals"), body, http.StatusForbidden,
		[]string{`Requires the \"reports\" scope`}, []string{`"rows"`})
	// The species column reads zoo_animals, which zoo:read does not open.
	z.scenario(t, "a column's collection is outside the scopes", run, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read"), body, http.StatusForbidden,
		[]string{`The access token's scopes do not cover what this report reads: zoo_animals.`}, []string{`"rows"`, `"cost"`})
	// zoo_keepers, which only the feedings' list rule reads, is not named.
	z.scenario(t, "the reports scope alone reads nothing", run, z.oauthToken(t, "ann", oauth.ScopeReports), body, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_animals, zoo_feedings.`}, []string{`"rows"`, `zoo_keepers`})
	// The scopes are a ceiling, never a grant: the user's own list rules still
	// filter the rows. bob keeps other animals than ann and reads f5, not f1.
	bobs := runByID(z.saved(t, "bobs", "bob", "Feedings By Day", feedingsByDay, false), `{}`)
	z.scenario(t, "the user's rules still apply", run, z.oauthToken(t, "bob", oauth.ScopeReports, "zoo:read", "zoo:animals"), bobs, http.StatusOK,
		[]string{`"note":"f5"`}, []string{`"note":"f1"`, `"cost":4500`})
	// A session has no scope ceiling.
	z.scenario(t, "a session", run, z.token(t, "ann"), body, http.StatusOK, []string{`"kind":"total"`}, nil)
}

// An error of a run that read rows says something of them: a token whose
// scopes do not cover the report gets the scope refusal, not the error.
func TestRunRefusesATokenBeforeItSaysAnAmountIsTooLarge(t *testing.T) {
	z := tokenZoo(t)
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 9223372036854775807 WHERE note IN ('f1', 'f2')").Execute(); err != nil {
		t.Fatal(err)
	}
	body := runByID(z.saved(t, "by-day", "ann", "Feedings By Day", feedingsByDay, false), `{}`)
	z.scenario(t, "outside the scopes", "/api/reports/run", z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:animals"), body, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_feedings.`}, []string{`too large`})
	z.scenario(t, "in the scopes", "/api/reports/run", z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals"), body, http.StatusUnprocessableEntity,
		[]string{`An amount is too large to total.`}, nil)
}

// A token runs a saved report only. A draft spec is the builder's preview.
func TestRunRefusesADraftSpecToAnOAuthToken(t *testing.T) {
	z := tokenZoo(t)
	draft := runBodyOf(feedingsFlat)
	z.scenario(t, "a token", "/api/reports/run", z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals"), draft, http.StatusForbidden,
		[]string{`An access token runs a saved report only: send reportId, not spec.`}, []string{`"rows"`})
	// The refusal comes before the spec is read: it names nothing of a source.
	z.scenario(t, "a token, a spec with an unknown source", "/api/reports/run", z.oauthToken(t, "ann", oauth.ScopeReports),
		`{"spec":{"version":4,"source":"zoo.nothing","columns":[]}}`, http.StatusForbidden,
		[]string{`An access token runs a saved report only`}, []string{`zoo.nothing`})
	z.scenario(t, "a session", "/api/reports/run", z.token(t, "ann"), draft, http.StatusOK, []string{`"kind":"total"`}, nil)
}

// A choice param reads its labels from a source that no statement of the run
// may read: its collections count too, and so does a collection that a label
// path reaches over a link.
func TestRunHoldsAParamsSourceToTheScopes(t *testing.T) {
	z := tokenZoo(t)
	const run = "/api/reports/run"
	body := runByID(z.saved(t, "at-a-site", "ann", "Feedings At A Site", feedingsAtASite, false), `{}`)
	z.scenario(t, "the param's source is outside the scopes", run,
		z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals"), body, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_sites.`}, []string{`"rows"`})
	z.scenario(t, "the param's source is in the scopes", run,
		z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals", "zoo:sites"), body, http.StatusOK,
		[]string{`"kind":"total"`}, nil)

	// The header line of the pick is an animal's name, which zoo:read does not
	// open; no column of the report reads the animals.
	picked := runByID(z.saved(t, "of-one", "ann", "Feedings Of One", feedingsOfOne, false), fmt.Sprintf(`{"one":%q}`, z.id("f1")))
	z.scenario(t, "a pick's label is outside the scopes", run, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read"), picked, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_animals.`}, []string{`Leo`, `"rows"`})
	z.scenario(t, "a pick's label is in the scopes", run, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals"), picked, http.StatusOK,
		[]string{`Feeding: Leo`}, nil)
}

// A token runs a saved report it can open, by id, and reads the list of saved
// reports through the records API.
func TestATokenListsAndRunsASavedReport(t *testing.T) {
	z := tokenZoo(t)
	rep := z.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	all := z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals")
	z.scenario(t, "run by id", "/api/reports/run", all, fmt.Sprintf(`{"reportId":%q}`, rep.Id), http.StatusOK,
		[]string{`"kind":"total"`}, nil)
	z.scenario(t, "another user's report", "/api/reports/run", z.oauthToken(t, "bob", oauth.ScopeReports, "zoo:read", "zoo:animals"),
		fmt.Sprintf(`{"reportId":%q}`, rep.Id), http.StatusForbidden, []string{`You cannot view this report.`}, nil)

	list := tests.ApiScenario{
		Name: "the list of saved reports", Method: http.MethodGet, URL: "/api/collections/reports/records",
		Headers:        map[string]string{"Authorization": all},
		ExpectedStatus: http.StatusOK, ExpectedContent: []string{`"title":"Feeding Costs"`, `"totalItems":1`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	list.Test(t)
}

// The export route holds a token to its scopes as the run route does, and a
// refused export writes no audit row.
func TestExportHoldsAnOAuthTokenToItsScopes(t *testing.T) {
	z := tokenZoo(t)
	rep := z.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	body := csvExportBody(rep.Id)

	z.scenario(t, "a collection is outside the scopes", exportURL, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:animals"), body, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_feedings.`}, []string{"f1"})
	z.scenario(t, "no reports scope", exportURL, z.oauthToken(t, "ann", "zoo:read", "zoo:animals"), body, http.StatusForbidden,
		[]string{`Requires the \"reports\" scope`}, []string{"f1"})
	if n := len(exportAuditRows(t, z.app)); n != 0 {
		t.Fatalf("refused exports wrote %d audit rows", n)
	}
	z.scenario(t, "every collection is in the scopes", exportURL, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals"), body, http.StatusOK,
		[]string{"f1", "Total"}, nil)
	if n := len(exportAuditRows(t, z.app)); n != 1 {
		t.Fatalf("one download wrote %d audit rows, want 1", n)
	}
	z.scenario(t, "a session", exportURL, z.token(t, "ann"), body, http.StatusOK, []string{"f1"}, nil)
}

// A file with a header names the picks of a choice param. Those labels are
// read after the run, by statements of their own, and count as the run's do.
func TestExportHoldsThePickLabelsToTheScopes(t *testing.T) {
	z := tokenZoo(t)
	// No header line in the spec: only the file's own header reads the label.
	spec := strings.Replace(feedingsOfOne, `"header":"Feeding: {label}",`, "", 1)
	rep := z.saved(t, "of-one", "ann", "Feedings Of One", spec, false)
	body := fmt.Sprintf(`{"reportId":%q,"params":{"one":%q},"options":{"today":"2026-11-16","timeZone":"UTC"},"format":"xlsx","hour12":false}`, rep.Id, z.id("f1"))

	z.scenario(t, "the label's collection is outside the scopes", exportURL, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read"), body, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_animals.`}, nil)
	if n := len(exportAuditRows(t, z.app)); n != 0 {
		t.Fatalf("a refused export wrote %d audit rows", n)
	}
	z.scenario(t, "the label's collection is in the scopes", exportURL, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read", "zoo:animals"), body, http.StatusOK,
		[]string{"[Content_Types].xml"}, nil)
}

// "Too many rows" says something of the rows: a token whose scopes do not
// cover the report gets the scope refusal, not the count.
func TestExportRefusesATokenBeforeItSaysTooManyRows(t *testing.T) {
	z := tokenZoo(t)
	keepExportLimits(t)
	exportLimits.sheetRows = 3
	rep := z.saved(t, "costs", "ann", "Feeding Costs", feedingsFlat, false)
	body := csvExportBody(rep.Id)

	z.scenario(t, "outside the scopes", exportURL, z.oauthToken(t, "ann", oauth.ScopeReports), body, http.StatusForbidden,
		[]string{`do not cover what this report reads: zoo_feedings.`}, nil)
	z.scenario(t, "in the scopes", exportURL, z.oauthToken(t, "ann", oauth.ScopeReports, "zoo:read"), body, http.StatusUnprocessableEntity,
		[]string{`"status":422`}, []string{"f1"})
}

// The builder's routes and the email routes have no scope: a token with every
// scope is refused by default-deny.
func TestTheOtherReportRoutesStayClosedToAToken(t *testing.T) {
	z := tokenZoo(t)
	all := z.oauthToken(t, "ann", oauth.AllScopes()...)
	for _, url := range []string{"/api/reports/param-options", "/api/reports/email"} {
		z.scenario(t, url, url, all, `{}`, http.StatusForbidden, []string{`not available to API tokens`}, nil)
	}
}
