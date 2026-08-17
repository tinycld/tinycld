package coreserver

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/hook"

	"tinycld.org/core/automation"
	"tinycld.org/core/logging"
	"tinycld.org/core/notify"
	"tinycld.org/core/oauth"
	"tinycld.org/core/offboard"
	"tinycld.org/core/pkgaccess"
	"tinycld.org/core/quota"
	"tinycld.org/core/realtime"
	"tinycld.org/core/search"
	"tinycld.org/core/sharelink"
)

// Options configure the core server's registered plugins, flags, and wiring.
// A runnable `main` package builds this struct and calls Register(app, opts).
type Options struct {
	// HTTP server
	PublicDir    string
	FallbackFile string

	// WebsiteDir is the marketing website's static root, served BEFORE PublicDir
	// and the SPA fallback. Kept separate from PublicDir so the app's web export
	// (which bundles PublicDir) can't absorb the site. Empty disables it (dev /
	// com mode); org-mode deploys point it at the built Astro site.
	WebsiteDir string

	// ReleasesDir is the directory containing per-deploy web bundle state.
	// On a deployed image it lives on the persistent volume and contains:
	//   - <id>/             one dir per retained release, holding app.html
	//                       + release-id.txt
	//   - current           symlink → <id> for the active release
	//   - _static/          cross-release asset pool (filled by entrypoint)
	// Used by the asset-pool handlers, /api/version, and the SPA fallback.
	// When empty or missing on disk the server falls back to the legacy
	// single-PublicDir behavior used in dev.
	ReleasesDir string

	// Schema generation
	TypesDir string

	// JS hooks / migrations plugins
	HooksDir      string
	HooksWatch    bool
	HooksPoolSize int
	MigrationsDir string
	Automigrate   bool

	// BinaryName is the file name of the running app binary (without
	// directory). Used by package install/upgrade flows to locate the
	// existing binary for migrate-and-swap operations. Defaults to
	// `filepath.Base(os.Args[0])` when empty.
	BinaryName string

	// RegisterExtras is called after core's own registrations. Use it to inject
	// code that lives outside this package — in particular, the generator's
	// `registerPackageExtensions(app)` which wires sibling package servers.
	RegisterExtras func(app *pocketbase.PocketBase)

	// QuotaSources are the storage-bearing collections whose bytes count
	// toward the deployment's ceilings, materialized from each package's
	// manifest `quota` block.
	//
	// Core binds the enforcement hooks rather than the feature, so the limit
	// holds on every write path and in a tenant process (which links no
	// feature package). Empty disables enforcement.
	QuotaSources []quota.Source

	// QuotaLimits resolves the ceilings. Defaults to quota.SettingsLimits,
	// which reads the `settings` collection — correct for a single-org
	// deployment. A multi-org tenant passes a resolver whose org ceiling comes
	// from the router's runtime config instead, so the org cannot raise the
	// plan limit it was sold.
	QuotaLimits quota.LimitsFunc
}

// binaryName holds the resolved app binary name (set by Register()).
// Package-internal helpers in pkg_install.go and pkg_go_build.go read it to
// locate the running binary on disk. Default `tinycld` matches the original
// app; production callers should set Options.BinaryName.
var binaryName = "tinycld"

