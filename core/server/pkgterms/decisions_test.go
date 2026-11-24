package pkgterms_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/pkgterms"
)

const decisionsURL = "/api/collections/pkg_terms_decisions/records"

type decisionEnv struct {
	app    *tests.TestApp
	pkg    *core.Record
	owner  *core.Record
	admin  *core.Record
	ownerT string
	adminT string
}

// newDecisionEnv is the app with the fictional package acme, pending on
// version 2 of its terms, and pkgterms registered.
func newDecisionEnv(t *testing.T) *decisionEnv {
	t.Helper()
	app := newApp(t)
	pkgterms.Register(app)
	env := &decisionEnv{app: app, pkg: addPackage(t, app, "acme", "bundled", "2", "")}
	env.owner = addUser(t, app, "owner@terms.test", "owner")
	env.admin = addUser(t, app, "admin@terms.test", "admin")
	env.ownerT, env.adminT = tokenOf(t, env.owner), tokenOf(t, env.admin)
	return env
}

func decisionBody(pkg, version, decision string) string {
	return `{"pkg":"` + pkg + `","terms_version":"` + version + `","decision":"` + decision + `"}`
}

func (e *decisionEnv) decisions(t *testing.T) []*core.Record {
	t.Helper()
	rows, err := e.app.FindRecordsByFilter(pkgterms.DecisionsCollection, "id != ''", "created", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestOwnerAcceptsTheTerms(t *testing.T) {
	e := newDecisionEnv(t)
	// The body names another user: the server records who decided.
	body := `{"pkg":"acme","terms_version":"2","decision":"accepted","user":"` + e.admin.Id + `"}`
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, body, http.StatusOK, `"decision":"accepted"`, `"user":"`+e.owner.Id+`"`)

	pkg := reload(t, e.app, e.pkg)
	if pkg.GetString(pkgterms.FieldAcceptedVersion) != "2" || pkgterms.Pending(pkg) {
		t.Fatalf("after the acceptance: accepted %q, pending %v", pkg.GetString(pkgterms.FieldAcceptedVersion), pkgterms.Pending(pkg))
	}
	rows := e.decisions(t)
	if len(rows) != 1 || rows[0].GetString("pkg") != "acme" || rows[0].GetString("terms_version") != "2" ||
		rows[0].GetString("user") != e.owner.Id || rows[0].GetDateTime("created").IsZero() {
		t.Fatalf("the record of the acceptance: %v", rows)
	}
}

func TestOnlyTheOwnerDecides(t *testing.T) {
	e := newDecisionEnv(t)
	member := tokenOf(t, addUser(t, e.app, "member@terms.test", "member"))
	for name, token := range map[string]string{"an admin": e.adminT, "a member": member, "signed out": ""} {
		t.Run(name, func(t *testing.T) {
			call(t, e.app, http.MethodPost, decisionsURL, token, decisionBody("acme", "2", "accepted"), http.StatusBadRequest, `"status":400`)
		})
	}
	if rows := e.decisions(t); len(rows) != 0 {
		t.Fatalf("a decision of somebody who is not the owner was stored: %v", rows)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("the package is no longer pending")
	}
}

// triggerDecisionCreate runs the create request of the record API for a
// decision as its handler does, but past the create rule: the hook is a second
// check behind that rule, and only a direct trigger reaches it.
func triggerDecisionCreate(t *testing.T, e *decisionEnv, auth *core.Record, user string) (*core.Record, error) {
	t.Helper()
	col, err := e.app.FindCollectionByNameOrId(pkgterms.DecisionsCollection)
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(col)
	row.Set("pkg", "acme")
	row.Set("terms_version", "2")
	row.Set("decision", pkgterms.DecisionAccepted)
	row.Set("user", user)
	request := &core.RequestEvent{App: e.app, Auth: auth}
	request.Request = httptest.NewRequest(http.MethodPost, decisionsURL, nil)
	request.Response = httptest.NewRecorder()
	event := &core.RecordRequestEvent{RequestEvent: request, Collection: col, Record: row}
	err = e.app.OnRecordCreateRequest().Trigger(event, func(ev *core.RecordRequestEvent) error {
		return ev.App.Save(ev.Record)
	})
	return row, err
}

func TestTheCreateHookRefusesAMemberBehindTheCreateRule(t *testing.T) {
	e := newDecisionEnv(t)
	member := addUser(t, e.app, "member2@terms.test", "member")
	_, err := triggerDecisionCreate(t, e, member, member.Id)
	var apiErr *router.ApiError
	if !errors.As(err, &apiErr) || apiErr.Status != http.StatusForbidden || apiErr.Message != "Only the owner decides on the terms of a package." {
		t.Fatalf("a member's decision: error %v", err)
	}
	if rows := e.decisions(t); len(rows) != 0 {
		t.Fatalf("a member's decision was stored: %v", rows)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a member's decision accepted the terms")
	}
}

func TestTheCreateHookRecordsTheOwnerAsTheDecider(t *testing.T) {
	e := newDecisionEnv(t)
	row, err := triggerDecisionCreate(t, e, e.owner, e.admin.Id)
	if err != nil {
		t.Fatal(err)
	}
	if got := row.GetString("user"); got != e.owner.Id {
		t.Fatalf("the decider is %q, want the owner %q", got, e.owner.Id)
	}
	if rows := e.decisions(t); len(rows) != 1 || rows[0].GetString("user") != e.owner.Id {
		t.Fatalf("the stored decision: %v", rows)
	}
}

func TestADecisionIsCheckedAgainstThePackage(t *testing.T) {
	e := newDecisionEnv(t)
	addPackage(t, e.app, "plain", "bundled", "", "")
	cases := []struct{ name, body, code string }{
		{"a version that is not current", decisionBody("acme", "1", "accepted"), `"terms_version":{"code":"not_current"`},
		{"a package that does not exist", decisionBody("nothing", "2", "accepted"), `"pkg":{"code":"unknown_package"`},
		{"a package with no terms", decisionBody("plain", "2", "accepted"), `"pkg":{"code":"no_terms"`},
		{"a decision that is neither", decisionBody("acme", "2", "maybe"), `"decision":{"code":"validation_`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, c.body, http.StatusBadRequest, c.code)
		})
	}
	if rows := e.decisions(t); len(rows) != 0 {
		t.Fatalf("a refused decision was stored: %v", rows)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a refused decision accepted the terms")
	}
}

