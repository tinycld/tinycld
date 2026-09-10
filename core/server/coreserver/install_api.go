package coreserver

import (
	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/installjob"
	"tinycld.org/core/pkgbuild"
)

// The package-install API is served by more than one composition: the
// deployment that rebuilds itself in process (pkg_install.go and friends), and
// one whose deploys ride a supervisor's control socket and therefore lives
// outside this repo. Both drive the SAME machinery — the same compat gates, the
// same install log, the same migration sync — because the alternative is two
// copies of the install POLICY drifting apart, which is the failure this
// codebase has already paid for once (see the composition parity tests).
//
// This file is that machinery's public surface, gathered in ONE place so its
// size is visible rather than scattered across a dozen //export renames. Each
// entry is a thin wrapper: no logic lives here.
//
// Everything below is a seam, not a general-purpose API. Do not reach for these
// from feature packages — they exist so a second composition can reuse the
// install pipeline, and they will move again if that stops being true.

// ---------- authorization ----------

// RequireOwner rejects a request that is not the deployment owner's. Package
// operations rebuild the artifact and change what every user runs.
func RequireOwner(re *core.RequestEvent) error { return requireOwner(re) }

// RequireOwnerOrToken accepts the owner OR a valid deploy token.
func RequireOwnerOrToken(app core.App, re *core.RequestEvent) error {
	return requireOwnerOrToken(app, re)
}

// ---------- validation and compat gates ----------

// ValidatePackageSpec rejects a malformed install spec before anything runs.
func ValidatePackageSpec(spec string) error { return validatePackageSpec(spec) }

// ValidateManifest checks a fetched package's manifest against what this
// deployment allows.
func ValidateManifest(m *pkgbuild.ParsedManifest, allowServer bool, bundledSlugs map[string]bool) error {
	return validateManifest(m, allowServer, bundledSlugs)
}

// CheckInstallCompat runs the version-compatibility gates for an install.
func CheckInstallCompat(app core.App, m *pkgbuild.ParsedManifest) error {
	return checkInstallCompat(app, m)
}

// CheckVersionChangeCompat runs the same gates for a version change.
func CheckVersionChangeCompat(app core.App, changes []installjob.VersionChange) error {
	return checkVersionChangeCompat(app, changes)
}

// RejectBaseUninstall refuses to uninstall the app shell itself.
func RejectBaseUninstall(slug string) error { return rejectBaseUninstall(slug) }

// GetBundledSlugs reports the packages built into this binary, which cannot be
// installed or removed at runtime.
func GetBundledSlugs(app core.App) map[string]bool { return getBundledSlugs(app) }

// ClassifySpec splits an install spec into its source kind and remainder.
func ClassifySpec(spec string) (pkgSource, string) { return classifySpec(spec) }

// ---------- the install log ----------

// CreateInstallLog opens the durable pkg_install_log row for a job.
func CreateInstallLog(app core.App, job *installjob.Job, action string) *core.Record {
	return createInstallLog(app, job, action)
}

// UpdateInstallLogSlug fills in the slug once a spec has been resolved.
func UpdateInstallLogSlug(app core.App, record *core.Record, slug string) {
	updateInstallLogSlug(app, record, slug)
}

// FinalizeInstallLog closes the row with a terminal status and the job's log.
func FinalizeInstallLog(app core.App, record *core.Record, status, errMsg string, logLines []string) {
	finalizeInstallLog(app, record, status, errMsg, logLines)
}

// ---------- progress reporting ----------

// EmitProgress advances a job and fans the update out to its SSE listeners.
func EmitProgress(job *installjob.Job, step string, progress int, message string) {
	emitProgress(job, step, progress, message)
}

// JobLogf appends a detail line to a job's recorded log.
func JobLogf(job *installjob.Job, format string, args ...any) { jobLogf(job, format, args...) }

// FinishJob releases the interlock and closes the job's Done channel. Every
// pipeline defers this.
func FinishJob(job *installjob.Job) { finishJob(job) }

// FailJob records a step failure on the job and returns the error to return.
func FailJob(job *installjob.Job, step string, err error) error { return failJob(job, step, err) }

// ---------- shared HTTP handlers ----------
//
// These four are IDENTICAL in every composition — they read the job the
// interlock is holding, or the durable log, neither of which depends on how the
// deploy is carried out. A second copy would be a place for the API to drift.

// HandleEvents streams a running job's progress as server-sent events.
func HandleEvents(re *core.RequestEvent) error { return handleEvents(re) }

// HandleStatus reports the installed package set.
func HandleStatus(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	return handleStatus(app, re)
}

// HandleJobStatus reports one job's state, live or from the durable log.
func HandleJobStatus(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	return handleJobStatus(app, re)
}

// HandleVersionsCheck runs the compatibility pre-flight for a proposed set.
func HandleVersionsCheck(app *pocketbase.PocketBase, re *core.RequestEvent) error {
	return handleVersionsCheck(app, re)
}

// UpsertPkgRegistry writes the pkg_registry row for an installed package.
func UpsertPkgRegistry(app core.App, m *pkgbuild.ParsedManifest, npmPkg string, manifestJSON []byte) error {
	return upsertPkgRegistry(app, m, npmPkg, manifestJSON)
}

// ---------- migrations ----------

// AppliedMigrationFiles lists the migrations this database has applied.
func AppliedMigrationFiles(app core.App) ([]string, error) { return appliedMigrationFiles(app) }

// RegisteredOnly filters a migration list down to those the binary still ships,
// so a file deleted from the tree cannot be reverted against.
func RegisteredOnly(files []string) []string { return registeredOnly(files) }

// SubtractStrings returns the entries of a not present in b.
func SubtractStrings(a, b []string) []string { return subtractStrings(a, b) }

// SyncMigrations brings the database in line with the new package set.
func SyncMigrations(app core.App, applied, newSet []string) (SyncResult, error) {
	return syncMigrations(app, applied, newSet)
}

// LogSyncResult records a migration sync's outcome onto the job.
func LogSyncResult(job *installjob.Job, res SyncResult) { logSyncResult(job, res) }

// DryRevertNamedMigrations reports what reverting these migrations WOULD drop,
// without doing it — the gate that stops a version change silently destroying
// data.
func DryRevertNamedMigrations(app core.App, files []string) (DropReport, error) {
	return dryRevertNamedMigrations(app, files)
}

// RecoverLiveDBAfterExternalWrite re-bootstraps the live app's DB pools after
// something replaced the database file underneath it.
func RecoverLiveDBAfterExternalWrite(app *pocketbase.PocketBase) error {
	return recoverLiveDBAfterExternalWrite(app)
}

// ---------- version discovery ----------

// VersionInfoForRegistryRow builds the per-package version summary the Packages
// UI renders.
func VersionInfoForRegistryRow(rec *core.Record, discover func(spec string) (pkgSource, []string, string)) VersionInfo {
	return versionInfoForRegistryRow(rec, discover)
}
