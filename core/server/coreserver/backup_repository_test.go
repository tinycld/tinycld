package coreserver

import (
	"encoding/json"
	"testing"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/repo"
	"tinycld.org/core/syscfg"
)

type mapProvider struct {
	values  map[string]string
	managed []string
}

func (m mapProvider) Get(k string) string       { return m.values[k] }
func (m mapProvider) ManagedPrefixes() []string { return m.managed }

// hasJob reports whether app's cron has a job with the given id. Cron exposes
// Jobs() but not a lookup by id.
func hasJob(app core.App, id string) bool {
	for _, j := range app.Cron().Jobs() {
		if j.Id() == id {
			return true
		}
	}
	return false
}

func TestScheduleFollowsTheConfig(t *testing.T) {
	app := backupTestApp(t)
	t.Cleanup(syscfg.ResetForTesting)

	syscfg.SetResolver(mapProvider{values: map[string]string{}}.Get)
	scheduleRepositoryBackup(app)
	if hasJob(app, repoJobID) {
		t.Fatal("scheduled with nothing configured")
	}

	syscfg.SetResolver(mapProvider{values: map[string]string{
		repoKeyKind: "pbs", repoKeyEnabled: "true", repoKeySchedule: "15 2 * * *",
	}}.Get)
	scheduleRepositoryBackup(app)
	if !hasJob(app, repoJobID) {
		t.Fatal("not scheduled")
	}

	syscfg.SetResolver(mapProvider{values: map[string]string{repoKeyKind: "pbs", repoKeyEnabled: "false"}}.Get)
	scheduleRepositoryBackup(app)
	if hasJob(app, repoJobID) {
		t.Fatal("still scheduled after disable")
	}
}

func TestManagedPrefixTurnsTheScheduleOff(t *testing.T) {
	app := backupTestApp(t)
	t.Cleanup(syscfg.ResetForTesting)
	syscfg.SetProvider(mapProvider{
		values:  map[string]string{repoKeyKind: "pbs", repoKeyEnabled: "true"},
		managed: []string{"backup.repository."},
	})
	scheduleRepositoryBackup(app)
	if hasJob(app, repoJobID) {
		t.Fatal("scheduled under a managed prefix")
	}
}

func TestOpenRepositoryFillsTheBackupID(t *testing.T) {
	app := backupTestApp(t)
	app.Settings().Meta.AppURL = "https://acme.example"
	var seen json.RawMessage
	repo.ResetForTesting()
	t.Cleanup(repo.ResetForTesting)
	repo.Register("capture", func(cfg json.RawMessage) (repo.Repository, error) { seen = cfg; return nil, nil })
	if _, err := openRepositoryWith(app, "capture", json.RawMessage(`{"server":"10.0.0.5"}`)); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	_ = json.Unmarshal(seen, &got)
	if got["backup_id"] != "acme.example" {
		t.Fatalf("cfg = %s", seen)
	}
}
