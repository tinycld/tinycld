package reports

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func createBody(owner, extra string) string {
	return fmt.Sprintf(`{"owner":%q,"title":"New","spec":%s%s}`, owner, savedSpec, extra)
}

func TestOnlyAnAdminPublishes(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodPost, reportsURL, createBody(s.id("ann"), `,"published":true`),
		http.StatusForbidden, `"published":{"code":"admin_only"`)
	s.expect(t, "ad", http.MethodPost, reportsURL, createBody(s.id("ad"), `,"published":true`),
		http.StatusOK, `"published":true`, fmt.Sprintf(`"published_by":%q`, s.id("ad")))

	// ed edits R but is no admin; ad is an admin and owns its own report.
	s.expect(t, "ed", http.MethodPatch, reportURL(s.id("R")), `{"published":true}`,
		http.StatusForbidden, `"published":{"code":"admin_only"`)
	mine := s.saved(t, "adR", "ad", "adR", savedSpec, false)
	s.expect(t, "ad", http.MethodPatch, reportURL(mine.Id), `{"published":true}`,
		http.StatusOK, `"published":true`, fmt.Sprintf(`"published_by":%q`, s.id("ad")))
	s.expect(t, "ad", http.MethodPatch, reportURL(mine.Id), `{"published":false}`,
		http.StatusOK, `"published":false`, `"published_by":""`)
	// The owner, no admin, cannot unpublish P either.
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("P")), `{"published":false}`,
		http.StatusForbidden, `"published":{"code":"admin_only"`)
	// A save that leaves published as it was is no publish.
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("P")), `{"published":true,"title":"P2"}`, http.StatusOK, `"title":"P2"`)
}

// The server owns source, published_by, broken and the shipped fields: a
// client value is replaced, on create and on update.
func TestTheServerOwnsItsFields(t *testing.T) {
	s := newSavedZoo(t)
	lies := fmt.Sprintf(`,"source":"zoo.lies","published_by":%q,"broken":{"status":1},"shipped_key":"zoo.fake","shipped_revision":9`, s.id("ad"))
	s.expect(t, "ann", http.MethodPost, reportsURL, createBody(s.id("ann"), lies), http.StatusOK,
		`"source":"zoo.feedings"`, `"published_by":""`, `"broken":null`, `"shipped_key":""`, `"shipped_revision":0`)
	s.expect(t, "ed", http.MethodPatch, reportURL(s.id("R")), `{"source":"zoo.lies","shipped_key":"zoo.fake","shipped_revision":3,"broken":{"status":1}}`,
		http.StatusOK, `"source":"zoo.feedings"`, `"shipped_key":""`, `"shipped_revision":0`, `"broken":null`)
	s.expect(t, "ed", http.MethodPatch, reportURL(s.id("R")), fmt.Sprintf(`{"owner":%q}`, s.id("ed")),
		http.StatusBadRequest, `"owner":{"code":"read_only"`)
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("R")), fmt.Sprintf(`{"owner":%q}`, s.id("ann")), http.StatusOK)
}

