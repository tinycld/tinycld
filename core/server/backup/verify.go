package backup

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

// VerifyReport is what an operator reads after a restore. Each count is
// [expected, actual]: expected comes from the archive's manifest, actual from
// the live data. An actual of -1 means the collection could not be counted at
// all. OK is the integrity check alone; see Verify.
type VerifyReport struct {
	IntegrityOK bool              `json:"integrityOk"`
	Collections map[string][2]int `json:"collections"`
	Files       [2]int            `json:"files"`
	OK          bool              `json:"ok"`
}

// Verify runs the integrity check and reports the live counts beside the
// manifest of the most recent succeeded restore. With no restore on record there
// is nothing to compare against, and that is not a failure.
//
// Only the integrity check decides OK. Verify runs after the restored app has
// booted, so migrations, package boot hooks, scheduled jobs and the restore's
// own ledger, notification and audit rows may already have added or removed rows
// and files. Any package can do this, so no fixed list of exceptions can be
// correct. Loss is caught before boot instead: the transports checksum every
// member, and the staged database's row counts must equal the manifest's
// (arm.CheckCounts) before the swap can happen. The counts are still reported,
// so an operator can see what changed.
func Verify(app core.App) (VerifyReport, error) {
	rep := VerifyReport{Collections: map[string][2]int{}}
	var result string
	if err := app.DB().NewQuery("PRAGMA integrity_check").Row(&result); err != nil {
		return rep, err
	}
	rep.IntegrityOK = result == "ok"
	rep.OK = rep.IntegrityOK

	rows, err := app.FindRecordsByFilter(collection, "kind = 'restore' && status = 'succeeded'", "-created", 1, 0)
	if err != nil {
		return rep, err
	}
	if len(rows) == 0 {
		return rep, nil
	}
	var m format.Manifest
	if err := rows[0].UnmarshalJSONField("manifest", &m); err != nil {
		return rep, err
	}
	for name, want := range m.Counts.Collections {
		var n int
		// A collection name is an identifier, not a parameter, so it is quoted
		// rather than bound.
		if err := app.DB().Select("count(*)").From("`" + name + "`").Row(&n); err != nil {
			log.Warn("could not count a collection while verifying a restore", "collection", name, "err", err)
			n = -1
		}
		rep.Collections[name] = [2]int{want, n}
	}

	fs, err := app.NewFilesystem()
	if err != nil {
		return rep, err
	}
	defer func() {
		if cerr := fs.Close(); cerr != nil {
			log.Warn("could not close the filesystem after verifying a restore", "err", cerr)
		}
	}()
	files, err := fs.List("")
	if err != nil {
		return rep, err
	}
	rep.Files = [2]int{m.Counts.Files, len(files)}
	return rep, nil
}
