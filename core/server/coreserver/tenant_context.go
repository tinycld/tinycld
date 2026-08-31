package coreserver

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/mailproto"
)

// The single-Register package contract: a feature package exports ONE
// composition entry point, `Register(app *pocketbase.PocketBase)`, and it runs
// unchanged in the single-org app and in a hosting tenant. The rare package
// that must behave differently hosted (mail: the host binds real ports, a
// tenant serves router-injected sockets) DETECTS tenancy from the app instead
// of exporting a second function: RegisterTenant stamps a TenantContext into
// the app's store before feature registration, and GetTenantContext/IsTenant
// read it back. Packages that bind no listener need no detection at all —
// which is the vast majority, and the point of the contract.
//
// The historical Register/RegisterTenant split existed for exactly two
// things: DAV mounts (now self-mounted by feature Go in both modes — a
// per-org artifact contains exactly the org's features, so the artifact is
// the gate) and mail's listeners (now the TenantContext.Mail injection).

// MailListeners are the router-managed mail sockets a tenant serves on, as
// lazy ListenFuncs. The router owns the public ports (:993/:465/:25),
// terminates TLS with the wildcard cert, and forwards plaintext over private
// per-org unix sockets — these listeners. A nil entry means the router
// manages no socket for that service and the tenant must not start it.
type MailListeners struct {
	IMAP       mailproto.ListenFunc
	Submission mailproto.ListenFunc
	InboundMX  mailproto.ListenFunc
}

// TenantContext is what a feature package may learn about the tenant process
// it is registering into. Present (GetTenantContext ok) only in hosting
// tenant mode; a single-org app has none.
type TenantContext struct {
	// Slug is the org's slug (identification and logging only).
	Slug string
	// Mail are the router-managed mail listeners (zero value = the org runs
	// no mail listeners).
	Mail MailListeners
	// ControlSocket is the router-bound deploy-proposal socket path (empty =
	// no deploy channel).
	ControlSocket string
}

// tenantContextKey namespaces the store entry; the app store is shared with
// PocketBase internals and package code.
const tenantContextKey = "tinycld.tenantContext"

// setTenantContext stamps the context. Called exactly once, at the top of
// RegisterTenant — BEFORE RegisterExtras runs, so every feature Register can
// see it.
func setTenantContext(app core.App, tc TenantContext) {
	app.Store().Set(tenantContextKey, tc)
}

// GetTenantContext reports the tenant context, and whether this app is a
// hosting tenant at all.
func GetTenantContext(app core.App) (TenantContext, bool) {
	v := app.Store().Get(tenantContextKey)
	tc, ok := v.(TenantContext)
	return tc, ok
}

// IsTenant reports whether app is composed as a hosting tenant. Feature
// packages use it to skip behavior a tenant must not have — binding a port is
// the canonical example (the router owns every listening socket).
func IsTenant(app core.App) bool {
	_, ok := GetTenantContext(app)
	return ok
}
