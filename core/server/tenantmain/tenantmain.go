// Package tenantmain is the tenant-mode entry point: it runs ONE
// organization's PocketBase app in its own OS process, serving on a unix
// socket handed down by a hosting router.
//
// This is the tenant side of per-process isolation. The process is confined
// by its parent (per-tenant uid, mount and PID namespaces, cgroups on Linux)
// so that a hostile tenant's JS cannot reach another org's data even though
// it holds a full $app/DB surface — a boundary in-process allowlisting
// provably could not provide.
//
// The tenant is composed by coreserver.RegisterTenant — the same composition
// root as the single-org app, minus the host-only pieces. This package owns
// only what is genuinely tenant-transport: runtime-config loading (the
// tenantcfg ABI), the unix socket, the readiness handshake, confinement, and
// shutdown. Do NOT register core behavior here; add it to coreserver's shared
// composition so both the app and every tenant get it.
//
// It lives in CORE — not in the router repo — because the dual-mode binary a
// per-org build produces must be able to run as a tenant, and that binary is
// built from the org's own fetched workspace, pinned to its own core version
// (DESIGN-org-package-agency.md D5). The app shell's tenant mode composes its
// feature registration on top of this via Options.RegisterExtras.
//
// The architectural rule about ports still holds: a service that must BIND A
// PORT moves into core so the router can open it. A tenant serves on unix
// sockets the router hands down and the router owns every listening socket,
// so a feature cannot bring its own listener — mail's IMAP/SMTP listeners
// arrive as Extras.Mail ListenFuncs over router-managed socket paths, and
// CardDAV/CalDAV/WebDAV stay core libraries driven by the declarative source
// lists the host materialized.
package tenantmain

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/coreserver"
	"tinycld.org/core/logging"
	"tinycld.org/core/mailproto"
	"tinycld.org/core/orgcookie"
	"tinycld.org/core/quota"
	"tinycld.org/core/tenantcfg"
)

// log is tenantmain's package-wide structured logger. No file in this
// package imports stdlib "log" any longer, so the short identifier is free.
var log = logging.ForPackage("tenantmain")

// Options composes a tenant on top of the transport.
type Options struct {
	// Args are the command-line args to parse (without the program name /
	// mode selector). Nil means os.Args[1:].
	Args []string

	// RegisterExtras registers the tenant's feature Go — the SAME generated
	// registrar the host composition uses (single-Register contract): a
	// per-org build links exactly its package set, the artifact is the gate,
	// and per-org facts (slug, mail listeners, control socket) reach feature
	// packages through coreserver's TenantContext, stamped before this runs.
	RegisterExtras func(app *pocketbase.PocketBase)

	// QuotaLimits overrides how the tenant resolves its storage ceilings.
	// Nil (the default) derives them from .runtime/quota.json, read once at
	// boot. A composition that must reflect limit changes WITHOUT a respawn
	// supplies its own resolver here; it is called per write, so it must be
	// cheap (a cached struct read, not a query).
	QuotaLimits quota.LimitsFunc

	// QuotaSources overrides the storage-bearing collections the ceiling is
	// computed over. Nil derives them from .runtime/quota.json, which is
	// correct for every router-spawned tenant.
	QuotaSources []quota.Source

	// RegisterRuntime lets a composition register behavior that needs BOTH the
	// app and the runtime-config paths this package parsed from the command
	// line. Neither is available to the caller before Run: the app does not
	// exist yet, and re-parsing os.Args to recover the paths would duplicate
	// this flagset and drift from it.
	//
	// Distinct from RegisterExtras, which registers feature Go inside
	// RegisterTenant and cannot fail; this runs earlier, receives the parsed
	// runtime paths, and may abort the boot. The two are independent — a
	// composition may set either, both, or neither.
	//
	// It runs after the app is constructed and before coreserver.RegisterTenant,
	// which is the only window that works. Registering later would miss the
	// composition; the returned LimitsFunc has to exist by then because
	// quota.Register binds NO hooks at all when its LimitsFunc is nil, so a
	// resolver supplied afterwards would leave enforcement silently off.
	//
	// A non-nil returned LimitsFunc takes precedence over Options.QuotaLimits;
	// returning nil leaves that field's resolution untouched. An error aborts
	// the boot and is reported on --ready-fd, so a composition whose limits
	// failed to register never comes up claiming to be healthy.
	//
	// It is called on the goroutine that runs Run, before any socket this
	// package binds, so an implementation may use process-wide state such as
	// syscall.Umask without racing core's own binds.
	RegisterRuntime func(app *pocketbase.PocketBase, cfg RuntimeConfig) (quota.LimitsFunc, error)
}

