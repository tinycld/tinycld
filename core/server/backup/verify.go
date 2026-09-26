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

// selfWritten are the collections finalizing a restore WRITES INTO, and which
// therefore cannot be held to the archive's manifest as an equality.
//
// The manifest counted each collection as it was when the BACKUP ran. Completing
// the restore then adds rows of its own: the succeeded row in the backup ledger,
// the notification that tells the administrators the restore happened, and the
// audit entry that records it (announceRestore, plus reportRollbacks on the
// rollback path). So the live count of each of these is always at least one
// higher than the manifest's, through no fault of the data — and every restore
// reported a mismatch it had caused itself.
//
// The consequence was total rather than cosmetic: one mismatch clears rep.OK, so
// Verify could NEVER report ok after a restore. A restore drill reads exactly that
// field to decide whether a backup is restorable, so every drill of a healthy
// backup failed, and the one signal that says "this org can be recovered" was
// permanently false.
//
// They are held to a LOWER BOUND rather than skipped outright, and rather than
// given a counted allowance (see countAgrees). A skip reports real loss in the
// audit log as fine, which is the worst place to be blind: that is where an
// operator goes for evidence of what happened. An allowance would need to know
// how many rows a restore writes, which is not fixed — it depends on how many
// administrators are notified, and the rollback path writes one per marker — so it
// would be a second thing to keep in step with every future write, and it would
// mask a real discrepancy of exactly that size. A bound needs no bookkeeping and
// still catches a shortfall.
//
// It is a package-level map rather than a call-time set because these names are a
// property of what the engine writes, and a caller has no way to know them.
var selfWritten = map[string]bool{
	collection:      true, // the backup ledger: the succeeded restore row
	"notifications": true, // announceRestore tells the administrators
	"audit_logs":    true, // announceRestore records the restore
}

// countAgrees decides whether one collection's live count is acceptable.
//
// For most collections it is equality: the restore replaced the whole database,
// so anything but the manifest's number is data that did not come across.
//
// For a collection the restore WRITES INTO (selfWritten) the test is a LOWER
// BOUND. Those are always at least one row ahead of the manifest through the
// restore's own doing, so equality is unachievable — but they are not exempt,
// which is the point of a bound rather than a skip. Rows can still be MISSING
// from them, and an audit log or a notification history that came back short is
// real loss on exactly the collections an operator would later go to for evidence
// of what happened. A skip reported that as fine; the bound still catches it.
//
// A count of -1 is the "could not be counted at all" marker, and it fails every
// test including the bound: a collection that cannot be read is a stronger signal
// than a wrong number, not a weaker one.
func countAgrees(name string, want, got int) bool {
	if got < 0 {
		return false
	}
	if selfWritten[name] {
		return got >= want
	}
	return got == want
}

// Verify compares the live data with the manifest of the most recent succeeded
// restore. With no restore on record it only runs the integrity check: there is
// nothing to compare against, and that is not a failure.
//
// Collections the restore writes into itself are compared as a lower bound rather
// than skipped — see countAgrees and selfWritten.
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
		if !countAgrees(name, want, n) {
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
