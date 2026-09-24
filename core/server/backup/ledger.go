package backup

import (
	"fmt"
	"net/url"
	"path/filepath"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

const collection = "backups"

// ledgerPathOverride pins the working directory. Only the package's own tests
// set it: tests.NewTestApp clones its data dir straight into the system temp
// root, so the derived path would be shared by every test app in the process.
var ledgerPathOverride string

// LedgerPath is the directory the engine keeps its working state under —
// the parent of pb_data in every deployment shape (a Docker state dir, a
// single-binary data dir, an organization's own directory).
func LedgerPath(app core.App) string {
	if ledgerPathOverride != "" {
		return ledgerPathOverride
	}
	return filepath.Dir(app.DataDir())
}

func tmpDir(app core.App) string { return filepath.Join(LedgerPath(app), "backup-tmp") }

func newRow(app core.App, kind Kind, initiator, targetHost string) *core.Record {
	col, err := app.FindCollectionByNameOrId(collection)
	if err != nil {
		panic("backups collection missing: " + err.Error())
	}
	r := core.NewRecord(col)
	r.Set("kind", string(kind))
	r.Set("status", "running")
	r.Set("started", types.NowDateTime())
	if initiator != "" {
		r.Set("initiated_by", initiator)
	}
	r.Set("target_host", targetHost)
	return r
}

func finishRow(app core.App, r *core.Record, status string, written int64, sha, errMsg string, manifest any) error {
	r.Set("status", status)
	r.Set("finished", types.NowDateTime())
	r.Set("bytes", written)
	r.Set("sha256", sha)
	r.Set("error", truncate(errMsg, 2000))
	if manifest != nil {
		r.Set("manifest", manifest)
	}
	return app.Save(r)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// HostOnly reduces a signed URL to the part safe to keep. A presigned target
// URL carries its own credentials in the query string, so the ledger records
// where a backup went and nothing more.
func HostOnly(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

// MarkInterrupted closes every row a previous process left running. Called
// once per boot with the process start time: a row started before this process
// existed cannot still be running, because the goroutine that owned it died
// with the old process.
func MarkInterrupted(app core.App, bootedAt time.Time) error {
	rows, err := app.FindRecordsByFilter(collection, "status = 'running' || status = 'waiting_for_source'", "", 0, 0)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.GetDateTime("started").Time().After(bootedAt) {
			continue
		}
		r.Set("status", "interrupted")
		r.Set("finished", types.NowDateTime())
		r.Set("error", "the server restarted before this run finished")
		if err := app.Save(r); err != nil {
			return fmt.Errorf("backup: mark run %s interrupted: %w", r.Id, err)
		}
	}
	return nil
}
