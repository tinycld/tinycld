package backup

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

// MaintenanceMiddleware answers 503 while a restore is staging, and while one
// has swapped its data in but the process has not finalized it. Serving in
// either window would either discard the write (the staged copy is what boots
// next) or expose data the process has not finished accepting.
//
// Health stays reachable so a supervisor can tell a restoring server from a dead
// one and does not kill it mid-restore.
func MaintenanceMiddleware() func(*core.RequestEvent) error {
	return func(re *core.RequestEvent) error {
		if !Restoring() || re.Request.URL.Path == "/api/health" {
			return re.Next()
		}
		return re.JSON(http.StatusServiceUnavailable, map[string]any{
			"status":  "restoring",
			"message": "Restoring from a backup. Try again in a minute.",
		})
	}
}