// A spec is checked on save as on run, and saved as Load returns it: an
// aliased field is stored under its current key.
func TestASpecIsCheckedAndSavedLoaded(t *testing.T) {
	s := newSavedZoo(t)
	ann := s.id("ann")
	s.expect(t, "ann", http.MethodPost, reportsURL,
		fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"source":"zoo.feedings","columns":[{"id":"1x","path":[{"field":"cost"}]}]}}`, ann),
		http.StatusBadRequest, `"message":"The report is not valid."`, `"columns.0.id":{"code":"invalid"`)
	s.expect(t, "ann", http.MethodPost, reportsURL,
		fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}}`, ann),
		http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":2}}`, ann),
		http.StatusBadRequest, `"version":{"code":"invalid"`)
	body := s.expect(t, "ann", http.MethodPost, reportsURL,
		fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"source":"zoo.animals","columns":[{"id":"m","path":[{"field":"mass"}],"aggregate":"sum"}]}}`, ann),
		http.StatusOK, `"source":"zoo.animals"`)
	if !strings.Contains(body, `"field":"weight"`) || strings.Contains(body, `"mass"`) {
		t.Fatalf("the alias was not saved resolved: %s", body)
	}
}

// An update that keeps a broken spec keeps it, so a broken report can be
// renamed; a changed spec must be valid, and clears the mark.
func TestABrokenReportCanBeRenamedNotSavedBroken(t *testing.T) {
	s := newSavedZoo(t)
	gone := `{"version":1,"source":"zoo.gone","columns":[{"id":"a","path":[{"field":"cost"}]}]}`
	b := s.saved(t, "B", "ann", "B", gone, false)
	b.Set("broken", `{"status":422}`)
	if err := s.app.Save(b); err != nil {
		t.Fatal(err)
	}
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"title":"B renamed"}`, http.StatusOK, `"title":"B renamed"`, `"broken":{"status":422}`)
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"spec":{"version":1,"source":"zoo.gone","columns":[{"id":"b","path":[{"field":"cost"}]}]}}`,
		http.StatusUnprocessableEntity, `"source":{"code":"unknown_source"`)
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"spec":`+savedSpec+`}`, http.StatusOK, `"broken":null`, `"source":"zoo.feedings"`)
}

// A stored spec Load refuses (a version from the future) is not checked when
// an update leaves it as it was: the report can be renamed, spec unchanged.
func TestAnUnloadableStoredSpecCanBeRenamed(t *testing.T) {
	s := newSavedZoo(t)
	b := s.saved(t, "V", "ann", "V", `{"version":99}`, false)
	body := s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"title":"V renamed"}`, http.StatusOK, `"title":"V renamed"`, `"spec":{"version":99}`)
	if strings.Contains(body, `"version":1`) {
		t.Fatalf("the spec changed: %s", body)
	}
	s.expect(t, "ann", http.MethodPatch, reportURL(b.Id), `{"spec":{"version":99,"x":1}}`, http.StatusBadRequest)
}

// Every input that grows a saved report is capped, and a value past a cap
// answers 400, not 500.
func TestSavedReportCaps(t *testing.T) {
	s := newSavedZoo(t)
	ann := s.id("ann")
	big := fmt.Sprintf(`{"owner":%q,"title":"x","spec":{"version":1,"x":"%s"}}`, ann, strings.Repeat("x", maxSpecBytes))
	s.expect(t, "ann", http.MethodPost, reportsURL, big, http.StatusBadRequest, `"spec":{"code":"too_large"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":%q,"spec":%s}`, ann, strings.Repeat("t", 201), savedSpec),
		http.StatusBadRequest, `"title":{"code":"validation_max_text_constraint"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","description":%q,"spec":%s}`, ann, strings.Repeat("d", 2001), savedSpec),
		http.StatusBadRequest, `"description":{"code":"validation_max_text_constraint"`)
	s.expect(t, "ann", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"","spec":%s}`, ann, savedSpec),
		http.StatusBadRequest, `"title":{"code":"validation_required"`)
}

// A superuser request is not guarded: it may set any field.
func TestASuperuserIsNotGuarded(t *testing.T) {
	s := newSavedZoo(t)
	superusers, err := s.app.FindCollectionByNameOrId(core.CollectionNameSuperusers)
	if err != nil {
		t.Fatal(err)
	}
	su := core.NewRecord(superusers)
	su.SetEmail("root@zoo.test")
	su.SetPassword("password123")
	if err := s.app.Save(su); err != nil {
		t.Fatal(err)
	}
	s.recs["root"] = su
	s.expect(t, "root", http.MethodPatch, reportURL(s.id("R")), `{"published":true,"shipped_key":"zoo.fixed"}`,
		http.StatusOK, `"published":true`, `"shipped_key":"zoo.fixed"`)
}
