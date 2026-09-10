package coreserver

import (
	"github.com/pocketbase/pocketbase/core"
)

// DEPRECATED, and deleted once mail and hosting/limits have migrated: the seam
// is now EmbeddedContext (embedded.go), which says what it means — a supervisor
// supplied this process's wiring — without naming any particular supervisor.
//
// These are aliases, not a parallel implementation: same store key, same
// values, so a caller on either name sees what the other stamped.

// MailListeners is the former name of MailSockets.
type MailListeners = MailSockets

// TenantContext is the former name of EmbeddedContext.
type TenantContext = EmbeddedContext

// GetTenantContext is the former name of GetEmbeddedContext.
func GetTenantContext(app core.App) (TenantContext, bool) {
	return GetEmbeddedContext(app)
}