// Register configures a pocketbase.PocketBase app with all of core's
// server-side behavior: jsvm, migratecmd, notifications, invites, audit,
// package management, setup bootstrap, account deletion, schema generation,
// and static file serving.
//
// Callers provide the `app` (so they can build it with their own Sentry/env
// setup) and Options. Run `app.Start()` after this returns.
func Register(app *pocketbase.PocketBase, opts Options) {
	if opts.BinaryName != "" {
		binaryName = opts.BinaryName
	} else if len(os.Args) > 0 {
		// Fall back to the running executable's basename. Strips ".test"
		// in test contexts so package-install code paths still see a
		// production-shaped name (purely cosmetic).
		base := filepath.Base(os.Args[0])
		if base != "" && base != "." && base != "/" {
			binaryName = base
		}
	}

	registerFlags(app, &opts)

	// Parse CLI args so flag-backed fields are populated before we register
	// plugins that read them (jsvm, migratecmd).
	_ = app.RootCmd.ParseFlags(os.Args[1:])

	registerSharedEarly(app)

	// Install the process-wide logger before features register, so their
	// package loggers and any boot-time logging reach every destination.
	//
	// app.Logger().Handler() is resolved HERE and handed to Install, never
	// resolved inside the fan-out: app.Logger() falls back to slog.Default()
	// pre-bootstrap, so a lazy lookup inside the default handler would recurse.
	logging.Install(app.Logger().Handler())

	// Feature packages register BEFORE jsvm, and the order is load-bearing:
	// jsvm.Register executes the hook files synchronously (its registerHooks
	// call, not deferred to OnBootstrap), so any `$`-binding or loader binding
	// a package contributes has to exist by then. Registering features after
	// this point would leave their hook files calling undefined globals —
	// `webdavHook is not defined` at boot.
	//
	// Nothing here mounts routes directly; features bind OnServe, which fires
	// later, so moving this earlier does not change route ordering.
	if opts.RegisterExtras != nil {
		opts.RegisterExtras(app)
	}

	// Storage ceilings. Bound by core, not by the feature that owns the data,
	// so no write path and no host can skip them.
	quotaLimits := opts.QuotaLimits
	if quotaLimits == nil {
		quotaLimits = quota.SettingsLimits
	}
	// Explicit sources win; otherwise take whatever the installed packages
	// declared during RegisterExtras above.
	quotaSources := opts.QuotaSources
	if len(quotaSources) == 0 {
		quotaSources = quota.RegisteredSources()
	}
	if err := quota.Register(app, quotaSources, quotaLimits); err != nil {
		log.Fatalf("coreserver: quota config: %v", err)
	}

	jsvm.MustRegister(app, jsvm.Config{
		MigrationsDir: opts.MigrationsDir,
		HooksDir:      opts.HooksDir,
		HooksWatch:    opts.HooksWatch,
		HooksPoolSize: opts.HooksPoolSize,
		// Install core's native $-bindings on every VM (hook + callback pools).
		// Binders are registered by core sub-packages (fts, carddav, …) and by
		// feature packages via RegisterJSVMBinder; see jsvm_binds.go.
		OnInit: buildJsvmOnInit(app),
		// Install the loader-only bindings that REGISTER package TS handlers
		// against a Go→TS hook point (e.g. webdavHook). These must run once,
		// not once per pooled VM, so they ride OnLoaderInit rather than
		// OnInit; see ts_hooks.go.
		OnLoaderInit: buildJsvmOnLoaderInit(app),
	})

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		TemplateLang: migratecmd.TemplateLangJS,
		Automigrate:  opts.Automigrate,
		Dir:          opts.MigrationsDir,
	})

	registerSharedCore(app)

	// ---- Host-only registrations ----
	// Everything below runs ONLY in the single-org app, never in a multi-org
	// tenant. Each entry needs a reason a tenant must not have it, and a
	// matching entry in the hostOnlyHookDiff allowlist in
	// composition_parity_test.go — the parity test fails on an unexplained
	// divergence between the two compositions.

	// Package install/upgrade swaps the running binary and invokes the Go
	// toolchain. In multi-org the ROUTER owns deploys (re-materialize + evict);
	// a tenant that could rebuild or restart itself would escape the router's
	// supervision.
	RegisterPackageInstallEndpoints(app)
	// Admin-console panel endpoint. Tenant push keys will arrive via the org's
	// system_settings (control-plane provisioned), not a self-serve panel.
	RegisterVapidAdminEndpoints(app)
	// OTA bundles are served from the host's build archive; an org dir has no
	// build archive. Hosted-app OTA delivery is a separate open question.
	RegisterAppUpdateEndpoints(app)
	// CLI binaries are served from the host's own activated build dir
	// (cli-dist/ next to the server binary); a tenant serves its artifact's
	// copy via RegisterTenantCliDownloadEndpoints.
	RegisterCliDownloadEndpoints(app)
	// First-run installer + TINYCLD_PUBLIC_URL sync. A tenant needs neither:
	// the router provisions orgs (migrations apply inside the tenant's own
	// first spawn), serve-org nils the installer, and tenant AppURL comes from
	// the router-materialized .runtime/app.json.
	RegisterSetupBootstrap(app)
	// Marketing-site demo machinery (shared demo user, nightly reset cron).
	// Demos run on the single-org deployment, never inside a customer org.
	RegisterDemoStart(app)
	RegisterDemoLead(app)
	RegisterDemoReset(app)

	// Regenerates TypeScript schema files into the developer workspace on
	// collection edits. A tenant has no workspace and no TypesDir.
	registerSchemaHooks(app, opts.TypesDir)

	// Feature Go (CardDAV, full-text search, audit registration) is contributed by
	// each package's own server module via Options.RegisterExtras — not wired here.
	// Core provides the reusable helpers those servers call (tinycld.org/core/audit,
	// the RegisterJSVMBinder/$-binding seam); it links no feature package itself.

	// Static/SPA serving is host-shaped (releases dir, asset pool, website).
	// Tenant static serving is separate open work: the router materializes
	// pb_public but nothing serves it yet.
	registerStaticServe(app, opts)
}

