// Command export-types regenerates pbSchema.ts + pbZodSchema.ts from the
// PocketBase migration set and exits.
//
// This is the standalone binary the workspace build uses (via
// app/scripts/export-types.ts → invoked by `npm install`'s postinstall).
// Pure Go, no CGO, imports only core/coreserver — so it builds quickly
// inside the lean web-builder Docker stage without dragging in the
// feature-server CGO dependency chain (mupdf, goheif, …) that the full
// `tinycld` binary needs.
//
// The full `tinycld` binary exposes the same operation as a
// `tinycld export-types` subcommand for ad-hoc dev use. Both code paths
// share coreserver.ExportTypes so output is byte-identical.
//
// Usage:
//
//	export-types --dir <pb_data> --migrationsDir <pb_migrations> --typesDir <core/types>
//
// All three flags are required. The caller is expected to wrap this with a
// fresh tmpdir for --dir (see app/scripts/export-types.ts) so the output is
// reproducible from a clean checkout rather than depending on whatever's in
// a developer's dev DB.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/coreserver"
)

func main() {
	pbData := flag.String("dir", "", "PocketBase data dir (will be created if missing)")
	migrationsDir := flag.String("migrationsDir", "", "directory containing JS migration files")
	typesDir := flag.String("typesDir", "", "directory to write pbSchema.ts and pbZodSchema.ts")
	flag.Parse()

	if *pbData == "" || *migrationsDir == "" || *typesDir == "" {
		flag.Usage()
		os.Exit(2)
	}

	if err := os.MkdirAll(*pbData, 0o755); err != nil {
		log.Fatalf("export-types: mkdir pb_data: %v", err)
	}

	app := pocketbase.NewWithConfig(pocketbase.Config{
		DefaultDataDir:  *pbData,
		HideStartBanner: true,
	})
	if err := coreserver.ExportTypes(app, coreserver.ExportTypesOptions{
		MigrationsDir: *migrationsDir,
		TypesDir:      *typesDir,
	}); err != nil {
		log.Fatalf("export-types: %v", err)
	}
}
