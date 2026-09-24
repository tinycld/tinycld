package coreserver

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/format"
	"tinycld.org/core/installjob"
)

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
	backup.RegisterRebuilder(func(_ context.Context, lf format.Lockfile) error {
		job := installjob.New("restore", "", "")
		if _, ok := installjob.Claim(job); !ok {
			return backup.ErrBusy
		}
		defer finishJob(job)

		m := RebuildManifest{BuildID: newBuildID()}
		for slug, spec := range lf {
			// The lockfile speaks registry slugs; the build speaks member slugs.
			// Core is the only one they differ on, and the archive already
			// writes it as the member slug — mapping anyway keeps this correct
			// if that ever changes.
			m.Members = append(m.Members, MemberSpec{Slug: registrySlugToMember(slug), Spec: spec})
		}
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
	backup.SetRestart(func() {
		checkpointWAL(app)
		requestRestart("")
	})
}
