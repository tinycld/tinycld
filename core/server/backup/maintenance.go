package backup

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// exemptWhileRestoring is the only traffic served while a restore is in flight.
// Everything on it is READ-ONLY about the restore, or the one write that can
// unblock it — nothing that lands a row in a pb_data the next process replaces.
//
//   - GET /api/health — a supervisor has to tell a restoring server from a dead
//     one, or it kills the process mid-restore.
//   - GET /api/org-backups/… and GET /api/collections/backups/records — the
//     ledger row and the list. A client that cannot read the row cannot learn
//     the restore's outcome, so the 503 would hide exactly the progress it is
//     announcing.
//   - PATCH /api/org-backups/restore/… — handing a stalled restore a fresh
//     presigned URL. It is the only way to unblock one, so blocking it makes an
//     expired URL unrecoverable.
var exemptWhileRestoring = []struct {
	method string
	prefix string
}{
	{http.MethodGet, "/api/health"},
	{http.MethodGet, "/api/org-backups/"},
	{http.MethodPatch, "/api/org-backups/restore/"},
	{http.MethodGet, "/api/collections/backups/records"},
}

func exemptFromMaintenance(method, path string) bool {
	for _, e := range exemptWhileRestoring {
		if method == e.method && strings.HasPrefix(path, e.prefix) {
			return true
		}
	}
	return false
}

// MaintenanceMiddleware answers 503 while a restore is staging, and while one
// has swapped its data in but the process has not finalized it. Serving in
// either window would either discard the write (the staged copy is what boots
// next) or expose data the process has not finished accepting.
func MaintenanceMiddleware() func(*core.RequestEvent) error {
	return func(re *core.RequestEvent) error {
		if !Restoring() || exemptFromMaintenance(re.Request.Method, re.Request.URL.Path) {
			return re.Next()
		}
		return re.JSON(http.StatusServiceUnavailable, map[string]any{
			"status":  "restoring",
			"message": "Restoring from a backup. Try again in a minute.",
		})
	}
}
