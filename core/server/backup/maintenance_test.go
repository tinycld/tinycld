package backup

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/hook"
)

// The 503 has to let the progress protocol through. A restore announces itself
// through the ledger row, and the CLI's only way to learn the outcome is to read
// that row — so a blanket 503 hides exactly the state it is reporting. This
// table is the contract: everything not on the exempt list stays blocked,
// including every write.
func TestExemptFromMaintenance(t *testing.T) {
	cases := []struct {
		method string
		path   string
		exempt bool
	}{
		{http.MethodGet, "/api/health", true},
		{http.MethodGet, "/api/org-backups/abc123", true},
		{http.MethodPatch, "/api/org-backups/restore/abc123", true},
		{http.MethodGet, "/api/collections/backups/records", true},
		{http.MethodGet, "/api/collections/backups/records/abc123", true},

		{http.MethodPost, "/api/org-backups", false},
		{http.MethodPost, "/api/org-backups/restore", false},
		{http.MethodGet, "/api/collections/users/records", false},
		{http.MethodPost, "/api/collections/backups/records", false},
		{http.MethodDelete, "/api/collections/backups/records/abc123", false},
		// The method matters, not only the path: a write to an exempt path is
		// still a write into a pb_data the next process replaces.
		{http.MethodPost, "/api/org-backups/abc123", false},
		{http.MethodPost, "/api/health", false},
		{http.MethodGet, "/", false},
	}
	for _, c := range cases {
		if got := exemptFromMaintenance(c.method, c.path); got != c.exempt {
			t.Errorf("%s %s: exempt = %v, want %v", c.method, c.path, got, c.exempt)
		}
	}
}

// The middleware itself has to honour the table, not just the predicate: a 503
// body of {"status":"restoring"} is what the CLI polls on, and a blocked request
// must never reach the next handler.
func TestMaintenanceMiddlewareBlocksAndExempts(t *testing.T) {
	resetRestoreState(t)
	mw := MaintenanceMiddleware()
	call := func(method, path string) (int, string, bool) {
		rec := httptest.NewRecorder()
		reached := false
		re := &core.RequestEvent{}
		re.Request = httptest.NewRequest(method, path, nil)
		re.Response = rec
		// A real hook chain, so re.Next() means what it means in the router: the
		// middleware either passes control on or answers instead.
		h := &hook.Hook[*core.RequestEvent]{}
		h.BindFunc(mw)
		h.BindFunc(func(e *core.RequestEvent) error {
			reached = true
			return nil
		})
		if err := h.Trigger(re); err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		return rec.Code, rec.Body.String(), reached
	}

	// Nothing is blocked while no restore is in flight.
	if _, _, reached := call(http.MethodPost, "/api/collections/users/records"); !reached {
		t.Fatal("a write must be served when no restore is in flight")
	}

	restoring.Store(true)

	if _, _, reached := call(http.MethodGet, "/api/collections/backups/records"); !reached {
		t.Fatal("the ledger list must stay reachable while restoring")
	}
	code, body, reached := call(http.MethodPost, "/api/collections/users/records")
	if reached {
		t.Fatal("a write must not reach the next handler while restoring")
	}
	if code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", code)
	}
	if !strings.Contains(body, `"status":"restoring"`) {
		t.Fatalf("body = %q", body)
	}
}