// registerSharedEarly holds the registrations that must precede everything
// else that binds OnServe, in BOTH compositions: Sentry's router middleware
// only applies to routes registered after it, and SystemConfig must be
// loading before any subsystem that reads it (mailer, push, Sentry init).
//
// SHARED COMPOSITION CONTRACT: this function and registerSharedCore are the
// single source of truth for what the single-org app (Register) and a
// multi-org tenant process (RegisterTenant) both run. A new registration goes
// in one of them unless it genuinely must not run in a tenant — in which case
// it goes in Register's host-only tail WITH a reason comment and an entry in
// composition_parity_test.go's allowlist. See
// multi-org/docs/FINDING-tenant-composition-gap.md for what silent divergence
// cost.
func registerSharedEarly(app *pocketbase.PocketBase) {
	// Sentry must register first so its router middleware sees every route.
	// Middleware bound after a route is added does not apply retroactively.
	// The client only initializes when a DSN exists in system_settings, so in
	// an unconfigured tenant this is an inert pass-through.
	RegisterSentry(app)

	// System-wide settings (Sentry/web-push/mail creds). Loads the
	// system_settings collection into the in-memory SystemConfig once the DB is
	// ready and keeps it in sync on edits, so subsystems read current values
	// without env vars and stateful ones (Sentry) re-init on change. In a
	// tenant this reads the org's own DB, so creds are per-org.
	RegisterSystemConfig(app)
}

