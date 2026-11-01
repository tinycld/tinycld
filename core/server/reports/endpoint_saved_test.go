package reports

import (
	"fmt"
	"net/http"
	"testing"
)

const runURL = "/api/reports/run"

func savedRun(id string) string { return fmt.Sprintf(`{"reportId":%q}`, id) }

const savedTopics = `"topics":["collection/report_shares","collection/reports","collection/zoo_animals","collection/zoo_feedings","collection/zoo_keepers"]`

// A saved report runs for anyone who can view it, each with their own
// rights: sharing gives the design, never the data.
func TestASavedReportRunsAsItsReader(t *testing.T) {
	s := newSavedZoo(t)
	s.share(t, "R.bob", "R", "bob", "", "viewer")
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"kind":"total"`, `"cost":4500`, savedTopics)
	s.expect(t, "bob", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK, `"cost":1150`)
	// bob's own draft run of the same spec gives the same figure.
	s.expect(t, "bob", http.MethodPost, runURL, runBodyOf(savedSpec), http.StatusOK, `"cost":1150`)
	// A published report runs for every member, and params and options pass:
	// bob keeps three feedings and asks for a page of one.
	s.expect(t, "bob", http.MethodPost, runURL, fmt.Sprintf(`{"reportId":%q,"params":{},"options":{"limit":1}}`, s.id("P")), http.StatusOK, `"size":1`)
	s.expect(t, "ot", http.MethodPost, runURL, savedRun(s.id("P")), http.StatusOK, `"kind":"total"`)
}

func TestASavedRunTheCallerCannotViewIs403(t *testing.T) {
	s := newSavedZoo(t)
	noAccess := `"reportId":{"code":"no_access"`
	s.expect(t, "ot", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusForbidden, noAccess)
	s.expect(t, "gu", http.MethodPost, runURL, savedRun(s.id("P")), http.StatusForbidden, noAccess)
	// An id that does not exist answers the same: an id tells nobody whether a report exists.
	s.expect(t, "ann", http.MethodPost, runURL, savedRun("nosuchreport00"), http.StatusForbidden, noAccess)
	s.expect(t, "", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusUnauthorized)
	// A share removed while the report is open: the next run is refused.
	s.expect(t, "vi", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK)
	if err := s.app.Delete(s.recs["R.vi"]); err != nil {
		t.Fatal(err)
	}
	s.expect(t, "vi", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusForbidden, noAccess)
}

// Only "not found" and "the rule says no" are a 403. A rule that cannot be
// evaluated is a server fault: a 500, never a refusal that hides it.
func TestASavedRunWhoseRuleFailsIs500(t *testing.T) {
	s := newSavedZoo(t)
	col, err := s.app.FindCollectionByNameOrId(reportsCollection)
	if err != nil {
		t.Fatal(err)
	}
	broken := "owner = @request.auth.id && nosuchfield = 1"
	col.ViewRule = &broken
	if err := s.app.SaveNoValidate(col); err != nil {
		t.Fatal(err)
	}
	code, text := s.call(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")))
	if code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500\n%s", code, text)
	}
}

func TestASavedRunBody(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodPost, runURL, fmt.Sprintf(`{"reportId":%q,"spec":%s}`, s.id("R"), savedSpec),
		http.StatusBadRequest, `not both`)
	s.expect(t, "ann", http.MethodPost, runURL, `{"reportId":7}`, http.StatusBadRequest, `not a report run`)
	s.expect(t, "ann", http.MethodPost, runURL, `{"params":{}}`, http.StatusBadRequest, `no spec`)
}

// A saved report that names something gone opens in the error state: 422
// naming the missing part. One stored with an old field key runs, and says so.
func TestASavedReportNamesWhatIsMissing(t *testing.T) {
	s := newSavedZoo(t)
	gone := s.saved(t, "gone", "ann", "gone", `{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`, false)
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(gone.Id), http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	old := s.saved(t, "old", "ann", "old", `{"version":1,"source":"zoo.animals","columns":[{"id":"m","path":[{"field":"mass"}],"aggregate":"sum"}]}`, false)
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(old.Id), http.StatusOK, `"code":"aliased"`, `"from":"mass"`)
}

// A saved run reads and writes nothing it should not: no report, no share
// and no feeding changes.
func TestASavedRunWritesNothing(t *testing.T) {
	s := newSavedZoo(t)
	before, _ := s.app.FindRecordById(reportsCollection, s.id("R"))
	s.expect(t, "ann", http.MethodPost, runURL, savedRun(s.id("R")), http.StatusOK)
	after, _ := s.app.FindRecordById(reportsCollection, s.id("R"))
	if before.GetString("updated") != after.GetString("updated") {
		t.Fatal("a run changed the report")
	}
	if n, _ := s.app.CountRecords("zoo_feedings"); n != 6 {
		t.Fatalf("the run wrote: %d feedings", n)
	}
}
