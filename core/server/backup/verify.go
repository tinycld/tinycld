package backup

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/backup/format"
)

// VerifyReport is what an operator reads after a restore. Each count is
// [expected, actual]: expected comes from the archive's manifest, actual from
// the live data. An actual of -1 means the collection could not be counted at
// all, which is a stronger signal than a wrong number.
type VerifyReport struct {
	IntegrityOK bool              `json:"integrityOk"`
	Collections map[string][2]int `json:"collections"`
	Files       [2]int            `json:"files"`
	OK          bool              `json:"ok"`
}

// Verify compares the live data with the manifest of the most recent succeeded
// restore. With no restore on record it only runs the integrity check: there is
// nothing to compare against, and that is not a failure.
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
		// The ledger cannot be compared with itself. The archive's manifest
		// counted it as it was when the backup ran, and finalizing the restore
		// then inserts the succeeded row into that very collection — so the live
		// count is always at least one higher and every restore would report a
		// mismatch it caused itself.
		if name == collection {
			continue
		}
		var n int
		// A collection name is an identifier, not a parameter, so it is quoted
		// rather than bound.
		if err := app.DB().Select("count(*)").From("`" + name + "`").Row(&n); err != nil {
			log.Warn("could not count a collection while verifying a restore", "collection", name, "err", err)
			n = -1
		}
		rep.Collections[name] = [2]int{want, n}
		if n != want {
			rep.OK = false
		}
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
	if rep.Files[0] != rep.Files[1] {
		rep.OK = false
	}
	return rep, nil
}
