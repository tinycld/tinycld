//go:build !embedassets

package main

import "io/fs"

// Without the embedassets build tag the binary carries no assets and reads
// them from disk, exactly as the container and managed deployments do. This
// keeps `go build ./...` working with no staged tree present.
func embeddedWebFS() fs.FS                { return nil }
func embeddedMigrationsFS() fs.FS         { return nil }
func embeddedHooksFS() fs.FS              { return nil }
func embeddedBundledPackagesJSON() []byte { return nil }