func TestADeclineKeepsThePackagePendingAndCanBeFollowedByAnAcceptance(t *testing.T) {
	e := newDecisionEnv(t)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "declined"), http.StatusOK, `"decision":"declined"`)
	pkg := reload(t, e.app, e.pkg)
	if !pkgterms.Pending(pkg) || pkg.GetString(pkgterms.FieldAcceptedVersion) != "" {
		t.Fatal("a decline accepted the terms")
	}

	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "accepted"), http.StatusOK, `"decision":"accepted"`)
	if pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("the acceptance after a decline did not accept")
	}
	// An acceptance is final for its version: no decline, no second acceptance.
	for _, decision := range []string{"declined", "accepted"} {
		call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", decision), http.StatusBadRequest, `"decision":{"code":"already_accepted"`)
	}
	if rows := e.decisions(t); len(rows) != 2 {
		t.Fatalf("%d decision rows, want the decline and the acceptance", len(rows))
	}
}

// The owner declines as often as the terms are shown, and accepts a version
// again when the package left it and came back to it (an older release put
// back).
func TestADeclineAndAReturnedVersionCanBeDecidedAgain(t *testing.T) {
	e := newDecisionEnv(t)
	for range 2 {
		call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "declined"), http.StatusOK, `"decision":"declined"`)
	}
	for _, version := range []string{"2", "3", "2"} {
		pkg := reload(t, e.app, e.pkg)
		pkg.Set(pkgterms.FieldTermsVersion, version)
		if err := e.app.Save(pkg); err != nil {
			t.Fatal(err)
		}
		call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", version, "accepted"), http.StatusOK, `"terms_version":"`+version+`"`)
		if pkgterms.Pending(reload(t, e.app, e.pkg)) {
			t.Fatalf("version %s is not accepted", version)
		}
	}
	if rows := e.decisions(t); len(rows) != 5 {
		t.Fatalf("%d decision rows, want two declines and three acceptances", len(rows))
	}
}

