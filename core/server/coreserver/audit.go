package coreserver

import (
	"github.com/pocketbase/pocketbase"
	"tinycld.org/core/audit"
)

// RegisterAuditHooks registers audit logging for core collections.
// Package-specific collections are registered by each package in its own
// Register() function using the tinycld.org/core/audit package directly.
func RegisterAuditHooks(app *pocketbase.PocketBase) {
	audit.RegisterCollection(app, "labels", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("name"),
	})
	audit.RegisterCollection(app, "settings", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromFields("app", "key"),
	})
	// pkg_registry.status is the single source of truth for whether a package
	// is active, so enable/disable — plus install, uninstall, and version
	// changes — all land here as record writes worth auditing.
	audit.RegisterCollection(app, "pkg_registry", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("name"),
	})
	audit.RegisterCollections(app, []string{"label_assignments", "org_pkg_access"}, nil)
}
