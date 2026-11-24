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
	decide := func(decision string) {
		t.Helper()
		row := core.NewRecord(decisions)
		row.Set("pkg", "gizmos")
		row.Set("terms_version", "2026-01")
		row.Set("decision", decision)
		request := &core.RequestEvent{App: app, Auth: owner}
		request.Request = httptest.NewRequest(http.MethodPost, "/api/collections/pkg_terms_decisions/records", nil)
		request.Response = httptest.NewRecorder()
		event := &core.RecordRequestEvent{RequestEvent: request, Collection: decisions, Record: row}
		err := app.OnRecordCreateRequest().Trigger(event, func(e *core.RecordRequestEvent) error {
			return e.App.Save(e.Record)
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	decide(pkgterms.DecisionDeclined)
	if !pkgterms.Pending(registryRow(t, app, "gizmos")) {
		t.Fatal("a decline accepted the terms")
	}
	decide(pkgterms.DecisionAccepted)
	if pkgterms.Pending(registryRow(t, app, "gizmos")) {
		t.Fatal("the decision did not accept the terms")
	}

	var logs []*core.Record
	waitFor(t, func() bool {
		logs, _ = app.FindRecordsByFilter("audit_logs", "resource_type = 'pkg_terms_decisions'", "created", 0, 0)
		return len(logs) >= 2
	})
	if len(logs) != 2 {
		t.Fatalf("%d audit rows, want 2", len(logs))
	}
	labels := map[string]bool{}
	for _, log := range logs {
		labels[log.GetString("resource_label")] = true
		if log.GetString("action") != "created" {
			t.Fatalf("the audit action is %q, want created", log.GetString("action"))
		}
		if actor := log.GetString("actor"); actor != owner.Id {
			t.Fatalf("the actor is %q, want the owner %q", actor, owner.Id)
		}
	}
	if !labels["gizmos:2026-01:declined"] || !labels["gizmos:2026-01:accepted"] {
		t.Fatalf("the audit labels: %v", labels)
	}
}
