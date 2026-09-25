package coreserver

import (
	"context"
	"fmt"
	"sort"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
)

// nameRestoreJob gives a restore's job a package slug.
//
// beginRestore builds the job with no slug — a restore is not about one package —
// but pkg_install_log.pkg_slug is REQUIRED, and createInstallLog's fallback is
// job.NpmPkg, which a restore also leaves empty. The row therefore failed to save
// on every restore rebuild, logging "failed to create install log" and leaving the
// operator's install history with no trace of the rebuild that replaced their
// deployment. The base member is the honest slug: a restore rebuild replaces the
// whole package set, not one member of it.
func nameRestoreJob(job *installjob.Job) {
	if job.Slug == "" {
		job.Slug = baseRegistrySlug
	}
}

// RegisterBackupSelfRebuild plugs this deployment's own rebuild pipeline in as
// the restore's rebuilder, so an archive whose package set differs from this
// binary's can be restored rather than refused: build a binary carrying exactly
// the archive's lockfile, activate it, and end the process so the supervisor
// launches it onto the staged data.
//
// Only a deployment that has a toolchain registers this. One that does not
// registers nothing and the restore refuses a mismatch instead (restore.go
// phase 2), which is the correct outcome there — restoring rows that belong to
// packages the binary does not carry leaves data no screen can reach.
func RegisterBackupSelfRebuild(app *pocketbase.PocketBase) {
	backup.RegisterRebuilder(func(_ context.Context, job *installjob.Job, lf format.Lockfile) error {
		// The restore hands its own claim over rather than releasing it, so there
		// is nothing to claim here: an install slipping into a release/re-claim
		// window would have wasted a pre-restore backup and a fully staged
		// archive. From here this function owns the job, and finishJob releases
		// it on every path that returns.
		defer finishJob(job)

		nameRestoreJob(job)

		m := RebuildManifest{BuildID: newBuildID()}
		for slug, spec := range lf {
			// The lockfile speaks registry slugs; the build speaks member slugs.
			// Core is the only one they differ on, and the archive already
			// writes it as the member slug — mapping anyway keeps this correct
			// if that ever changes.
			m.Members = append(m.Members, MemberSpec{Slug: registrySlugToMember(slug), Spec: spec})
		}
		// Map iteration is random; a build's manifest is its rollback record and
		// its log line, so the member order must not change run to run.
		sort.Slice(m.Members, func(i, j int) bool { return m.Members[i].Slug < m.Members[j].Slug })
		logRecord := createInstallLog(app, job, "install")
		deps := productionRebuildDeps(app, job, m, logRecord)
		// No migration sync. The live database is about to be replaced by the
		// archive's, so reconciling THIS database's schema to the new build
		// would migrate a database that is on its way out — and the staged one
		// already carries the schema its own packages wrote.
		deps.syncMig = func(string) (SyncResult, error) { return SyncResult{}, nil }
		if err := rebuildWith(job, m, deps); err != nil {
			return fmt.Errorf("rebuild for restore: %w", err)
		}
		return nil
	})
	// A restore that needs no rebuild still has to end the process: the staged
	// data is what the next one boots on. The WAL checkpoint first, or the new
	// process reads a data.db missing this one's last writes.
	//
	// In dev mode there is no supervisor to relaunch anything, so requestRestart
	// is a no-op — and a restore that believed it had been honoured left the
	// deployment behind the maintenance 503 for good. Say so instead: the restore
	// then goes back to serving the current data and its row records that a
	// restart is still owed.
	backup.SetRestart(func() bool {
		if isDevelopment() {
			srvLog.Error("restart skipped in dev mode — restart the server manually to apply the restore")
			return false
		}
		checkpointWAL(app)
		requestRestart("")
		return true
	})
}
