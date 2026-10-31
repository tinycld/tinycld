package reports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"testing"

	"tinycld.org/core/oauth"
	"tinycld.org/core/rlstest"
)

// Every rule cell of reports and report_shares, through PocketBase's API as
// each kind of reader. PocketBase answers a refused list with fewer items, a
// refused view, update or delete with 404, and a refused create with 400.
var readers = []string{"ann", "ed", "vi", "gm", "ot", "ad", "gu", ""}

func (s *savedZoo) listTitles(t *testing.T, user, url string) []string {
	t.Helper()
	body := s.expect(t, user, http.MethodGet, url+"?perPage=200&sort=title&fields=title", "", http.StatusOK)
	var page struct {
		Items []struct {
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, it := range page.Items {
		out = append(out, it.Title)
	}
	return out
}

func TestReportRulesListAndView(t *testing.T) {
	s := newSavedZoo(t)
	list := map[string][]string{
		"ann": {"P", "R"}, "ed": {"P", "R"}, "vi": {"P", "R", "R2"}, "gm": {"P", "R"},
		"ot": {"P"}, "ad": {"P"}, "gu": {}, "": {},
	}
	viewR := map[string]int{"ann": 200, "ed": 200, "vi": 200, "gm": 200, "ot": 404, "ad": 404, "gu": 404, "": 404}
	viewP := map[string]int{"ann": 200, "ed": 200, "vi": 200, "gm": 200, "ot": 200, "ad": 200, "gu": 404, "": 404}
	for _, who := range readers {
		if got := s.listTitles(t, who, reportsURL); !slices.Equal(got, list[who]) {
			t.Errorf("list as %q = %v, want %v", who, got, list[who])
		}
		s.expect(t, who, http.MethodGet, reportURL(s.id("R")), "", viewR[who])
		s.expect(t, who, http.MethodGet, reportURL(s.id("P")), "", viewP[who])
	}
}

// A guest sees a report shared with them, and never a published one; a
// disabled owner sees nothing.
func TestReportRulesGuestShareAndDisabledOwner(t *testing.T) {
	s := newSavedZoo(t)
	s.share(t, "R.gu", "R", "gu", "", "viewer")
	if got := s.listTitles(t, "gu", reportsURL); !slices.Equal(got, []string{"R"}) {
		t.Fatalf("guest with a share lists %v, want [R]", got)
	}
	ann := s.recs["ann"]
	ann.Set("disabled", true)
	if err := s.app.Save(ann); err != nil {
		t.Fatal(err)
	}
	if got := s.listTitles(t, "ann", reportsURL); len(got) != 0 {
		t.Fatalf("a disabled owner lists %v", got)
	}
}

func TestReportRulesCreate(t *testing.T) {
	s := newSavedZoo(t)
	want := map[string]int{"ann": 200, "ed": 200, "vi": 200, "gm": 200, "ot": 200, "ad": 200, "gu": 200, "": 400}
	for _, who := range readers {
		owner := ""
		if who != "" {
			owner = s.id(who)
		}
		body := fmt.Sprintf(`{"owner":%q,"title":"mine","source":"zoo.feedings","spec":%s}`, owner, savedSpec)
		s.expect(t, who, http.MethodPost, reportsURL, body, want[who])
	}
	// Nobody creates a report for somebody else.
	s.expect(t, "ad", http.MethodPost, reportsURL, fmt.Sprintf(`{"owner":%q,"title":"x","source":"zoo.feedings","spec":%s}`, s.id("ann"), savedSpec), 400, `Failed to create record.`)
}

func TestReportRulesUpdate(t *testing.T) {
	s := newSavedZoo(t)
	want := map[string]int{"ann": 200, "ed": 200, "vi": 404, "gm": 200, "ot": 404, "ad": 404, "gu": 404, "": 404}
	for _, who := range readers {
		s.expect(t, who, http.MethodPatch, reportURL(s.id("R")), `{"title":"R"}`, want[who])
	}
	// An admin who neither owns nor edits a published report cannot edit it.
	s.expect(t, "ad", http.MethodPatch, reportURL(s.id("P")), `{"title":"P"}`, 404)
	s.expect(t, "ann", http.MethodPatch, reportURL(s.id("P")), `{"title":"P"}`, 200)
}

func TestReportRulesDelete(t *testing.T) {
	s := newSavedZoo(t)
	unpublished := map[string]int{"ann": 204, "ed": 404, "vi": 404, "gm": 404, "ot": 404, "ad": 404, "gu": 404, "": 404}
	published := map[string]int{"ann": 204, "ed": 404, "vi": 404, "gm": 404, "ot": 404, "ad": 204, "gu": 404, "": 404}
	for i, who := range readers {
		for kind, want := range map[string]map[string]int{"unpublished": unpublished, "published": published} {
			key := fmt.Sprintf("del%d%s", i, kind)
			s.saved(t, key, "ann", key, savedSpec, kind == "published")
			s.share(t, key+".ed", key, "ed", "", "editor")
			s.share(t, key+".vi", key, "vi", "", "viewer")
			s.share(t, key+".keepers", key, "", s.id("keepers"), "editor")
			s.expect(t, who, http.MethodDelete, reportURL(s.id(key)), "", want[who])
			_, err := s.app.FindRecordById(reportsCollection, s.id(key))
			if gone := err != nil; gone != (want[who] == 204) {
				t.Fatalf("delete %s as %q: gone = %v", kind, who, gone)
			}
		}
	}
}

func TestShareRulesCreateAndDelete(t *testing.T) {
	s := newSavedZoo(t)
	add := map[string]int{"ann": 200, "ed": 200, "vi": 400, "gm": 200, "ot": 400, "ad": 400, "gu": 400, "": 400}
	remove := map[string]int{"ann": 204, "ed": 204, "vi": 404, "gm": 204, "ot": 404, "ad": 404, "gu": 404, "": 404}
	for i, who := range readers {
		added := s.person(t, fmt.Sprintf("a%d", i), "member")
		s.expect(t, who, http.MethodPost, sharesURL, fmt.Sprintf(`{"report":%q,"user":%q,"role":"viewer"}`, s.id("R"), added.Id), add[who])
		removed := fmt.Sprintf("r%d", i)
		s.person(t, removed, "member")
		row := s.share(t, "R."+removed, "R", removed, "", "viewer")
		s.expect(t, who, http.MethodDelete, shareURL(row.Id), "", remove[who])
		_, err := s.app.FindRecordById(sharesCollection, row.Id)
		if gone := err != nil; gone != (remove[who] == 204) {
			t.Fatalf("remove a share as %q: gone = %v", who, gone)
		}
	}
}

// Managers see every share of the report; anyone else sees only their own
// rows (a group member's derived row included).
func TestShareRulesList(t *testing.T) {
	s := newSavedZoo(t)
	count := func(who string) int {
		body := s.expect(t, who, http.MethodGet, sharesURL+"?perPage=200", "", http.StatusOK)
		var page struct {
			TotalItems int `json:"totalItems"`
		}
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatal(err)
		}
		return page.TotalItems
	}
	// R: ed, vi, the keepers grant and gm's derived row; R2: vi.
	want := map[string]int{"ann": 4, "ed": 4, "vi": 2, "gm": 4, "ot": 0, "ad": 0, "gu": 0, "": 0}
	for _, who := range readers {
		if got := count(who); got != want[who] {
			t.Errorf("shares listed as %q = %d, want %d", who, got, want[who])
		}
	}
}

// No OAuth scope covers saved reports: a report reads across every
// package, so an API token reaches neither collection (default-deny).
func TestSavedReportsAreNotOpenToOAuthTokens(t *testing.T) {
	for _, c := range []string{reportsCollection, sharesCollection} {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPatch, http.MethodDelete} {
			if got := oauth.ScopeForRoute(method, "/api/collections/"+c+"/records"); len(got) != 0 {
				t.Errorf("%s %s resolved to scopes %v, want default-deny", method, c, got)
			}
		}
	}
}

// Every rule that reads report_shares.user also requires a login, so an
// anonymous request's NULL id never matches a grant row's empty user.
// (rlstest's TestCoreMigrations_EveryAccessPathRequiresLogin scans every
// access path of these core rules too.)
func TestReportRulesGuardGrantRows(t *testing.T) {
	s := newSavedZoo(t)
	rlstest.RequireAuthGuardOnGrantRules(t, s.app, sharesCollection)
}