func TestANewVersionNeedsANewAcceptance(t *testing.T) {
	e := newDecisionEnv(t)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "accepted"), http.StatusOK, `"terms_version":"2"`)

	// The boot sync writes the version of the new package release.
	pkg := reload(t, e.app, e.pkg)
	pkg.Set(pkgterms.FieldTermsVersion, "3")
	if err := e.app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a new terms version did not make the package pending")
	}
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "accepted"), http.StatusBadRequest, `"not_current"`)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "3", "accepted"), http.StatusOK, `"terms_version":"3"`)
	pkg = reload(t, e.app, e.pkg)
	if pkgterms.Pending(pkg) || pkg.GetString(pkgterms.FieldAcceptedVersion) != "3" {
		t.Fatal("the acceptance of the new version did not accept")
	}
}

// A decision stored by the server with no request (a seed) follows the same
// rule: an acceptance sets the accepted version.
func TestASeededAcceptanceAccepts(t *testing.T) {
	e := newDecisionEnv(t)
	col, err := e.app.FindCollectionByNameOrId(pkgterms.DecisionsCollection)
	if err != nil {
		t.Fatal(err)
	}
	row := core.NewRecord(col)
	row.Set("pkg", "acme")
	row.Set("terms_version", "2")
	row.Set("decision", pkgterms.DecisionAccepted)
	if err := e.app.Save(row); err != nil {
		t.Fatal(err)
	}
	if pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a seeded acceptance did not accept")
	}
}

func TestNobodyChangesOrRemovesADecision(t *testing.T) {
	e := newDecisionEnv(t)
	call(t, e.app, http.MethodPost, decisionsURL, e.ownerT, decisionBody("acme", "2", "declined"), http.StatusOK, `"decision":"declined"`)
	id := e.decisions(t)[0].Id
	call(t, e.app, http.MethodPatch, decisionsURL+"/"+id, e.ownerT, `{"decision":"accepted"}`, http.StatusForbidden, `"status":403`)
	call(t, e.app, http.MethodDelete, decisionsURL+"/"+id, e.ownerT, "", http.StatusForbidden, `"status":403`)
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("an edit of a decision accepted the terms")
	}
}

func TestTheOwnerCannotWriteTheTermsFieldsOfTheRegistry(t *testing.T) {
	e := newDecisionEnv(t)
	url := "/api/collections/pkg_registry/records/" + e.pkg.Id
	const refusal = "The terms of a package are accepted on the terms screen."
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"terms_accepted_version":"2"}`, http.StatusForbidden, refusal)
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"terms_version":""}`, http.StatusForbidden, refusal)
	call(t, e.app, http.MethodPost, "/api/collections/pkg_registry/records", e.ownerT,
		`{"name":"Other","slug":"other","status":"available","terms_accepted_version":"1"}`, http.StatusForbidden, refusal)
	call(t, e.app, http.MethodPost, "/api/collections/pkg_registry/records", e.ownerT,
		`{"name":"Another","slug":"another","status":"available","terms_version":"1"}`, http.StatusForbidden, refusal)
	if !pkgterms.Pending(reload(t, e.app, e.pkg)) {
		t.Fatal("a registry write accepted the terms")
	}

	// The owner still turns the package off and on, and a superuser repairs.
	call(t, e.app, http.MethodPatch, url, e.ownerT, `{"status":"disabled"}`, http.StatusOK, `"status":"disabled"`)
	call(t, e.app, http.MethodPatch, url, superuserToken(t, e.app), `{"terms_accepted_version":"2"}`, http.StatusOK, `"terms_accepted_version":"2"`)
}
