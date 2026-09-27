// Package format is the tinycld-backup-v1 container: a tar stream, zstd
// compressed, age encrypted. It has no PocketBase dependency so the CLI can
// read archives without a server.
package format

import "time"

const FormatV1 = "tinycld-backup-v1"

const (
	MemberManifest  = "manifest.json"
	MemberDB        = "data.db"
	MemberChecksums = "checksums.txt"
	StoragePrefix   = "storage/"
)

// Lockfile is slug → spec. "tinycld" is the core entry.
type Lockfile map[string]string

type Counts struct {
	Collections map[string]int `json:"collections"`
	Files       int            `json:"files"`
	Bytes       int64          `json:"bytes"`
}

type Manifest struct {
	Format   string            `json:"format"`
	Created  time.Time         `json:"created"`
	Instance string            `json:"instance"`
	Source   string            `json:"source"` // hosted | docker | standalone
	Kind     string            `json:"kind"`   // manual | scheduled | pre_restore
	Core     string            `json:"core"`
	Lockfile Lockfile          `json:"lockfile"`
	Packages map[string]string `json:"packages"` // slug → version
	Counts   Counts            `json:"counts"`
}