// RuntimeConfig carries the router-materialized runtime paths that this package
// parses but does not itself consume, so a composition layered on top can reach
// them without re-parsing the command line.
type RuntimeConfig struct {
	// OrgDir is the org's data directory (--org-dir).
	OrgDir string

	// LimitsConfig is the path the router materialized the org's limits.json to
	// (--limits-config). Empty means no router materialized one, which a
	// consumer should treat as "unconstrained", not as an error — a standalone
	// boot has no hosting runtime behind it.
	LimitsConfig string

	// ConfigSocket is the unix socket the router pushes live runtime-config
	// updates on (--cfg-socket). Empty means no push channel: whatever the
	// files carried at spawn stands until the next spawn.
	ConfigSocket string

	// QuotaConfig is the path the router materialized the org's quota.json to
	// (--quota-config). This package already loads it for its own enforcement
	// (quotaCfg in run()); it is handed here too so a composition layered on
	// top — e.g. one that reports current usage — can read the same source
	// list via tenantcfg.LoadQuota + tenantcfg.DecodeQuota without re-parsing
	// the command line. Empty means no router materialized one.
	QuotaConfig string
}

// Run parses tenant flags, composes the org's app, and serves until
// signalled. Readiness (or the failure reason) is reported on --ready-fd; a
// returned error has already been reported there.
func Run(opts Options) error {
	args := opts.Args
	if args == nil {
		args = os.Args[1:]
	}
	fs := flag.NewFlagSet("tenant", flag.ExitOnError)
	var (
		orgDir       = fs.String("org-dir", "", "org directory containing pb_data, pb_hooks, pb_migrations")
		socketPath   = fs.String("socket", "", "unix socket to create and serve on")
		readyFD      = fs.Int("ready-fd", 0, "file descriptor to report readiness on")
		slug         = fs.String("slug", "", "org slug (identification and logging only)")
		hooksPool    = fs.Int("hooks-pool", 15, "jsvm hook runtime pool size")
		webdavConfig = fs.String("webdav-config", "", "path to the org's materialized webdav.json")
		caldavConfig = fs.String("caldav-config", "", "path to the org's materialized caldav.json")
		quotaConfig  = fs.String("quota-config", "", "path to the org's materialized quota.json")
		limitsConfig = fs.String("limits-config", "", "path to the org's materialized limits.json")
		appConfig    = fs.String("app-config", "", "path to the org's materialized app.json (public URL + proxy trust)")
		davConfig    = fs.String("carddav-config", "", "path to the CardDAV source JSON")
		imapSocket   = fs.String("imap-socket", "", "unix socket to serve IMAP on (router-managed; empty = no IMAP)")
		smtpSocket   = fs.String("smtp-socket", "", "unix socket to serve SMTP submission on (router-managed; empty = no submission)")
		mxSocket     = fs.String("mx-socket", "", "unix socket to serve inbound MX SMTP on (router-managed; empty = no inbound)")
		ctlSocket    = fs.String("control-socket", "", "router-bound unix socket for deploy proposals (empty = no deploy channel)")
		cfgSocket    = fs.String("cfg-socket", "", "unix socket to serve the router's live config pushes on (empty = no push channel)")
		drain        = fs.Duration("drain", 10*time.Second, "graceful shutdown budget")
		confinePkg   = fs.String("confine-packages", "", "remount this dir read-only in our mount namespace")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *orgDir == "" || *socketPath == "" {
		return fmt.Errorf("tenant mode: --org-dir and --socket are required")
	}

	// Readiness is reported on a pipe the parent holds the read end of.
	// Report failures through it too, so the parent gets a reason instead of
	// inferring one from an exit code.
	var ready *os.File
	if *readyFD > 0 {
		ready = os.NewFile(uintptr(*readyFD), "ready")
	}

	cfg := runConfig{
		orgDir:     *orgDir,
		socketPath: *socketPath,
		slug:       *slug,
		hooksPool:  *hooksPool,
		configs: configPaths{
			carddav: *davConfig,
			caldav:  *caldavConfig,
			webdav:  *webdavConfig,
			quota:   *quotaConfig,
			limits:  *limitsConfig,
			app:     *appConfig,
		},
		confinePkg:   *confinePkg,
		mailSocks:    mailSocketPaths{imap: *imapSocket, smtp: *smtpSocket, mx: *mxSocket},
		ctlSocket:    *ctlSocket,
		cfgSocket:    *cfgSocket,
		drain:        *drain,
		ready:        ready,
		extras:       opts.RegisterExtras,
		quotaLimits:  opts.QuotaLimits,
		quotaSources: opts.QuotaSources,
		runtime:      opts.RegisterRuntime,
	}
	if err := run(cfg); err != nil {
		reportNotReady(ready, err)
		return err
	}
	return nil
}

// mailSocketPaths are grouped so the three same-typed paths can't be
// transposed at a call site.
type mailSocketPaths struct {
	imap, smtp, mx string
}

type configPaths struct {
	carddav, caldav, webdav, quota, app string
	// limits is the path to the org's materialized limits.json. This package
	// does not interpret it — the values it carries are the operator's, and
	// core has no opinion on them — it only hands the path to
	// Options.RegisterRuntime so the composition that DOES understand it can
	// read it without re-parsing the command line.
	limits string
}

type runConfig struct {
	orgDir, socketPath, slug string
	hooksPool                int
	configs                  configPaths
	confinePkg               string
	mailSocks                mailSocketPaths
	ctlSocket                string
	// cfgSocket is where the tenant serves the router's live config pushes —
	// the mirror of ctlSocket, which the tenant DIALS. The composition that
	// binds it lives outside this module, same as configs.limits, and reaches it
	// through Options.RegisterRuntime.
	cfgSocket    string
	drain        time.Duration
	ready        *os.File
	extras       func(app *pocketbase.PocketBase)
	quotaLimits  quota.LimitsFunc
	quotaSources []quota.Source
	runtime      func(app *pocketbase.PocketBase, cfg RuntimeConfig) (quota.LimitsFunc, error)
}

func run(cfg runConfig) error {
	// Confinement steps that must happen inside our own mount namespace. Go
	// cannot run code between fork and exec, so the parent creates the
	// namespace and we finish the job here.
	if cfg.confinePkg != "" {
		if err := confinePackages(cfg.confinePkg); err != nil {
			return fmt.Errorf("confine packages: %w", err)
		}
	}

	// The DAV config flags (--carddav-config etc.) are still parsed — the
	// router materializes and passes them for every org, and OLDER artifact
	// binaries consume them — but THIS core version ignores them: under the
	// single-Register contract each feature package mounts its own DAV server
	// in both compositions, and a per-org build contains exactly the org's
	// features, so the artifact is the gate (DESIGN-org-package-agency §5's
	// DAV-materialization deletion candidate, now taken).
	quotaCfg, err := tenantcfg.LoadQuota(cfg.configs.quota)
	if err != nil {
		return fmt.Errorf("load quota config: %w", err)
	}
	appCfg, err := tenantcfg.LoadAppConfig(cfg.configs.app)
	if err != nil {
		return fmt.Errorf("load app config: %w", err)
	}

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir: filepath.Join(cfg.orgDir, "pb_data"),
		// The start banner is derived from Server.Addr, which is meaningless
		// for a unix socket.
		HideStartBanner: true,
		// This process runs the org's package JS, which is untrusted: $app
		// stays bound in a sandboxed VM and reaches raw SQL, so ATTACH DATABASE
		// would be an arbitrary-file read/write primitive covering every other
		// org's data.db. The uid confinement the router applies is the other
		// half of this, and neither is sufficient alone — confinement is absent
		// on developer hosts and when the router runs unprivileged.
		DBConnect: core.NoAttachDBConnect,
	})

	// The layered composition's window: the app exists, and nothing this
	// package binds has been bound yet, so a hook here may touch process-wide
	// state (umask) without racing our socket binds. Its limits resolver must
	// be in hand BEFORE RegisterTenant, because quota.Register binds no hooks
	// at all against a nil LimitsFunc.
	quotaLimits := cfg.quotaLimits
	if cfg.runtime != nil {
		limits, err := cfg.runtime(app, RuntimeConfig{
			OrgDir:       cfg.orgDir,
			LimitsConfig: cfg.configs.limits,
			ConfigSocket: cfg.cfgSocket,
			QuotaConfig:  cfg.configs.quota,
		})
		if err != nil {
			return fmt.Errorf("register runtime: %w", err)
		}
		if limits != nil {
			quotaLimits = limits
		}
	}

	// The full shared core composition: guards, invites, account lifecycle,
	// notify, realtime, audit, quota, sandboxed jsvm with the `$`-binding and
	// hook-point seams, and the DAV protocol servers from the materialized
	// source lists. The org storage ceiling comes from the router's runtime
	// config (FixedLimits), NOT this org's settings — its superusers must not
	// be able to raise a ceiling the operator set for them.
	if err := coreserver.RegisterTenant(app, coreserver.TenantOptions{
		Slug:           cfg.slug,
		HooksDir:       filepath.Join(cfg.orgDir, "pb_hooks"),
		MigrationsDir:  filepath.Join(cfg.orgDir, "pb_migrations"),
		HooksPoolSize:  cfg.hooksPool,
		OrgDir:         cfg.orgDir,
		ArtifactDir:    detectArtifactDir(),
		ControlSocket:  cfg.ctlSocket,
		RegisterExtras: cfg.extras,
		MailListeners:  tenantMailListeners(cfg.mailSocks),
		QuotaSources:   resolveQuotaSources(cfg.quotaSources, quotaCfg.Sources),
		QuotaLimits:    resolveQuotaLimits(quotaLimits, quotaCfg.StorageLimitBytes),
	}); err != nil {
		return fmt.Errorf("register tenant: %w", err)
	}
	// Errors in tenant request handling report to Sentry once the org's
	// system_settings carry a DSN; until then the SDK is a no-op. Flush is
	// also called on the signal path below, since os.Exit skips defers.
	defer sentry.Flush(2 * time.Second)

	// Cross-org switcher cookie: on every successful users auth, upsert this
	// org's {slug, name} into the parent-domain tinycld_orgs cookie so
	// sibling orgs' switcher UIs can offer this one. Requires the router to
	// have materialized the base domain; a standalone tenant (no baseDomain)
	// sets nothing. Navigation hint only — the cookie authorizes nothing, and
	// each org authenticates independently. No URL is stored: the cookie is
	// writable by JS on sibling tenants, so the client derives each org's URL
	// from the slug and its own origin's parent domain (see orgcookie).
	if appCfg.BaseDomain != "" {
		orgName := appCfg.OrgName
		if orgName == "" {
			orgName = cfg.slug
		}
		entry := orgcookie.Entry{Slug: cfg.slug, Name: orgName}
		app.OnRecordAuthRequest("users").BindFunc(func(e *core.RecordAuthRequestEvent) error {
			existing := ""
			if c, err := e.Request.Cookie(orgcookie.Name); err == nil {
				existing = c.Value
			}
			http.SetCookie(e.Response, orgcookie.Cookie(orgcookie.Merge(existing, entry), appCfg.BaseDomain))
			return e.Next()
		})
	}

	if err := app.Bootstrap(); err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	defer app.ResetBootstrapState()

	// Adopt the router-materialized app config into settings. Persisted rather
	// than patched in memory so a settings reload (e.g. a tenant superuser
	// saving unrelated settings) cannot revert it.
	//
	//   - Meta.AppURL: the value PB interpolates into {APP_URL} for
	//     verification / password-reset / email-change links; without it every
	//     auth email carries PB's default http://localhost:8090.
	//   - TrustedProxy.Headers: over the router's unix socket RemoteAddr is
	//     empty, so unless the router's X-Forwarded-For is trusted, RealIP()
	//     resolves every request to the same client and per-IP rate limiting
	//     is one shared bucket. The router guarantees the rightmost entry is
	//     the client, matching PB's default (UseLeftmostIP false).
	settingsDirty := false
	if appCfg.AppURL != "" && app.Settings().Meta.AppURL != appCfg.AppURL {
		app.Settings().Meta.AppURL = appCfg.AppURL
		settingsDirty = true
	}
	// Org branding: the control-plane display_name becomes the tenant's
	// AppName — the value /api/org-info serves to the client (document title,
	// org avatar) and PB interpolates into {APP_NAME} in auth emails. Same
	// guard shape as AppURL: empty means the host manages no name here.
	if appCfg.OrgName != "" && app.Settings().Meta.AppName != appCfg.OrgName {
		app.Settings().Meta.AppName = appCfg.OrgName
		settingsDirty = true
	}
	if len(appCfg.TrustedProxyHeaders) > 0 &&
		!slices.Equal(app.Settings().TrustedProxy.Headers, appCfg.TrustedProxyHeaders) {
		app.Settings().TrustedProxy.Headers = appCfg.TrustedProxyHeaders
		settingsDirty = true
	}
	if settingsDirty {
		if err := app.Save(app.Settings()); err != nil {
			return fmt.Errorf("adopt app config into settings: %w", err)
		}
	}

	listener, err := BindTenantSocket(cfg.socketPath)
	if err != nil {
		return err
	}

	// Captured from the ServeEvent so the signal handler can drain the real
	// server rather than just closing the listener under it.
	var srv *http.Server

	// DAV routes are mounted by RegisterTenant (webdav/caldav/carddav
	// Register bind their own OnServe handlers, same as the single-org app);
	// this handler owns only the tenant transport concerns.
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		// apis.Serve uses this listener verbatim instead of dialing TCP.
		e.Listener = listener
		srv = e.Server
		// The installer builds a URL from Server.Addr and would try to open a
		// browser; meaningless for a headless tenant on a socket.
		e.InstallerFunc = nil

		if err := e.Next(); err != nil {
			return err
		}

		// Routes are bound and the server is about to accept: tell the parent.
		reportReady(cfg.ready)
		return nil
	})

	// Our own signal handling. apis.Serve's graceful shutdown is a hard-coded
	// one second and is wired through pb.Execute(), which we deliberately do
	// not call — Execute would also re-bootstrap and install its own signal
	// handling on top of the parent's supervision.
	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		<-sigs
		// Stop accepting and let in-flight requests finish, bounded by the
		// parent's drain budget. Exits as soon as the last one completes; the
		// parent SIGKILLs us if we overrun.
		ctx, cancel := context.WithTimeout(context.Background(), cfg.drain)
		defer cancel()
		if srv != nil {
			_ = srv.Shutdown(ctx)
		} else {
			_ = listener.Close()
		}
		if err := app.ResetBootstrapState(); err != nil {
			// Leaves bootstrap state stale for the next boot to sort out —
			// same class of "deferred to next boot" condition tenant_pkg_state
			// pages on, so this pages too.
			log.WarnContext(ctx, "reset bootstrap state failed", "err", err)
		}
		// os.Exit skips deferred functions, so flush Sentry here too.
		sentry.Flush(2 * time.Second)
		os.Exit(0)
	}()

	err = apis.Serve(app, apis.ServeConfig{ShowStartBanner: false})
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// detectArtifactDir reports the directory of the build artifact this process
// booted from — its own binary's directory, iff a recipe.json sits beside it
// (the committed-artifact marker, DESIGN-org-package-agency D4). The shared
// serve-org binary and dev builds have none, so they return "" and the
// artifact-only behaviors (package-state reconcile, hosted Packages UI) stay
// off.
func detectArtifactDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	dir := filepath.Dir(exe)
	if _, ok, err := tenantcfg.LoadArtifactRecipe(dir); err != nil || !ok {
		return ""
	}
	return dir
}