// registerSharedCore is the bulk of the shared composition: everything both
// the single-org app and a multi-org tenant register after their respective
// engine plugins (quota, jsvm) are in place. See the contract note on
// registerSharedEarly. Order within this list is preserved from the original
// single-org composition; the users hooks in particular bind in
// guard → demo-audit → disabled order.
func registerSharedCore(app *pocketbase.PocketBase) {
	notify.Register(app)
	notify.RegisterCommentMentionHooks(app)
	// Teach the realtime broker how to verify anonymous share-session
	// tokens so calc/text editable share links can open a WS without a
	// PB account. Adapts sharelink.VerifySession to the broker's
	// ShareClaims shape (the broker stays decoupled from drive's feature).
	realtime.SetShareSessionResolver(func(a core.App, token string) (realtime.ShareClaims, error) {
		claims, err := sharelink.VerifySession(a, token)
		if err != nil {
			return realtime.ShareClaims{}, err
		}
		return realtime.ShareClaims{
			ShareToken:  claims.ShareToken,
			AnonID:      claims.AnonID,
			DisplayName: claims.DisplayName,
			Role:        claims.Role,
			ItemID:      claims.ItemID,
		}, nil
	})
	realtime.Register(app, realtime.Options{})
	RegisterInviteEndpoint(app)
	RegisterInviteLinkEndpoints(app)
	RegisterOrgInfoEndpoint(app)
	RegisterPasswordResetMailer(app)
	RegisterAuditHooks(app)
	RegisterAccountDelete(app)
	RegisterAdminOffboard(app)
	offboard.Register(app)
	RegisterUsersFieldGuard(app)
	RegisterLastOwnerGuard(app)
	RegisterUsersDemoAuditHook(app)
	RegisterDisabledUserGuard(app)
	pkgaccess.Register(app)

	// OAuth 2.1 authorization server: the device grant the tinycld CLI logs in
	// with, and authorization-code + PKCE for third-party integrations. Shared
	// (not host-only) because a multi-org tenant must be able to authorize a
	// CLI or an integration exactly like a self-hosted deployment.
	oauth.Register(app)

	// Federated search over every installed package. Shared for the same reason
	// as OAuth above: a tenant's packages are searchable exactly as a
	// self-hosted deployment's are. Sources register themselves from their own
	// package Register, so with no packages linked this serves an empty result
	// rather than failing.
	search.Register(app)

	// Workflow rules engine: record-hook + scheduled dispatch, manual-run and
	// dry-run endpoints. Shared for the same reason as OAuth/search above — a
	// tenant's rules fire exactly as a self-hosted deployment's do. Inert with
	// no materialized defs (a workspace with no automation-contributing
	// packages), so this is a no-op call in that case.
	automation.Register(app, automation.Options{
		DefsPath: filepath.Join(resolveServerDir(), "automation_defs.json"),
	})

	// Keep the /carddav (and /caldav, /dav) CORS bypass here even though core no
	// longer serves a protocol handler itself: a package's own Go server (e.g.
	// contacts) mounts /carddav via OnServe and relies on this bypass for
	// non-browser DAV clients. A tenant mounts the same protocols from
	// materialized config, so it needs the bypass for the same reason.
	registerDavCorsBypass(app)
}

// registerDavCorsBypass wraps PocketBase's default CORS middleware so that
// requests under /caldav, /carddav, and /dav skip CORS entirely.
//
// Why: the default middleware always returns 204 for OPTIONS requests
// (including non-browser DAV clients like macOS Finder that send no Origin
// header). The 204 has no DAV: or Allow: headers, so Finder concludes the
// endpoint is not a DAV server and aborts with a generic "problem connecting
// to the server" dialog. CORS is irrelevant for these protocols — clients
// are not browsers — so we let DAV requests bypass CORS and reach the
// underlying handler, which sets the correct DAV class advertisement.
func registerDavCorsBypass(app *pocketbase.PocketBase) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		for _, mw := range e.Router.Middlewares {
			if mw.Id != apis.DefaultCorsMiddlewareId {
				continue
			}
			original := mw.Func
			mw.Func = func(re *core.RequestEvent) error {
				if isDavPath(re.Request.URL.Path) {
					return re.Next()
				}
				return original(re)
			}
			break
		}
		return e.Next()
	})
}

// isDavPath reports whether a request belongs to a DAV protocol mount rather
// than the SPA.
//
// `/dav` is the RESERVED namespace for protocol mounts; no package slug may
// claim it. This used to list bare "/drive", which is also the in-app route:
// once the single-org migration dropped the /a/<orgSlug> segment the two
// collided, and since a literal route beats the SPA catch-all, a hard load of
// /drive reached Basic-Auth WebDAV instead of the app.
func isDavPath(path string) bool {
	return strings.HasPrefix(path, "/caldav") ||
		strings.HasPrefix(path, "/carddav") ||
		strings.HasPrefix(path, "/dav") ||
		strings.HasPrefix(path, "/.well-known/caldav") ||
		strings.HasPrefix(path, "/.well-known/carddav") ||
		strings.HasPrefix(path, "/.well-known/webdav")
}

