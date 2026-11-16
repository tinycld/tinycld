package reports

import (
	"testing"

	"tinycld.org/core/audit"
	"tinycld.org/core/rlstest"
)

// audit_logs.action is a select under core's real migrations: a value it
// does not list fails the save. A download and a sent email each write one
// row, so both actions must save.
func TestAuditLogsAcceptTheReportActions(t *testing.T) {
	app := rlstest.NewAssembledApp(t)
	for _, action := range []string{"report.export", "report.email"} {
		t.Run(action, func(t *testing.T) {
			meta := map[string]any{"format": "csv", "rows": 5}
			if err := audit.Log(app, action, "reports", "zooreport00001", "Feeding Costs", nil, meta); err != nil {
				t.Fatalf("audit.Log(%q): %v", action, err)
			}
			rows, err := app.FindRecordsByFilter("audit_logs", "action = {:action}", "", 0, 0, map[string]any{"action": action})
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 1 || rows[0].GetString("resource_id") != "zooreport00001" {
				t.Fatalf("audit rows for %q = %d, want 1", action, len(rows))
			}
		})
	}
}
