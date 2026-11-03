package forms

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

func TestHostsRouteListsTheInstalledHosts(t *testing.T) {
	e := newTestEnv(t)
	_, member := formsUser(t, e.app, "hosts-member@test.local", "member")
	_, guest := formsUser(t, e.app, "hosts-guest@test.local", "guest")
	factory := func(testing.TB) *tests.TestApp { return e.app }
	for _, sc := range []tests.ApiScenario{
		{Name: "member reads", Method: http.MethodGet, URL: "/api/forms/hosts", Headers: map[string]string{"Authorization": member}, ExpectedStatus: 200,
			ExpectedContent: []string{`"key":"gizmos:widget"`, `"label":"Widget"`, `"builtins":[`, `"kinds":[`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "guest refused", Method: http.MethodGet, URL: "/api/forms/hosts", Headers: map[string]string{"Authorization": guest}, ExpectedStatus: 403, ExpectedContent: []string{`"status":403`}, TestAppFactory: factory, DisableTestAppCleanup: true},
		{Name: "anonymous refused", Method: http.MethodGet, URL: "/api/forms/hosts", ExpectedStatus: 401, ExpectedContent: []string{`"status":401`}, TestAppFactory: factory, DisableTestAppCleanup: true},
	} {
		sc.Test(t)
	}
}

// A host that allows every kind lists every custom kind, so the caller
// never has to know that an empty list means "all".
func TestHostViewExpandsAnEmptyKindList(t *testing.T) {
	v := hostViewOf(Host{Key: "gizmos:any", Label: "Any"})
	if len(v.Kinds) != len(CustomKinds()) {
		t.Fatalf("kinds = %v, want every custom kind", v.Kinds)
	}
}
