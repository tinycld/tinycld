package reports

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/groups"
	"tinycld.org/core/offboard"
)

// The saved-report zoo: newZoo plus sharing, people in every role, and two
// reports of ann's.
//
//	ann  member, owns R (unpublished) and P (published by ad)
//	ed   member, editor of R (a direct share)
//	vi   member, viewer of R (a direct share) and editor of cy's R2, so an
//	     editor elsewhere and only a viewer on R
//	gm   member of the group keepers; keepers is an editor of R (a grant)
//	ot   member, no share
//	ad   admin, no share
//	gu   guest, no share
//	cy   member, owns R2
type savedZoo struct {
	*zoo
	mux http.Handler
}

// spec of a saved report on the zoo: the cost of every feeding by day.
const savedSpec = feedingsByDay

func newSavedZoo(t *testing.T) *savedZoo {
	t.Helper()
	z := newZoo(t)
	groups.ResetForTesting()
	offboard.ResetHandlersForTesting()
	t.Cleanup(groups.ResetForTesting)
	t.Cleanup(offboard.ResetHandlersForTesting)
	RegisterSharing(z.app)
	groups.BindHooks(z.app)
	Register(z.app)

	for name, role := range map[string]string{"ed": "member", "vi": "member", "gm": "member", "ot": "member", "ad": "admin", "gu": "guest", "cy": "member"} {
		z.person(t, name, role)
	}
	keepers := z.group(t, "keepers", "gm")
	z.saved(t, "R", "ann", "R", savedSpec, false)
	z.saved(t, "P", "ann", "P", savedSpec, true)
	z.saved(t, "R2", "cy", "R2", savedSpec, false)
	z.share(t, "R.ed", "R", "ed", "", "editor")
	z.share(t, "R.vi", "R", "vi", "", "viewer")
	z.share(t, "R.keepers", "R", "", keepers.Id, "editor")
	z.share(t, "R2.vi", "R2", "vi", "", "editor")
	return &savedZoo{zoo: z, mux: z.mux(t)}
}

// person adds a user with a role; z.user adds members.
func (z *zoo) person(t testing.TB, name, role string) *core.Record {
	t.Helper()
	u := z.user(t, name)
	u.Set("role", role)
	if err := z.app.Save(u); err != nil {
		t.Fatalf("set role of %s: %v", name, err)
	}
	return u
}

// group adds a group with members (fixture keys of users).
func (z *zoo) group(t testing.TB, name string, members ...string) *core.Record {
	t.Helper()
	g := z.rec(t, name, "groups", map[string]any{"name": name})
	for _, m := range members {
		z.rec(t, name+"."+m, "group_members", map[string]any{"group": g.Id, "user": z.id(m)})
	}
	return g
}

// saved stores a report directly (no request hooks), as a migration or a
// previous version of the server would have.
func (z *zoo) saved(t testing.TB, key, owner, title, spec string, published bool) *core.Record {
	t.Helper()
	source := "zoo.unreadable" // a spec Load refuses still has a stored source
	if s, _, err := Load([]byte(spec)); err == nil {
		source = s.Source
	}
	data := map[string]any{"owner": z.id(owner), "title": title, "spec": spec, "source": source, "published": published}
	if published {
		data["published_by"] = z.id("ad")
	}
	return z.rec(t, key, reportsCollection, data)
}

// share stores a direct share (user) or a group grant (group).
func (z *zoo) share(t testing.TB, key, report, user, group, role string) *core.Record {
	t.Helper()
	data := map[string]any{"report": z.id(report), "group": group, "role": role}
	if user != "" {
		data["user"] = z.id(user)
	}
	return z.rec(t, key, sharesCollection, data)
}

// call sends one request through the real router as user ("" = signed out)
// and returns the status and body.
func (s *savedZoo) call(t *testing.T, user, method, url, body string) (int, string) {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, url, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if user != "" {
		req.Header.Set("Authorization", s.token(t, user))
	}
	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

// expect is call that fails the test unless the status is want and the body
// holds every part.
func (s *savedZoo) expect(t *testing.T, user, method, url, body string, want int, parts ...string) string {
	t.Helper()
	got, text := s.call(t, user, method, url, body)
	if got != want {
		t.Fatalf("%s %s as %q: status %d, want %d\n%s", method, url, user, got, want, text)
	}
	for _, p := range parts {
		if !strings.Contains(text, p) {
			t.Fatalf("%s %s as %q: body lacks %s\n%s", method, url, user, p, text)
		}
	}
	return text
}

func reportURL(id string) string { return "/api/collections/reports/records/" + id }
func shareURL(id string) string  { return "/api/collections/report_shares/records/" + id }

const (
	reportsURL = "/api/collections/reports/records"
	sharesURL  = "/api/collections/report_shares/records"
)
