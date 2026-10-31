package reports

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func feedCosts(revision int, title string) Shipped {
	return Shipped{Key: "zoo.feed_costs", Title: title, Description: "Every feeding's cost", Spec: json.RawMessage(savedSpec), Revision: revision}
}

func shippedRow(t *testing.T, s *savedZoo, key string) *core.Record {
	t.Helper()
	rec, err := s.app.FindFirstRecordByFilter(reportsCollection, "shipped_key = {:k}", map[string]any{"k": key})
	if err != nil {
		return nil
	}
	return rec
}

func TestRegisterShippedRefusesAMalformedReport(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	for name, s := range map[string]Shipped{
		"a key without a package": {Key: "costs", Title: "x", Spec: json.RawMessage(`{}`), Revision: 1},
		"a key past the column":   {Key: "zoo." + strings.Repeat("k", 117), Title: "x", Spec: json.RawMessage(`{}`), Revision: 1},
		"no title":                {Key: "zoo.costs", Title: " ", Spec: json.RawMessage(`{}`), Revision: 1},
		"revision 0":              {Key: "zoo.costs", Title: "x", Spec: json.RawMessage(`{}`), Revision: 0},
		"a spec that is not JSON": {Key: "zoo.costs", Title: "x", Spec: json.RawMessage(`{`), Revision: 1},
		"a description too long":  {Key: "zoo.costs", Title: "x", Description: strings.Repeat("d", 2001), Spec: json.RawMessage(`{}`), Revision: 1},
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("RegisterShipped did not panic")
				}
			}()
			RegisterShipped(s)
		})
	}
}

// A shipped report waits for an admin, then belongs to the first one,
// published; every member sees it.
func TestAShippedReportWaitsForTheFirstAdmin(t *testing.T) {
	s := newSavedZoo(t)
	// The fixture's admin ad would own it: start from a deployment with no admin.
	s.recs["ad"].Set("role", "member")
	if err := s.app.Save(s.recs["ad"]); err != nil {
		t.Fatal(err)
	}
	RegisterShipped(feedCosts(1, "Feed costs"))
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	if shippedRow(t, s, "zoo.feed_costs") != nil {
		t.Fatal("a shipped report was made with no admin to own it")
	}
	// The setup wizard's first owner: its save runs shipForAdmin.
	owner := s.person(t, "own", "owner")
	row := shippedRow(t, s, "zoo.feed_costs")
	if row == nil {
		t.Fatal("the first admin's save did not ship the report")
	}
	if row.GetString("owner") != owner.Id || !row.GetBool("published") || row.GetString("published_by") != owner.Id ||
		row.GetString("source") != "zoo.feedings" || row.GetInt("shipped_revision") != 1 || row.GetString("title") != "Feed costs" {
		t.Fatalf("shipped row = %v", row.PublicExport())
	}
	if got := s.listTitles(t, "ot", reportsURL); !strings.Contains(strings.Join(got, ","), "Feed costs") {
		t.Fatalf("a member lists %v", got)
	}
}

// The oldest active owner owns shipped reports; an admin only when there
// is no owner.
func TestTheFirstAdminIsTheOldestOwner(t *testing.T) {
	s := newSavedZoo(t) // ad, an admin, is older than the owner below
	owner := s.person(t, "own", "owner")
	got, err := firstAdmin(s.app, "")
	if err != nil || got == nil || got.Id != owner.Id {
		t.Fatalf("firstAdmin = %v, %v; want the owner", got, err)
	}
	if got, _ := firstAdmin(s.app, owner.Id); got == nil || got.Id != s.id("ad") {
		t.Fatalf("firstAdmin without the owner = %v; want ad", got)
	}
}

// A newer revision replaces the spec, title and description and keeps the
// owner and what an admin did with publishing; the same revision changes
// nothing.
func TestANewerRevisionReplacesTheShippedSpec(t *testing.T) {
	s := newSavedZoo(t)
	RegisterShipped(feedCosts(1, "Feed costs"))
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row := shippedRow(t, s, "zoo.feed_costs")
	if row == nil || row.GetString("owner") != s.id("ad") {
		t.Fatalf("not shipped to ad: %v", row)
	}
	s.expect(t, "ad", http.MethodPatch, reportURL(row.Id), `{"published":false}`, http.StatusOK)
	updated := shippedRow(t, s, "zoo.feed_costs").GetString("updated")
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	if shippedRow(t, s, "zoo.feed_costs").GetString("updated") != updated {
		t.Fatal("the same revision rewrote the report")
	}
	RegisterShipped(Shipped{Key: "zoo.feed_costs", Title: "Feeding costs", Spec: json.RawMessage(
		`{"version":1,"source":"zoo.feedings","columns":[{"id":"grams","path":[{"field":"grams"}],"aggregate":"sum"}]}`), Revision: 2})
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row = shippedRow(t, s, "zoo.feed_costs")
	if row.GetString("title") != "Feeding costs" || row.GetInt("shipped_revision") != 2 || !strings.Contains(row.GetString("spec"), `"grams"`) ||
		row.GetBool("published") || row.GetString("owner") != s.id("ad") {
		t.Fatalf("after revision 2: %v", row.PublicExport())
	}
}

// A shipped spec that does not validate is skipped with its key named, and
// the others still ship.
func TestABadShippedSpecDoesNotStopTheOthers(t *testing.T) {
	s := newSavedZoo(t)
	RegisterShipped(Shipped{Key: "zoo.broken", Title: "Broken", Spec: json.RawMessage(goneSpec), Revision: 1})
	RegisterShipped(feedCosts(1, "Feed costs"))
	err := ReconcileShipped(s.app)
	if err == nil || !strings.Contains(err.Error(), `"zoo.broken"`) {
		t.Fatalf("err = %v", err)
	}
	if shippedRow(t, s, "zoo.broken") != nil || shippedRow(t, s, "zoo.feed_costs") == nil {
		t.Fatal("the bad spec stopped the good one, or shipped itself")
	}
}

// A shipped report cannot be deleted while its package ships it (the server
// would make it again); once the package is gone it can.
func TestAShippedReportIsUnpublishedNotDeleted(t *testing.T) {
	s := newSavedZoo(t)
	RegisterShipped(feedCosts(1, "Feed costs"))
	if err := ReconcileShipped(s.app); err != nil {
		t.Fatal(err)
	}
	row := shippedRow(t, s, "zoo.feed_costs")
	s.expect(t, "ad", http.MethodDelete, reportURL(row.Id), "", http.StatusBadRequest, `"shipped_key":{"code":"shipped"`, "An admin who owns or edits it can unpublish it instead.")
	shippedMu.Lock()
	delete(shipped, "zoo.feed_costs")
	shippedMu.Unlock()
	s.expect(t, "ad", http.MethodDelete, reportURL(row.Id), "", http.StatusNoContent)
}
