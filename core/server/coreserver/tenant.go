package coreserver

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"

	"tinycld.org/core/caldav"
	"tinycld.org/core/carddav"
	"tinycld.org/core/quota"
	"tinycld.org/core/webdav"
)

// TenantOptions configure RegisterTenant. Everything here is materialized or
// resolved by the multi-org router and handed to the tenant process — a
// tenant never reads deployment-wide flags or env for these.
type TenantOptions struct {
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
	// serve-org passes the router's pinned package menu here, gated by the
	// org's resolved package set (multi-org/docs/SCOPE-tenant-feature-go.md).
	//
	// It runs immediately after registerSharedEarly and BEFORE quota and jsvm,
	// for the same load-bearing reasons as the host composition: a feature's
	// record hooks must bind before quota's enforcement hook (drive corrects a
	// forged size the quota check then reads), and every `$`-binding or loader
	// binding a feature contributes must exist before jsvm.Register executes
	// the org's hook files synchronously.
	//
	// Features register through their RegisterTenant entry (shared set only —
	// no port listeners, no DAV mounts); the DAV protocol servers keep coming
	// from the materialized source lists below.
	RegisterExtras func(app *pocketbase.PocketBase)

	// QuotaSources come from each package's manifest `quota` block via the
	// router's materialized quota.json. QuotaLimits must resolve the org
	// ceiling from the ROUTER's runtime config (quota.FixedLimits), never
	// from the org's own settings — its superusers must not be able to raise
	// the plan they were sold. Empty sources or a nil LimitsFunc disable
	// enforcement, same as the single-org app.
	QuotaSources []quota.Source
	QuotaLimits  quota.LimitsFunc

	// DAV sources, materialized by the router from each package's manifest.
	// Registered here rather than by the caller because ordering is
	// load-bearing: their TS hook points must be declared BEFORE
	// jsvm.Register executes the org's hook files, exactly like
	// Options.RegisterExtras in the single-org composition. A caller mounting
	// them itself after jsvm would silently lose every `webdavHook` /
	// `caldavHook` handler the org's TS registers.
	WebDAVSources  []webdav.Source
	CalDAVSources  []caldav.Source
	CardDAVSources []carddav.Source
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
	registerSharedEarly(app)

	// Feature package Go, gated by the org's resolved package set. Mirrors
	// the host composition's RegisterExtras placement: before quota (feature
	// record hooks must precede the enforcement hook) and before jsvm (their
	// `$`-bindings and loader bindings must exist when the hook files run).
	if opts.RegisterExtras != nil {
		opts.RegisterExtras(app)
	}

	// Protocol servers driven by materialized config. Same mounting, auth,
	// and TS hook seams as the single-org app — webdav.Register /
	// caldav.Register are exactly what drive's and calendar's Go call there.
	// Features' own DAV mounts are host-only (their RegisterTenant entries
	// skip them), so the materialized lists stay the single source of truth
	// for what a tenant serves.
	if _, err := webdav.Register(app, opts.WebDAVSources, WebDAVHostBindings()); err != nil {
		return fmt.Errorf("webdav register: %w", err)
	}
	caldav.Register(app, opts.CalDAVSources, CalDAVHostBindings())
	carddav.Register(app, opts.CardDAVSources)

	// Storage ceilings. Bound by core so every write path in the tenant is
	// covered even though no feature package is linked here.
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
