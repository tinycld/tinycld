package coreserver

import (
	"fmt"
	"net/http"
	"path/filepath"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/quota"
)

// TenantOptions configure RegisterTenant. Everything here is materialized or
// resolved by the multi-org router and handed to the tenant process — a
// tenant never reads deployment-wide flags or env for these.
type TenantOptions struct {
	// Slug is the org's slug (identification and logging only; surfaced to
	// feature packages via the TenantContext).
	Slug string

	// HooksDir / MigrationsDir are the org's materialized pb_hooks /
	// pb_migrations symlink farms (built by the router from the org's
	// resolved package set).
	HooksDir      string
	MigrationsDir string
	HooksPoolSize int

	// OrgDir is the org's root directory (pb_data, .runtime, .deploy).
	// ArtifactDir is the directory of the build artifact this tenant booted
	// from — the tenant binary's own directory when a recipe.json sits beside
	// it (DESIGN-org-package-agency D4). When BOTH are set the tenant is
	// artifact-booted and RegisterTenant binds the boot-time package-state
	// reconcile: consume .runtime/deploy-result.json into pkg_install_log and
	// mirror the artifact's built-in set into pkg_registry. Empty on the
	// shared serve-org binary and standalone deployments — they have no
	// built-in set to reconcile from.
	OrgDir      string
	ArtifactDir string

	// ControlSocket is the ROUTER-bound unix socket for deploy proposals
	// (design D1). When set on an artifact-booted tenant, RegisterTenant
	// serves the hosted Packages UI endpoints (pkg_hosted.go): the same
	// /api/admin/packages surface as the single-tenant app, proposing
	// deploys over this socket instead of rebuilding in-process. Empty =
	// no deploy channel, no package endpoints (the pinned-menu serve-org
	// posture).
	ControlSocket string

	// RegisterExtras is the tenant's seam for FEATURE package Go — the
	// counterpart of Options.RegisterExtras in the single-org composition.
	// The dual-mode binary passes its generated registerPackageExtensions —
	// the SAME registrar host mode uses; a per-org build links exactly the
	// org's package set, so the artifact is the gate.
	//
	// It runs immediately after registerSharedEarly and BEFORE quota and jsvm,
	// for the same load-bearing reasons as the host composition: a feature's
	// record hooks must bind before quota's enforcement hook (drive corrects a
	// forged size the quota check then reads), and every `$`-binding or loader
	// binding a feature contributes must exist before jsvm.Register executes
	// the org's hook files synchronously. Feature DAV mounts (which declare
	// their TS hook points) self-register here too, keeping that ordering.
	//
	// Features register through their single Register entry; the TenantContext
	// is stamped before this runs, so a package that must differ hosted (mail
	// listeners) detects it via GetTenantContext.
	RegisterExtras func(app *pocketbase.PocketBase)

	// MailListeners are the router-managed mail sockets (zero value = none),
	// surfaced to feature packages via the TenantContext. The router owns
	// every listening socket; a tenant must never bind a port.
	MailListeners MailListeners

	// QuotaSources come from each package's manifest `quota` block via the
	// router's materialized quota.json. QuotaLimits must resolve the org
	// ceiling from the ROUTER's runtime config (quota.FixedLimits), never
	// from the org's own settings — its superusers must not be able to raise
	// the plan they were sold. Empty sources or a nil LimitsFunc disable
	// enforcement, same as the single-org app.
	QuotaSources []quota.Source
	QuotaLimits  quota.LimitsFunc
}