// BindTenantSocket binds a unix socket the router handed us — the HTTP socket
// and each mail socket share this contract:
//
//   - bind(2) creates the socket file honouring the umask, so mask it to 0600
//     AT creation instead of chmodding after — the chmod below still runs as a
//     belt, but without the umask there is a window where the socket sits at
//     0755. Process-wide, but socket binds in this process are sequential.
//     (On the primary layout the per-org socket dir is 0700 anyway; the
//     fallback dir and defence-in-depth want this.)
//   - The ROUTER owns the socket file's lifecycle, not us. On Evict-then-Get
//     a replacement binds this same deterministic path while we are still
//     draining, and Go's default unlink-on-close would delete the
//     REPLACEMENT's socket when our listener finally closes — permanently
//     breaking the org. The router clears stale files before each bind and
//     guards its own teardown removal by inode.
//   - Only the router connects to these sockets; the tenant uid owns them.
func BindTenantSocket(path string) (net.Listener, error) {
	oldUmask := syscall.Umask(0o177)
	ln, err := net.Listen("unix", path)
	syscall.Umask(oldUmask)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", path, err)
	}
	if ul, ok := ln.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, fmt.Errorf("chmod socket %s: %w", path, err)
	}
	return ln, nil
}

// tenantMailListeners adapts the router-managed mail socket paths into lazy
// ListenFuncs. Empty path ⇒ nil ListenFunc ⇒ that service is not started.
func tenantMailListeners(socks mailSocketPaths) coreserver.MailListeners {
	mk := func(path string) mailproto.ListenFunc {
		if path == "" {
			return nil
		}
		return func(string) (net.Listener, error) { return BindTenantSocket(path) }
	}
	return coreserver.MailListeners{
		IMAP:       mk(socks.imap),
		Submission: mk(socks.smtp),
		InboundMX:  mk(socks.mx),
	}
}

func reportReady(f *os.File) {
	if f == nil {
		return
	}
	body, _ := json.Marshal(map[string]any{"ok": true, "pid": os.Getpid()})
	_, _ = f.Write(append(body, '\n'))
	_ = f.Close()
}

func reportNotReady(f *os.File, cause error) {
	if f == nil {
		return
	}
	body, _ := json.Marshal(map[string]any{"ok": false, "pid": os.Getpid(), "error": cause.Error()})
	_, _ = f.Write(append(body, '\n'))
	_ = f.Close()
}

// resolveQuotaLimits picks the caller's limits resolver when supplied,
// otherwise the boot-time constant from quota.json. Split out so the choice is
// testable without booting a tenant.
func resolveQuotaLimits(override quota.LimitsFunc, bootPerOrg int64) quota.LimitsFunc {
	if override != nil {
		return override
	}
	return quota.FixedLimits(bootPerOrg)
}

// resolveQuotaSources prefers the caller's source list, falling back to the
// one the router materialized into quota.json.
func resolveQuotaSources(override []quota.Source, fromFile []tenantcfg.QuotaSource) []quota.Source {
	if override != nil {
		return override
	}
	return tenantcfg.DecodeQuota(fromFile)
}
