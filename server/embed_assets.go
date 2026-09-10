//go:build embedassets

package main

import (
	"embed"
	"io/fs"
	"log"
)

// Populated by `pnpm run stage:embed`, which materializes the generator's
// symlink farms into real files and copies the Expo export WITHOUT source
// maps. `all:` is required so Expo's dotfile-prefixed chunks are included.
//
//go:embed all:embedded_assets
var embeddedAssets embed.FS

func mustSub(dir string) fs.FS {
	sub, err := fs.Sub(embeddedAssets, "embedded_assets/"+dir)
	if err != nil {
		log.Fatalf("embedded assets: %s missing from the build: %v", dir, err)
	}
	return sub
}

func embeddedWebFS() fs.FS        { return mustSub("web") }
func embeddedMigrationsFS() fs.FS { return mustSub("pb_migrations") }
func embeddedHooksFS() fs.FS      { return mustSub("pb_hooks") }