// registerFlags binds the persistent CLI flags shared by every tinycld server.
// Option fields are used as defaults; the user can override via --flag.
func registerFlags(app *pocketbase.PocketBase, opts *Options) {
	f := app.RootCmd.PersistentFlags()
	f.StringVar(&opts.HooksDir, "hooksDir", opts.HooksDir, "the directory with the JS app hooks")
	f.BoolVar(&opts.HooksWatch, "hooksWatch", opts.HooksWatch,
		"auto restart the app on pb_hooks file change; it has no effect on Windows")
	f.IntVar(&opts.HooksPoolSize, "hooksPool", opts.HooksPoolSize,
		"the total prewarm goja.Runtime instances for the JS app hooks execution")
	f.StringVar(&opts.MigrationsDir, "migrationsDir", opts.MigrationsDir,
		"the directory with the user defined migrations")
	f.BoolVar(&opts.Automigrate, "automigrate", opts.Automigrate, "enable/disable auto migrations")
	f.StringVar(&opts.PublicDir, "publicDir", opts.PublicDir, "the directory to serve static files")
	f.StringVar(&opts.WebsiteDir, "websiteDir", opts.WebsiteDir,
		"the directory with the marketing website, served before publicDir (empty disables)")
	f.StringVar(&opts.FallbackFile, "fallbackFile", opts.FallbackFile,
		"fallback to this file on missing static path for SPA routes")
	f.StringVar(&opts.ReleasesDir, "releasesDir", opts.ReleasesDir,
		"the directory containing per-release web bundle subdirectories")
	f.StringVar(&opts.TypesDir, "typesDir", opts.TypesDir,
		"the directory to write generated TypeScript schema files")
}

func registerSchemaHooks(app *pocketbase.PocketBase, typesDir string) {
	app.OnCollectionCreateRequest().BindFunc(func(e *core.CollectionRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		GenerateSchemas(e.App, typesDir)
		return nil
	})

	app.OnCollectionUpdateRequest().BindFunc(func(e *core.CollectionRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		GenerateSchemas(e.App, typesDir)
		return nil
	})

	app.OnCollectionDeleteRequest().BindFunc(func(e *core.CollectionRequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		GenerateSchemas(e.App, typesDir)
		return nil
	})
}

func registerStaticServe(app *pocketbase.PocketBase, opts Options) {
	app.OnServe().Bind(&hook.Handler[*core.ServeEvent]{
		Func: func(e *core.ServeEvent) error {
			GenerateSchemas(e.App, opts.TypesDir)
			SyncBundledPackages(e.App)
			SeedBaseBuild(e.App)
			ReconcileRolledBackInstall(e.App)

			// Per-route asset handlers, registered before the catch-all so
			// the asset prefixes win. Both paths read from the cross-release
			// asset pool the entrypoint maintains under
			// <releasesDir>/_static/. _expo/static/ filenames are fully
			// content-hashed (immutable), while /assets/ contains a few
			// stable names like app-icon.png so a shorter max-age applies.
			if opts.ReleasesDir != "" {
				e.Router.GET("/_expo/static/{path...}", PoolAssets(opts.ReleasesDir, "_expo/static", "public, max-age=31536000, immutable"))
				e.Router.GET("/assets/{path...}", PoolAssets(opts.ReleasesDir, "assets", "public, max-age=300"))
				e.Router.GET("/api/version", VersionHandler(opts.ReleasesDir))
				e.Router.GET("/api/release", ReleaseHandler(opts.ReleasesDir))
			}

			if !e.Router.HasRoute(http.MethodGet, "/{path...}") {
				if opts.ReleasesDir != "" {
					e.Router.Any("/{path...}", StaticWithDynamicFallback(opts.PublicDir, opts.WebsiteDir, opts.ReleasesDir))
				} else {
					e.Router.Any("/{path...}", StaticWithFallback(opts.PublicDir, opts.FallbackFile))
				}
			}

			return e.Next()
		},
		Priority: 999,
	})
}
