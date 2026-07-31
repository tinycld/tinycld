package coreserver

import "tinycld.org/core/pkgbuild"

// runBuildPipeline turns an assembled build dir into a runnable one via the
// shared pkgbuild pipeline. The Pipeline value is constructed PER INVOCATION,
// not at init: binaryName is set by Register() after package init, and
// systemConfig values (the Sentry keys the export steps read) must be the
// live settings at build time.
func runBuildPipeline(job *installJob, buildDir, buildID string) (buildOutput, error) {
	p := pkgbuild.Pipeline{
		BinaryName:  binaryName,
		ConfigValue: systemConfig.Get,
	}
	return p.Execute(installJobSink{job}, buildDir, buildID)
}
