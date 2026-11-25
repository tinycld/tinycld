package oauth

import "testing"

// The reports scope opens the list of saved reports, the run and the file. A
// report is built, shared and sent by email in the app: every other report
// route, and every write of a saved report, stays closed to a token.
func TestReportsScopeOpensOnlyTheReadRoutes(t *testing.T) {
	withEmptyRegistry(t)
	closed := []struct{ method, path string }{
		{"POST", "/api/collections/reports/records"},
		{"PATCH", "/api/collections/reports/records/abc"},
		{"DELETE", "/api/collections/reports/records/abc"},
		{"GET", "/api/collections/report_shares/records"},
		{"GET", "/api/reports/sources"},
		{"POST", "/api/reports/param-options"},
		{"POST", "/api/reports/email"},
		{"GET", "/api/reports/email-status"},
		{"GET", "/api/reports/run"},
	}
	for _, c := range closed {
		if got := ScopeForRoute(c.method, c.path); len(got) != 0 {
			t.Errorf("ScopeForRoute(%s %s) = %q, want no scope", c.method, c.path, got)
		}
	}
}
