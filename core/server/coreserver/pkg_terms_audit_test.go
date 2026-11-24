package coreserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/pkgterms"
)

// A decision is in the audit log: the package, the version and the decision
// in the label, the owner as the actor.
func TestATermsDecisionIsAudited(t *testing.T) {
	app := bootedPocketBase(t)
	RegisterAuditHooks(app)
	pkgterms.Register(app)
	registry, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	pkg := core.NewRecord(registry)
	pkg.Set("name", "Gizmos")
	pkg.Set("slug", "gizmos")
	pkg.Set("version", "1.0.0")
	pkg.Set("status", "bundled")
	pkg.Set("terms_version", "2026-01")
	if err := app.Save(pkg); err != nil {
		t.Fatal(err)
	}
	owner := guestRLSUser(t, app, "terms-owner@test.local", "owner")

	// The create request of the record API, as its handler triggers it: the
	// audit hook and the pkgterms hooks run, then the record is saved.
	decisions, err := app.FindCollectionByNameOrId(pkgterms.DecisionsCollection)
	if err != nil {
		t.Fatal(err)
	}
	decision := core.NewRecord(decisions)
	decision.Set("pkg", "gizmos")
	decision.Set("terms_version", "2026-01")
	decision.Set("decision", pkgterms.DecisionAccepted)
	request := &core.RequestEvent{App: app, Auth: owner}
	request.Request = httptest.NewRequest(http.MethodPost, "/api/collections/pkg_terms_decisions/records", nil)
	request.Response = httptest.NewRecorder()
	event := &core.RecordRequestEvent{RequestEvent: request, Collection: decisions, Record: decision}
	err = app.OnRecordCreateRequest().Trigger(event, func(e *core.RecordRequestEvent) error {
		return e.App.Save(e.Record)
	})
	if err != nil {
		t.Fatal(err)
	}
	if pkgterms.Pending(registryRow(t, app, "gizmos")) {
		t.Fatal("the decision did not accept the terms")
	}

	var logs []*core.Record
	waitFor(t, func() bool {
		logs, _ = app.FindRecordsByFilter("audit_logs", "resource_type = 'pkg_terms_decisions'", "", 0, 0)
		return len(logs) > 0
	})
	if len(logs) != 1 || logs[0].GetString("action") != "created" || logs[0].GetString("resource_label") != "gizmos:2026-01:accepted" {
		t.Fatalf("the audit row: %d rows, action %q, label %q", len(logs), logs[0].GetString("action"), logs[0].GetString("resource_label"))
	}
	if actor := logs[0].GetString("actor"); actor != owner.Id {
		t.Fatalf("the actor is %q, want the owner %q", actor, owner.Id)
	}
}
