package coreserver

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup"
	"tinycld.org/core/backup/pbs"
	"tinycld.org/core/backup/repo"
	"tinycld.org/core/syscfg"
)

const (
	repoKeyPrefix       = "backup.repository."
	repoKeyKind         = "backup.repository.kind"
	repoKeyConfig       = "backup.repository.config"
	repoKeySchedule     = "backup.repository.schedule"
	repoKeyEnabled      = "backup.repository.enabled"
	repoJobID           = "backup-repository"
	defaultRepoSchedule = "0 3 * * *"
)

var errNoRepository = errors.New("No backup repository is configured.")

// RegisterBackupRepository registers the repository kinds core ships and keeps
// the scheduled backup in step with the settings. Called after
// RegisterSystemConfig, whose OnServe loads the values this reads.
func RegisterBackupRepository(app core.App) {
	pbs.Register()
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		scheduleRepositoryBackup(app)
		return e.Next()
	})
	SystemSettings().OnChange(func(key, _ string) {
		if strings.HasPrefix(key, repoKeyPrefix) {
			scheduleRepositoryBackup(app)
		}
	})
}

// scheduleRepositoryBackup adds or removes the cron job so it always matches
// the current settings. Called on boot and on every change to a
// "backup.repository." key, so toggling the switch takes effect without a
// restart.
func scheduleRepositoryBackup(app core.App) {
	app.Cron().Remove(repoJobID)
	// A composition that owns these keys backs the deployment up itself.
	if syscfg.IsManaged(repoKeyKind) {
		return
	}
	if syscfg.Get(repoKeyKind) == "" || syscfg.Get(repoKeyEnabled) != "true" {
		return
	}
	schedule := strings.TrimSpace(syscfg.Get(repoKeySchedule))
	if schedule == "" {
		schedule = defaultRepoSchedule
	}
	if err := app.Cron().Add(repoJobID, schedule, func() { runScheduledRepositoryBackup(app) }); err != nil {
		srvLog.Error("the backup schedule is not a valid cron expression", "schedule", schedule, "err", err)
	}
}

func runScheduledRepositoryBackup(app core.App) {
	r, err := openRepository(app)
	if err != nil {
		if ferr := backup.FailedRun(app, backup.KindScheduled, syscfg.Get(repoKeyKind), err); ferr != nil {
			srvLog.Error("could not record a failed scheduled backup", "err", ferr)
		}
		return
	}
	if _, err := backup.Start(app, backup.Request{Kind: backup.KindScheduled, Repo: r, TargetHost: r.Kind()}); err != nil {
		if errors.Is(err, backup.ErrStopping) {
			srvLog.Info("scheduled backup skipped: the server is shutting down")
			return
		}
		if errors.Is(err, backup.ErrBusy) {
			srvLog.Warn("scheduled backup skipped: another job is running")
			return
		}
		srvLog.Error("could not start the scheduled backup", "err", err)
	}
}

func openRepository(app core.App) (repo.Repository, error) {
	kind := syscfg.Get(repoKeyKind)
	if kind == "" {
		return nil, errNoRepository
	}
	return openRepositoryWith(app, kind, json.RawMessage(syscfg.Get(repoKeyConfig)))
}

// openRepositoryWith fills the backup ID from this deployment's hostname and
// refuses a server address only this machine can reach, as for a URL target.
func openRepositoryWith(app core.App, kind string, cfg json.RawMessage) (repo.Repository, error) {
	var m map[string]any
	if len(cfg) == 0 {
		cfg = json.RawMessage(`{}`)
	}
	if err := json.Unmarshal(cfg, &m); err != nil {
		return nil, errors.New("The repository settings are not valid.")
	}
	if id, _ := m["backup_id"].(string); id == "" {
		m["backup_id"] = backup.HostOnly(app.Settings().Meta.AppURL)
	}
	if server, _ := m["server"].(string); server != "" {
		if err := checkBackupTarget(serverURL(server)); err != nil {
			return nil, err
		}
	}
	filled, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return repo.Open(kind, filled)
}

func serverURL(server string) string {
	if strings.HasPrefix(server, "https://") || strings.HasPrefix(server, "http://") {
		return server
	}
	return "https://" + server
}
