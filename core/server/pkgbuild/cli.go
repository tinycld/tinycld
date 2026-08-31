package pkgbuild

import (
	"os"
	"path/filepath"
)

// CLIDistDirName is where cross-compiled CLI binaries land inside a build's
// app dir, and where the download endpoints and the hosting artifact stager
// look for them.
const CLIDistDirName = "cli-dist"

// CLITarget is one cross-compilation target of the per-org CLI.
type CLITarget struct {
	OS   string
	Arch string
}

// CLITargets is the fixed set of platforms every build cross-compiles the CLI
// for. The download endpoints resolve request paths ONLY against this list, so
// it doubles as the allowlist that keeps {platform} from becoming a path
// traversal vector.
var CLITargets = []CLITarget{
	{OS: "darwin", Arch: "arm64"},
	{OS: "darwin", Arch: "amd64"},
	{OS: "linux", Arch: "amd64"},
	{OS: "linux", Arch: "arm64"},
	{OS: "windows", Arch: "amd64"},
}

// Platform is the URL key for this target ("darwin-arm64").
func (t CLITarget) Platform() string { return t.OS + "-" + t.Arch }

// FileName is the on-disk name inside cli-dist ("tinycld-darwin-arm64").
// The "tinycld" prefix is fixed — the CLI's name is not the host-configurable
// server BinaryName.
func (t CLITarget) FileName() string {
	name := "tinycld-" + t.Platform()
	if t.OS == "windows" {
		name += ".exe"
	}
	return name
}

// DownloadName is the filename a download saves as ("tinycld"/"tinycld.exe").
// It must match the bypass command the About panel and help topic document
// verbatim: `xattr -d com.apple.quarantine ./tinycld`.
func (t CLITarget) DownloadName() string {
	if t.OS == "windows" {
		return "tinycld.exe"
	}
	return "tinycld"
}

// buildCLIBinaries cross-compiles <appDir>/cli for every CLITarget into
// <appDir>/cli-dist/.
//
// BEST-EFFORT BY DESIGN (spec: docs/superpowers/specs/2026-08-05-tinycld-cli-design.md
// Part 5) — unlike every other pipeline step, failure here logs and continues:
// users install packages to get the app working, and losing an install to a
// CLI compile error is the wrong trade. Per-target too, so a windows-only
// compile error still leaves the darwin/linux binaries downloadable. An
// assembled tree with no cli/ module (generator predates the CLI) skips
// silently-but-logged.
//
// CGO_ENABLED=0 needs no C cross-toolchain: the CLI is a pure HTTP client
// with no SQLite.
func (p Pipeline) buildCLIBinaries(sink ProgressSink, appDir string) {
	cliDir := filepath.Join(appDir, "cli")
	if _, err := os.Stat(cliDir); err != nil {
		sink.Logf("cli cross-compile skipped (no cli/ module in build)")
		return
	}
	distDir := filepath.Join(appDir, CLIDistDirName)
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		sink.Logf("cli cross-compile skipped: create %s: %v", CLIDistDirName, err)
		return
	}

	built := 0
	for _, t := range CLITargets {
		out, err := p.runEnv()(cliDir,
			[]string{"GOOS=" + t.OS, "GOARCH=" + t.Arch, "CGO_ENABLED=0"},
			"go", "build", "-trimpath", "-ldflags", "-s -w",
			"-o", filepath.Join(distDir, t.FileName()), ".",
		)
		if err != nil {
			sink.Logf("cli build %s FAILED (continuing): %v\n%s", t.Platform(), err, lastLines(out, 20))
			continue
		}
		built++
	}
	sink.Logf("cli binaries built: %d/%d", built, len(CLITargets))
}
