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

	// Group membership is an access-control change: who can see what moves
	// with every row here, so both land in the audit log.
	audit.RegisterCollection(app, "groups", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("name"),
	})
	audit.RegisterCollections(app, []string{"group_members"}, nil)

	// A report share or a publish changes who sees a report's design. The
	// spec and the server's broken mark stay out of the diffs: a spec can
	// hold a megabyte, so the log says that it changed, not what it holds.
	audit.RegisterCollection(app, "reports", &audit.CollectionConfig{
		ExtractLabel: audit.LabelFromField("title"),
		Redact:       []string{"spec", "broken"},
	})
	audit.RegisterCollections(app, []string{"report_shares"}, nil)

	// An owner or admin removing a member's authenticator app (a recovery
	// reset) is a collection delete. Passkeys and app passwords register
	// with their own packages.
	audit.RegisterCollections(app, []string{"mfa_totp"}, nil)
}
