package coreserver

import (
	"path/filepath"
	"strings"
)

// Standalone mode is the single-binary self-host distribution: the web bundle,
// the JS migrations, and the .pb.ts hooks are compiled into the executable, so
// it runs with no Docker, no Node, and no Go toolchain.
//
// There are deliberately no new location flags. PocketBase already owns --dir
// (its data directory) and --http / --https, so standalone mode reuses them
// rather than introducing a second, competing path mechanism that could
// disagree with the one PocketBase actually honors.

// FlagValue returns the value of a `--name value` or `--name=value` argument,
// or "" when the flag is absent or has no value.
//
// Standalone mode needs --dir before app.Start() parses flags, so that it can
// align core's state root (releases, builds) with the database location.
func FlagValue(args []string, name string) string {
	for i, arg := range args {
		if value, ok := strings.CutPrefix(arg, name+"="); ok {
			return value
		}
		if arg == name && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// StandaloneStateDir maps PocketBase's data directory to the root that core's
// own state helpers (resolveStateDir and the releases/builds paths derived from
// it) should use. PocketBase's --dir IS the pb_data directory, while
// TINYCLD_STATE_DIR is its parent — the directory pb_data sits inside.
func StandaloneStateDir(dataDir string) string {
	return filepath.Dir(dataDir)
}

// supportsSelfRebuild reports whether this deployment can rebuild its own
// binary. The package install/upgrade pipelines re-run the generator and invoke
// the Go toolchain and pnpm, none of which exists beside a single static binary,
// so a standalone build must not advertise that API. Embedded migrations are the
// discriminator: they are present only in a single-binary build.
func (o Options) supportsSelfRebuild() bool {
	return o.MigrationsFS == nil
}

// ShouldInjectDataDir reports whether a standalone build may append its default
// --dir to the argument list.
//
// Every real command (serve, superuser, create-owner, export-types) operates on
// the database and wants the default. A flag-only invocation must NOT get one:
// cobra reads the appended path as a stray positional and fails the command with
// `unknown command "./tinycld-data/pb_data"`, which breaks --help and --version.
// A --dir the user set always wins.
func ShouldInjectDataDir(args []string) bool {
	if FlagValue(args, "--dir") != "" || hasExactArg(args, "--dir") {
		return false
	}
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return true
		}
	}
	return false
}

// hasExactArg reports whether args contains exactly the given token, so a
// valueless trailing `--dir` is still treated as user-supplied.
func hasExactArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name {
			return true
		}
	}
	return false
}
