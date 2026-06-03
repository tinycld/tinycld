package coreserver

import (
	"fmt"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/plugins/jsvm"
	"github.com/spf13/cobra"
)

// ExportTypesOptions configures the standalone schema-export path.
// Mirrors a minimal subset of coreserver.Options — only the fields
// schema export needs. The full Options struct carries HTTP, static-
// serve, and auth settings that don't apply to a one-shot "boot,
// migrate, write two files, exit" run.
//
// MigrationsDir: where the JS migration files live. Required when the
// app's pb_data isn't the dev one (i.e. when --dir points at a
// tmpdir); the jsvm plugin defaults this to <pb_data>/../pb_migrations,
// which doesn't exist for a fresh tmpdir.
//
// TypesDir: where to write pbSchema.ts and pbZodSchema.ts.
//
// HooksPoolSize: how many goja runtimes to pre-warm in the jsvm pool.
// Schema export only needs to apply migrations once, so a tiny pool is
// fine. Defaults to 5 when zero.
type ExportTypesOptions struct {
	MigrationsDir string
	TypesDir      string
	HooksPoolSize int
}

// ExportTypes is the shared library path called by both the standalone
// `core/server/cmd/export-types` binary and the in-app `export-types`
// subcommand. Wires jsvm so JS migrations load, applies pending app
// migrations, then writes pbSchema.ts + pbZodSchema.ts to TypesDir.
//
// Why a shared library: the build pipeline uses a small standalone
// binary that imports only coreserver (no feature-server CGO deps),
// keeping the `pnpm install`-time toolchain lean. The full app binary
// also exposes the same operation as a `tinycld export-types`
// subcommand for ad-hoc dev runs. Both paths MUST produce byte-
// identical output, so they share this single function.
//
// Bootstrap: this function calls app.Bootstrap() to apply system
// migrations and open the data DB. If the caller (e.g. the subcommand,
// which runs inside pb.Execute()) already bootstrapped, this is a no-
// op rebootstrap — safe per PocketBase's API contract. The standalone
// binary needs the explicit call because it doesn't go through
// pb.Execute().
//
// Migration order: app.RunAppMigrations() applies the JS files under
// MigrationsDir that haven't been recorded in the _migrations table.
// On a fresh tmpdir pb_data the table is empty so every migration
// runs; on an existing pb_data only pending ones run (cheap).
//
// Concurrency: not safe for concurrent calls — both Bootstrap and
// GenerateSchemas mutate process-level state. Production callers run
// this once per process and exit.
func ExportTypes(app *pocketbase.PocketBase, opts ExportTypesOptions) error {
	if opts.TypesDir == "" {
		return fmt.Errorf("export-types: TypesDir is required")
	}
	if opts.MigrationsDir == "" {
		return fmt.Errorf("export-types: MigrationsDir is required")
	}

	poolSize := opts.HooksPoolSize
	if poolSize == 0 {
		poolSize = 5
	}

	// jsvm.MustRegister loads the JS migrations from MigrationsDir into
	// core.AppMigrations so RunAppMigrations can apply them. Without
	// this the runner sees an empty list and produces a system-only
	// schema. HooksDir is left at the jsvm default — the schema export
	// doesn't run hooks, but jsvm's hook scanner is tolerant of a
	// missing dir.
	jsvm.MustRegister(app, jsvm.Config{
		MigrationsDir: opts.MigrationsDir,
		HooksPoolSize: poolSize,
	})

	if err := app.Bootstrap(); err != nil {
		return fmt.Errorf("export-types: bootstrap: %w", err)
	}
	if err := app.RunAppMigrations(); err != nil {
		return fmt.Errorf("export-types: run app migrations: %w", err)
	}
	GenerateSchemas(app, opts.TypesDir)
	return nil
}

// NewExportTypesCommand registers a `tinycld export-types` subcommand
// on the full app binary that calls ExportTypes against whatever
// pb_data and migrations the binary's flags resolve to.
//
// Why a subcommand instead of a `--exportTypes` flag on `serve`: PB's
// own CLI is subcommand-shaped (`serve`, `migrate`, `superuser`),
// gets free `--help`, doesn't pollute `serve`'s flag schema, clearly
// signals "this runs and exits."
//
// Reproducibility: for a clean-checkout regeneration that doesn't
// depend on the developer's dev DB state, point PB at a fresh tmpdir
// via its standard `--dir` flag:
//
//	tinycld export-types --dir $(mktemp -d) --migrationsDir <path> --typesDir <path>
//
// Without `--dir` the command runs against whatever pb_data the
// binary would have used (the dev DB), which is occasionally useful
// for inspecting what the live boot-time hook would emit.
//
// Build pipeline note: `pnpm install`'s postinstall does NOT call this
// subcommand. It calls the standalone `core/server/cmd/export-types`
// binary (CGO-free, no feature-server imports) so it can run inside
// the lean web-builder Docker stage. This subcommand exists for
// ad-hoc dev use, parity testing, and `tinycld --help` discoverability.
//
// typesDir + migrationsDir: the caller passes defaults to use when no
// flag is set on the command line. The RunE handler also looks up the
// parsed flags at run time so explicit command-line values win. Without
// that lookup, callers using DefaultTypesDir() under `go run` get a
// build-cache path that ignores the user's --typesDir argument.
//
// jsvm already registered: the subcommand assumes the caller has
// registered the app via coreserver.Register, which calls
// jsvm.MustRegister. Calling jsvm.MustRegister again inside ExportTypes
// would re-register the hook on the same app and double-trigger
// migration loading. So this path skips the library function and
// calls just RunAppMigrations + GenerateSchemas. The standalone
// binary, which does NOT pre-register jsvm, calls the library.
func NewExportTypesCommand(app *pocketbase.PocketBase, typesDir, migrationsDir string) *cobra.Command {
	return &cobra.Command{
		Use:          "export-types",
		Short:        "Regenerate pbSchema.ts and pbZodSchema.ts, then exit",
		Long:         "Reads the current PocketBase collection state and writes the generated TypeScript schema files to the configured typesDir. Pair with PB's --dir flag pointing at a fresh tmpdir for a reproducible regeneration that doesn't depend on the dev pb_data.",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resolvedTypes := typesDir
			if f := cmd.Root().PersistentFlags().Lookup("typesDir"); f != nil && f.Changed {
				resolvedTypes = f.Value.String()
			}
			if resolvedTypes == "" {
				return fmt.Errorf("export-types: typesDir is empty (set Options.TypesDir or pass --typesDir)")
			}
			// jsvm is already wired via coreserver.Register (which the
			// caller ran before constructing this subcommand). All we
			// need here is to apply pending migrations and emit the
			// types — pb.Execute() already ran Bootstrap().
			if err := app.RunAppMigrations(); err != nil {
				return fmt.Errorf("export-types: run app migrations: %w", err)
			}
			GenerateSchemas(app, resolvedTypes)
			return nil
		},
	}
}
