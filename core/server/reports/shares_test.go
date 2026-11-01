package reports

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

func shareBody(report, user, group, role string) string {
	return fmt.Sprintf(`{"report":%q,"user":%q,"group":%q,"role":%q}`, report, user, group, role)
}

func TestAShareNamesOneUserOrOneGroupNotTheOwner(t *testing.T) {
	s := newSavedZoo(t)
	r := s.id("R")
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, "", "", "viewer"), http.StatusBadRequest, `"user":{"code":"one_target"`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ot"), s.id("keepers"), "viewer"), http.StatusBadRequest, `"user":{"code":"one_target"`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ann"), "", "viewer"), http.StatusBadRequest, `"user":{"code":"owner"`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ed"), "", "viewer"), http.StatusBadRequest, `Value must be unique`)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(r, s.id("ot"), "", "owner"), http.StatusBadRequest, `"role":{"code":"validation_invalid_value"`)
}

// A share changes only its role; a member's derived row follows its grant
// and is neither edited nor removed alone.
func TestAShareChangesOnlyItsRole(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), `{"role":"editor"}`, http.StatusOK, `"role":"editor"`)
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), fmt.Sprintf(`{"user":%q}`, s.id("ot")), http.StatusBadRequest, `"user":{"code":"read_only"`)
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), fmt.Sprintf(`{"report":%q}`, s.id("P")), http.StatusBadRequest, `"report":{"code":"read_only"`)
	derived, err := s.app.FindFirstRecordByFilter(sharesCollection, "user = {:u} && group = {:g}", map[string]any{"u": s.id("gm"), "g": s.id("keepers")})
	if err != nil {
		t.Fatal(err)
	}
	s.expect(t, "ann", http.MethodPatch, shareURL(derived.Id), `{"role":"viewer"}`, http.StatusBadRequest, `"group":{"code":"follows_grant"`)
	s.expect(t, "ann", http.MethodDelete, shareURL(derived.Id), "", http.StatusBadRequest, `"group":{"code":"follows_grant"`)
	// The grant's role reaches the member's row.
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.keepers")), `{"role":"viewer"}`, http.StatusOK)
	if got, _ := s.app.FindRecordById(sharesCollection, derived.Id); got.GetString("role") != "viewer" {
		t.Fatalf("the derived row kept role %q", got.GetString("role"))
	}
	s.expect(t, "gm", http.MethodPatch, reportURL(s.id("R")), `{"title":"R"}`, http.StatusNotFound)
}

// A group grant reaches each member through the group expander: a member
// who joins gets the report, one who leaves loses it, and removing the
// grant removes every member's row.
func TestAGroupShareFollowsTheGroup(t *testing.T) {
	s := newSavedZoo(t)
	s.person(t, "nm", "member")
	if got := s.listTitles(t, "nm", reportsURL); !slices.Equal(got, []string{"P"}) {
		t.Fatalf("before joining: %v", got)
	}
	s.rec(t, "keepers.nm", "group_members", map[string]any{"group": s.id("keepers"), "user": s.id("nm")})
	if got := s.listTitles(t, "nm", reportsURL); !slices.Equal(got, []string{"P", "R"}) {
		t.Fatalf("after joining: %v", got)
	}
	if err := s.app.Delete(s.recs["keepers.gm"]); err != nil {
		t.Fatal(err)
	}
	if got := s.listTitles(t, "gm", reportsURL); !slices.Equal(got, []string{"P"}) {
		t.Fatalf("after leaving: %v", got)
	}
	s.expect(t, "ann", http.MethodDelete, shareURL(s.id("R.keepers")), "", http.StatusNoContent)
	if n, _ := s.app.CountRecords(sharesCollection); n != 3 {
		t.Fatalf("after the grant went: %d share rows, want 3 (R.ed, R.vi, R2.vi)", n)
	}
}

// Deleting a report deletes every share row it had: direct, grant and
// derived.
func TestDeletingAReportDeletesItsShares(t *testing.T) {
	s := newSavedZoo(t)
	s.expect(t, "ann", http.MethodDelete, reportURL(s.id("R")), "", http.StatusNoContent)
	rows, err := s.app.FindRecordsByFilter(sharesCollection, "report = {:r}", "", 0, 0, map[string]any{"r": s.id("R")})
	if err != nil || len(rows) != 0 {
		t.Fatalf("shares left: %d (%v)", len(rows), err)
	}
}

// A report holds at most shareCap shares (direct and grants). The rows a
// grant derives do not count, and the next share past the cap is a 400.
func TestTheShareCapAnswers400(t *testing.T) {
	s := newSavedZoo(t)
	saved := shareCap
	shareCap = 4
	t.Cleanup(func() { shareCap = saved })
	// R has 3: R.ed, R.vi and the keepers grant (gm's derived row is not one).
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(s.id("R"), s.id("ot"), "", "viewer"), http.StatusOK)
	s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(s.id("R"), s.id("cy"), "", "viewer"),
		http.StatusBadRequest, `"report":{"code":"too_many_shares"`)
}

// A database fault while a share is checked answers 500 and is logged, not
// a 400 that reads as a refusal. The fault is a table renamed away after the
// create rule passed: reports for the report read, report_shares for the
// count.
func TestAShareCheckFaultAnswers500(t *testing.T) {
	for table, step := range map[string]string{reportsCollection: "read the shared report", sharesCollection: "count the report's shares"} {
		t.Run(table, func(t *testing.T) {
			s := newSavedZoo(t)
			var records []slog.Record
			silenced := log
			log = slog.New(messagesHandler{&records})
			t.Cleanup(func() { log = silenced })
			s.app.OnRecordCreateRequest(sharesCollection).Bind(&hook.Handler[*core.RecordRequestEvent]{
				Priority: -1,
				Func: func(e *core.RecordRequestEvent) error {
					if _, err := e.App.DB().NewQuery("ALTER TABLE " + table + " RENAME TO gone_" + table).Execute(); err != nil {
						return err
					}
					return e.Next()
				},
			})
			s.expect(t, "ann", http.MethodPost, sharesURL, shareBody(s.id("R"), s.id("ot"), "", "viewer"),
				http.StatusInternalServerError, "The share could not be saved.")
			if len(records) != 1 || records[0].Level != slog.LevelError || !strings.Contains(fmt.Sprint(attrs(records[0])), step) {
				t.Fatalf("logged %v, want one error at %q", records, step)
			}
		})
	}
}

func attrs(r slog.Record) []slog.Attr {
	var out []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		out = append(out, a)
		return true
	})
	return out
}
