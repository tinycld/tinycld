package audit

import (
	"encoding/json"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// Log writes one audit row for an action that is not a collection write —
// a job that ran, a file that was exported. re may be nil for a system
// actor; metadata is merged over the request info (e.g. the system-source
// marker setRequestInfo sets when re is nil).
func Log(app core.App, action, resourceType, resourceID, label string, re *core.RequestEvent, metadata map[string]any) error {
	// newAuditRecord only logs a lookup failure and returns nil; check the
	// collection ourselves first so a missing audit_logs table surfaces as an
	// error to the caller instead of a silently dropped audit write.
	if _, err := app.FindCollectionByNameOrId("audit_logs"); err != nil {
		return fmt.Errorf("audit: audit_logs collection unavailable: %w", err)
	}
	rec := newAuditRecord(app, action, resourceType, resourceID, label)
	if rec == nil {
		return fmt.Errorf("audit: failed to build audit record for action %q", action)
	}
	setRequestInfo(rec, re)
	if len(metadata) > 0 {
		merged := map[string]any{}
		if raw, ok := rec.Get("metadata").(types.JSONRaw); ok {
			var existing map[string]any
			if json.Unmarshal(raw, &existing) == nil {
				for k, v := range existing {
					merged[k] = v
				}
			}
		}
		for k, v := range metadata {
			merged[k] = v
		}
		rec.Set("metadata", merged)
	}
	return app.Save(rec)
}
