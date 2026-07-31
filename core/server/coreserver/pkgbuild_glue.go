package coreserver

import "tinycld.org/core/pkgbuild"

// This file is the coreserver ⇄ pkgbuild seam, and the extraction's audit
// trail: every alias/delegate below exists because a coreserver host tail
// (registry, install log, DB backup, activation, restart) still consumes an
// identifier that moved into the shared pkgbuild library
// (multi-org/docs/DESIGN-org-package-agency.md, D5). Keeping the aliases here
// means the DB-side callers and the rebuild orchestrator's tests compile
// unchanged while the library stays host-free.

// Exec helpers (moved from pkg_install.go). Consumers: pkg_build.go,
// rebuild.go, app_native_export.go, and the not-yet-moved assemble/pipeline
// steps.
func runCmd(dir string, name string, args ...string) (string, error) {
	return pkgbuild.RunCmd(dir, name, args...)
}

func runCmdEnv(dir string, extraEnv []string, name string, args ...string) (string, error) {
	return pkgbuild.RunCmdEnv(dir, extraEnv, name, args...)
}

func runCmdStreaming(onLine func(line string), dir, name string, args ...string) (string, error) {
	return pkgbuild.RunCmdStreaming(onLine, dir, name, args...)
}

func copyDir(src, dst string) error {
	return pkgbuild.CopyDir(src, dst)
}

// installJobSink adapts an *installJob onto pkgbuild's ProgressSink: milestones
// go through emitProgress (SSE + durable log line), detail lines through
// jobLogf. A nil job degrades exactly like the underlying helpers do.
type installJobSink struct{ job *installJob }

var _ pkgbuild.ProgressSink = installJobSink{}

func (s installJobSink) Progress(step string, percent int, message string) {
	emitProgress(s.job, step, percent, message)
}

func (s installJobSink) Logf(format string, args ...any) {
	jobLogf(s.job, format, args...)
}
