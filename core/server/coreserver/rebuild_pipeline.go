package coreserver

import (
	"tinycld.org/core/installjob"
	"tinycld.org/core/pkgbuild"
	"tinycld.org/core/syscfg"
)

// runBuildPipeline turns an assembled build dir into a runnable one via the
// shared pkgbuild pipeline. The Pipeline value is constructed PER INVOCATION,
// not at init: binaryName is set by Register() after package init, and
// systemConfig values (the Sentry keys the export steps read) must be the
// live settings at build time.
func runBuildPipeline(job *installjob.Job, buildDir, buildID string) (buildOutput, error) {
	p := pkgbuild.Pipeline{
		BinaryName: binaryName,
		// Through the seam, not systemConfig directly. A self-rebuild only ever
		// runs on a deployment that administers its own settings — a hosted
		// tenant never registers the install endpoints, because the router owns
		// its deploys — so the two resolve identically today. Using the seam
		// keeps that true if a supervising composition ever gains this path,
		// rather than silently building with an empty Sentry DSN.
		ConfigValue: syscfg.Get,
	}
	return p.Execute(installJobSink{job}, buildDir, buildID)
}
