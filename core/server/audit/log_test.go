package audit

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"
)

// createAuditLogsCollection builds a minimal audit_logs collection for tests
// that need to write audit rows without running the real pb_migrations. The
// action field is plain text here (not the select used by the real migration)
// so non-collection actions like "backup.created" can be written.
func createAuditLogsCollection(t *testing.T, app core.App) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	col := core.NewBaseCollection("audit_logs")
	col.Fields.Add(
		&core.TextField{Name: "action", Required: true},
		&core.TextField{Name: "resource_type"},
		&core.TextField{Name: "resource_id"},
		&core.TextField{Name: "resource_label"},
		&core.RelationField{Name: "actor", CollectionId: users.Id, MaxSelect: 1},
		&core.TextField{Name: "ip_address"},
		&core.TextField{Name: "user_agent"},
		&core.JSONField{Name: "metadata"},
		&core.JSONField{Name: "changes"},
		&core.JSONField{Name: "snapshot"},
	)
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
}

func TestLogWritesSystemRowWithoutRequest(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(app.Cleanup)
	createAuditLogsCollection(t, app)

	if err := Log(app, "backup.created", "backup", "bk_1", "manual backup", nil, map[string]any{"bytes": 42}); err != nil {
		t.Fatal(err)
	}
	rows, err := app.FindRecordsByFilter("audit_logs", "action = 'backup.created'", "", 0, 0)
	if err != nil || len(rows) != 1 {
		t.Fatalf("want 1 row, got %d (%v)", len(rows), err)
	}

	raw, ok := rows[0].Get("metadata").(types.JSONRaw)
	if !ok {
		t.Fatalf("expected metadata to be types.JSONRaw, got %T", rows[0].Get("metadata"))
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal metadata: %v", err)
	}
	if meta["source"] != "system" || meta["bytes"] != float64(42) {
		t.Fatalf("unexpected metadata %v", meta)
	}
}