// RegisterTenant configures a multi-org TENANT process with the same core
// server behavior as the single-org app, minus what is genuinely host-only.
//
// This is the tenant-shaped counterpart of Register, and the two must be read
// together: everything shared lives in registerSharedEarly/registerSharedCore
// (single source of truth), the host-only remainder is enumerated — each with
// a reason — in Register's tail, and composition_parity_test.go fails if the
// two compositions diverge anywhere without a recorded reason. This exists
// because the previous arrangement (serve-org hand-rolling a subset) silently
// dropped every guard core added, including the users field guard whose
// absence let any member promote themselves to owner; see
// multi-org/docs/FINDING-tenant-composition-gap.md.
//
// Differences from Register, all deliberate:
//   - No CLI flags, no migratecmd: the router owns deploys and the tenant is
//     never driven from a terminal.
//   - jsvm registers with Register (not MustRegister) so a hook that throws
//     at load fails this one org cleanly instead of panicking, with
//     HooksWatch off (a watcher's app.Restart() would execve the process out
//     from under the router's supervision) and Sandboxed on (defence in
//     depth inside the OS boundary).
//   - Protocol servers come from materialized config (TenantOptions sources)
//     instead of feature Go. Feature packages DO link into the tenant via
//     TenantOptions.RegisterExtras (the router's pinned menu, gated by the
//     org's package set), but only through their RegisterTenant entries,
//     which exclude DAV mounts and port listeners — the router owns every
//     listening socket, and the materialized lists stay authoritative for
//     what a tenant serves.
//
// Call before app.Bootstrap(). Run apis.Serve (with the router-provided
// listener) after.
func RegisterTenant(app *pocketbase.PocketBase, opts TenantOptions) error {
	// The tenant context must exist before ANY feature Register runs — it is
	// how a package detects tenancy under the single-Register contract (mail
	// picks injected listeners over binding ports).
	setTenantContext(app, TenantContext{
		Slug:          opts.Slug,
		Mail:          opts.MailListeners,
		ControlSocket: opts.ControlSocket,
	})

	registerSharedEarly(app)

	// Feature package Go — the artifact links exactly the org's package set.
	// Mirrors the host composition's RegisterExtras placement: before quota
	// (feature record hooks must precede the enforcement hook) and before
	// jsvm (their `$`-bindings, loader bindings, and DAV TS hook points must
	// exist when the hook files run). Feature DAV servers mount HERE, from
	// each package's own Register — the same call in both compositions; the
	// router-materialized DAV lists are no longer consumed by this core
	// version (older artifact binaries still read theirs).
	if opts.RegisterExtras != nil {
		opts.RegisterExtras(app)
	}

	// Storage ceilings. Bound by core so every write path in the tenant is
	// covered even though no feature package need be linked here.
	if err := quota.Register(app, opts.QuotaSources, opts.QuotaLimits); err != nil {
		return fmt.Errorf("quota register: %w", err)
	}

	if err := jsvm.Register(app, jsvm.Config{
		HooksDir:      opts.HooksDir,
		MigrationsDir: opts.MigrationsDir,
		HooksWatch:    false,
		HooksPoolSize: opts.HooksPoolSize,
		Sandboxed:     true,
		// The same binding seams the app installs: `$`-bindings on every VM,
		// loader-only bindings (webdavHook/caldavHook registration) once.
		// Binders come from core sub-packages plus whatever feature packages
		// RegisterExtras registered above (the org's enabled slice of the
		// router's pinned menu).
		OnInit:       buildJsvmOnInit(app),
		OnLoaderInit: buildJsvmOnLoaderInit(app),
	}); err != nil {
		return fmt.Errorf("jsvm register: %w", err)
	}

	registerSharedCore(app)

	// The org's own web bundle: the artifact's staged release, materialized at
	// <orgDir>/pb_public (app.html shell, _expo/static assets, public files).
	// Served by the TENANT — the router only reverse-proxies — with the same
	// handler the single-org app uses in dev (public dir + app.html SPA
	// fallback with public-config injection), minus the host-only website and
	// cross-release asset-pool machinery, which a per-org build has no
	// counterpart for: its pb_public IS the complete, self-contained release.
	if opts.OrgDir != "" {
		registerTenantStaticServe(app, filepath.Join(opts.OrgDir, "pb_public"))
	}

	// Artifact-booted tenants reconcile their durable package state at boot
	// (after RunAllMigrations — apis.Serve orders that before OnServe): the
	// deploy-result consume and the pkg_registry mirror of the built-in set.
	// See tenant_pkg_state.go.
	if opts.OrgDir != "" && opts.ArtifactDir != "" {
		orgDir, artifactDir := opts.OrgDir, opts.ArtifactDir
		app.OnServe().BindFunc(func(e *core.ServeEvent) error {
			reconcileTenantPackageState(e.App, orgDir, artifactDir)
			return e.Next()
		})

		// Per-org native OTA: serve this org's own bundles from its artifact.
		// The host reads a pkg_build row + build archive, neither of which an
		// org dir has; see app_updates_tenant.go.
		RegisterTenantAppUpdateEndpoints(app, orgDir, artifactDir)

		// The hosted Packages UI needs both the built-in set (above) and a
		// deploy channel; without a control socket the tenant has no way to
		// propose, so the endpoints are simply absent (as they were for every
		// tenant before step 4).
		if opts.ControlSocket != "" {
			RegisterHostedPackageEndpoints(app, NewDeployChannel(opts.ControlSocket), orgDir, artifactDir)
		}
	}
	return nil
}

// registerTenantStaticServe mounts the SPA catch-all for a tenant, mirroring
// the host's registerStaticServe registration shape (Priority 999 so every
// API route binds first; HasRoute so an explicit catch-all someone registered
// earlier wins). StaticWithDynamicFallback with no website and no releases dir
// is exactly the tenant's serving story: static files from the org's
// materialized pb_public, the /api/ JSON-404 guard, and the app.html fallback
// with public-config injection + ETag revalidation.
func registerTenantStaticServe(app core.App, publicDir string) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			if !e.Router.HasRoute(http.MethodGet, "/{path...}") {
				e.Router.Any("/{path...}", StaticWithDynamicFallback(publicDir, "", ""))
			}
			return e.Next()
		},
		Priority: 999,
	})
}
