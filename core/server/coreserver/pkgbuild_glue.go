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

// Manifest/spec validation (moved from pkg_validate.go). Consumers:
// pkg_install.go, pkg_versions.go, pkg_version_change.go, pkg_compat.go,
// pkg_seed.go, rebuild.go, rebuild_pipelines.go.
type parsedManifest = pkgbuild.ParsedManifest

type manifestRoutes = pkgbuild.ManifestRoutes

type manifestNav = pkgbuild.ManifestNav

type manifestServer = pkgbuild.ManifestServer

func parseManifestViaNode(packageDir string) (*parsedManifest, error) {
	return pkgbuild.ParseManifestViaNode(packageDir)
}

func validateManifest(m *parsedManifest, allowServer bool, bundledSlugs map[string]bool) error {
	return pkgbuild.ValidateManifest(m, allowServer, bundledSlugs)
}

func validatePackageSpec(spec string) error { return pkgbuild.ValidatePackageSpec(spec) }

func isTrustedScope(spec string) bool { return pkgbuild.IsTrustedScope(spec) }

// Spec-shape patterns shared with the host-side version/change handlers.
var (
	slugPattern         = pkgbuild.SlugPattern
	npmPackagePattern   = pkgbuild.NpmPackagePattern
	gitSpecPattern      = pkgbuild.GitSpecPattern
	npmVersionedPattern = pkgbuild.NpmVersionedPattern
	versionTokenPattern = pkgbuild.VersionTokenPattern
)

// Manifest types + slug mapping (moved from rebuild.go). Consumers: the
// rebuild orchestrator + its host tails, rebuild_pipelines.go, pkg_build.go,
// and the orchestrator tests' stubs.
type MemberSpec = pkgbuild.MemberSpec

type RebuildManifest = pkgbuild.RebuildManifest

const (
	baseRegistrySlug = pkgbuild.BaseRegistrySlug
	baseMemberSlug   = pkgbuild.BaseMemberSlug
	corePackageKey   = pkgbuild.CorePackageKey
)

func registrySlugToMember(slug string) string { return pkgbuild.RegistrySlugToMember(slug) }

func memberSlugToRegistry(slug string) string { return pkgbuild.MemberSlugToRegistry(slug) }

func packageJSONVersion(path string) string { return pkgbuild.PackageJSONVersion(path) }

// Compat solver + post-assemble verify (moved from pkg_compat.go). The
// DB-backed pre-flight gates that stay in pkg_compat.go call through these.
type compatViolation = pkgbuild.Violation

func solveCompat(resolved map[string]string, peerVersionsBySlug map[string]map[string]string) []compatViolation {
	return pkgbuild.SolveCompat(resolved, peerVersionsBySlug)
}

func compatError(violations []compatViolation) error { return pkgbuild.ViolationsError(violations) }

func verifyTargetPeerVersions(m RebuildManifest, buildDir string) error {
	return pkgbuild.VerifyTargetPeerVersions(m, buildDir)
}

func peerVersionsFromManifest(manifestJSON string) map[string]string {
	return pkgbuild.PeerVersionsFromManifest(manifestJSON)
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
