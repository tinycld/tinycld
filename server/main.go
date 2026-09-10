package main

import (
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/coreserver"
	"tinycld.org/core/tenantmain"
)

// defaultHTTPAddr is the loopback address tinycld serves on in dev when no
// --http flag is given. The local-ssl-proxy in `pnpm run dev` listens on 7090
// and forwards here, so this needs to match the SSL proxy's --target port.
const defaultHTTPAddr = "127.0.0.1:7090"

// defaultStandaloneDataDir is where the single-binary build keeps its state
// when no --dir is given. It is relative to the working directory (not the
// binary's own dir) so a downloaded binary does not write into wherever the
// user happened to leave it.
const defaultStandaloneDataDir = "./tinycld-data/pb_data"

// main composes the tinycld app server: load env, build the shared core
// server via coreserver.Register (which initializes Sentry), then start
// PocketBase.
//
// registerPackageExtensions is generator output (see scripts/generate-packages.ts
// → server/package_extensions.go). It's declared in this same package so we
// can hand it to coreserver.Options.RegisterExtras without a cross-package
// generated-import dance.
func main() {
	coreserver.LoadEnvFile()

	// Tenant mode (the dual-mode binary of DESIGN-org-package-agency D5): a
	// hosting router spawned this binary to serve ONE org on a unix socket.
	// The flag contract is exactly serve-org's, so a per-org build artifact is
	// a drop-in replacement for the shared tenant binary — --org-dir is the
	// discriminator, since no host-mode invocation uses that flag. The SAME
	// generated registrar serves both modes (single-Register contract): a
	// per-org build links exactly the org's package set, the artifact is the
	// gate, and a package that must differ hosted detects it via coreserver's
	// TenantContext (stamped before the registrar runs).
	if coreserver.HasFlag("--org-dir") {
		if err := tenantmain.Run(tenantmain.Options{
			RegisterExtras: registerPackageExtensions,
		}); err != nil {
			log.Fatalf("tenant: %v", err)
		}
		return
	}

	// Embedded assets are present only in the single-binary build (the
	// `embedassets` build tag). When absent every accessor returns nil and the
	// server reads from disk exactly as the container build does.
	webFS := embeddedWebFS()
	standalone := webFS != nil

	if standalone {
		// PocketBase owns --dir (its data directory), so standalone mode adds no
		// competing location flag. Core's own state helpers key off
		// TINYCLD_STATE_DIR instead, so derive it from --dir to keep releases
		// and builds beside the database rather than beside the binary.
		dataDir := coreserver.FlagValue(os.Args[1:], "--dir")
		if dataDir == "" {
			dataDir = defaultStandaloneDataDir
			os.Args = append(os.Args, "--dir", dataDir)
		}
		if err := os.Setenv("TINYCLD_STATE_DIR", coreserver.StandaloneStateDir(dataDir)); err != nil {
			log.Fatal(err)
		}
	}

	// Default --http to defaultHTTPAddr when running `serve` without an
	// explicit address and no domain args. PocketBase's autocert needs
	// :80/:443 when domain args are present, so we don't override there.
	// PB's own default for `serve` is 127.0.0.1:8090 — we override to 7090
	// to match the SSL proxy in `pnpm run dev`. Injecting through os.Args
	// (rather than registering a flag default) keeps PB's flag schema
	// untouched and lets explicit `--http :8090` overrides still work.
	if coreserver.HasSubcommand("serve") && !coreserver.HasFlag("--http") && !coreserver.HasDomainArgs() {
		os.Args = append(os.Args, "--http", defaultHTTPAddr)
	}

	// sentry.Init is called inside coreserver.Register so any app composing
	// the core server (this app, web, future apps) gets Sentry for free.
	// Flushing on process exit must stay in main — `defer` only fires when
	// main returns.
	defer sentry.Flush(2 * time.Second)

	app := pocketbase.New()
	coreserver.Register(app, coreserver.Options{
		PublicDir:    coreserver.DefaultPublicDir(),
		WebsiteDir:   coreserver.DefaultWebsiteDir(),
		ReleasesDir:  coreserver.DefaultReleasesDir(),
		FallbackFile: "app.html",
		TypesDir:     coreserver.DefaultTypesDir(),
		BinaryName:   "tinycld",
		// An embedded FS cannot be watched, and a standalone build has nowhere
		// to write generated migrations.
		HooksWatch:     !standalone,
		HooksPoolSize:  15,
		Automigrate:    !standalone,
		PublicFS:       webFS,
		MigrationsFS:   embeddedMigrationsFS(),
		HooksFS:        embeddedHooksFS(),
		RegisterExtras: registerPackageExtensions,
	})

	// `export-types` regenerates pbSchema.ts + pbZodSchema.ts and exits.
	// Wired here (not inside coreserver.Register) so each app opts in —
	// the subcommand assumes an installed app with its own pb_data and
	// migrations dir, which not every coreserver consumer has.
	app.RootCmd.AddCommand(coreserver.NewExportTypesCommand(app, coreserver.DefaultTypesDir(), ""))

	// `create-owner` mints the first app account against an existing pb_data.
	// The hosting router runs it on this binary when provisioning an org:
	// a hosted tenant never binds the setup wizard's routes, so without it a
	// new org serves correctly but has no user who can log in.
	app.RootCmd.AddCommand(coreserver.NewCreateOwnerCommand(app))

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
