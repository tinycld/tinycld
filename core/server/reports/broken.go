package reports

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"unicode/utf8"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/changesignal"
)

// Broken is why a saved report cannot run: its spec does not load or does
// not validate. Status is 400 (invalid) or 422 (it names a source, link,
// field or column that does not exist), as a run would answer. Problems are
// keyed by spec path, as the run endpoint keys them. At most
// maxBrokenProblems are kept; More counts the rest.
type Broken struct {
	Status   int                `json:"status"`
	Problems map[string]Problem `json:"problems"`
	More     int                `json:"more,omitempty"`
}

// Problem is one part of a spec that is wrong.
type Problem struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// maxBrokenProblems bounds the problems a mark lists; markJSON bounds its
// bytes.
const maxBrokenProblems = 50

// maxBrokenBytes is the broken column's json maxSize in the migration.
const maxBrokenBytes = 65536

// maxProblemMessage bounds one problem's message in bytes. A message can
// quote the spec: a decoder error names an unknown key of any length.
const maxProblemMessage = 500

// touch is changesignal.Touch; a test replaces it to count the signals.
var touch = changesignal.Touch

// cutMessage cuts msg to at most maxProblemMessage bytes on a rune
// boundary, with "…" at the end when it cut.
func cutMessage(msg string) string {
	if len(msg) <= maxProblemMessage {
		return msg
	}
	const ellipsis = "…"
	end := maxProblemMessage - len(ellipsis)
	for end > 0 && !utf8.RuneStart(msg[end]) {
		end--
	}
	return msg[:end] + ellipsis
}

// markJSON is the stored form of a mark. A mark past the column's maxSize
// (long spec paths can pass it even with every message cut) is stored as
// one problem that counts them, because PocketBase refuses every save of a
// row whose json field is past its maxSize: a rename, an offboard move.
func markJSON(b *Broken) ([]byte, error) {
	raw, err := json.Marshal(b)
	if err != nil || len(raw) <= maxBrokenBytes {
		return raw, err
	}
	n := len(b.Problems) + b.More
	return json.Marshal(Broken{Status: b.Status, Problems: map[string]Problem{"spec": {
		Code:    "too_many_problems",
		Message: fmt.Sprintf("the spec has %d problems, too many to store; run the report to see them", n),
	}}})
}

// BrokenReport is one saved report that cannot run, with its owner (nil
// when the owner row cannot be read).
type BrokenReport struct {
	ID      string
	Title   string
	OwnerID string
	Owner   *core.Record
	Broken  Broken
}

// CheckResult says what CheckSaved did. Batches counts the reads of saved
// reports; Written counts the reports whose broken column changed.
type CheckResult struct {
	Checked int
	Batches int
	Written int
	Broken  []BrokenReport
}

// checkBatch is how many saved reports one read loads. A spec has at most
// maxSpecBytes, so one read holds at most checkBatch specs in memory.
var checkBatch = 50

// CheckSaved loads and validates every saved report as a run would, against
// the sources registered and checked now, and stores the outcome in each
// report's broken column when it changed. It reads checkBatch reports at a
// time in id order, so the specs in memory and the size of each statement
// stay the same however many reports exist; res.Broken grows with the
// number of broken reports. The column is written with SQL, not a record
// save: it is the server's note, not an edit, so `updated` and the audit
// log stay as the people who edit the report left them. The reports
// collection is signaled whenever a mark was written, also when an error
// stops the check later. Broken comes back in id order.
func CheckSaved(app core.App) (*CheckResult, error) {
	res := &CheckResult{}
	defer func() {
		if res.Written > 0 {
			touch(app, reportsCollection)
		}
	}()
	if _, err := app.FindCachedCollectionByNameOrId(reportsCollection); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return res, nil // an app without core's report migration saves no reports
		}
		return res, fmt.Errorf("reports: find the reports collection: %w", err)
	}
	after := ""
	for {
		batch, err := app.FindRecordsByFilter(reportsCollection, "id > {:after}", "id", checkBatch, 0, dbx.Params{"after": after})
		res.Batches++
		if err != nil {
			return res, fmt.Errorf("reports: read saved reports: %w", err)
		}
		for _, rec := range batch {
			res.Checked++
			broken := checkSpec(specBytes(rec))
			if broken != nil {
				res.Broken = append(res.Broken, BrokenReport{ID: rec.Id, Title: rec.GetString("title"), OwnerID: rec.GetString("owner"), Broken: *broken})
			}
			written, err := writeBroken(app, rec, broken)
			if err != nil {
				return res, err
			}
			if written {
				res.Written++
			}
		}
		if len(batch) < checkBatch {
			break
		}
		after = batch[len(batch)-1].Id
	}
	for i := range res.Broken {
		owner, err := app.FindRecordById("users", res.Broken[i].OwnerID)
		switch {
		case err == nil:
			res.Broken[i].Owner = owner
		case !errors.Is(err, sql.ErrNoRows):
			return res, fmt.Errorf("reports: read owner of report %s: %w", res.Broken[i].ID, err)
		}
	}
	return res, nil
}

// checkSpec is nil for a spec that loads and validates, else why it fails.
func checkSpec(raw []byte) *Broken {
	spec, _, err := Load(raw)
	if err == nil {
		err = Validate(spec)
	}
	if err == nil {
		return nil
	}
	var specErr *SpecError
	if !errors.As(err, &specErr) {
		// An engine fault (a missing upgrade step), not the spec's: the run
		// would answer 500 as well.
		return &Broken{Status: 500, Problems: map[string]Problem{"version": {Code: "no_upgrade", Message: err.Error()}}}
	}
	keys := slices.Sorted(maps.Keys(specErr.Problems))
	status, _ := specStatus(specErr)
	b := &Broken{Status: status, Problems: map[string]Problem{}}
	for i, k := range keys {
		if i == maxBrokenProblems {
			b.More = len(keys) - i
			break
		}
		p := specErr.Problems[k]
		b.Problems[k] = Problem{Code: p.Code(), Message: cutMessage(p.Message())}
	}
	return b
}

// writeBroken stores broken (nil clears it) unless the column holds it
// already, and reports whether it wrote.
func writeBroken(app core.App, rec *core.Record, broken *Broken) (bool, error) {
	var want []byte
	if broken != nil {
		var err error
		if want, err = markJSON(broken); err != nil {
			return false, err
		}
	}
	have, _ := rec.Get("broken").(types.JSONRaw)
	if bytes.Equal(have, want) || (want == nil && (len(have) == 0 || string(have) == "null")) {
		return false, nil
	}
	var value any
	if want != nil {
		value = string(want)
	}
	if _, err := app.DB().NewQuery("UPDATE reports SET broken = {:broken} WHERE id = {:id}").
		Bind(dbx.Params{"broken": value, "id": rec.Id}).Execute(); err != nil {
		return false, fmt.Errorf("reports: mark report %s: %w", rec.Id, err)
	}
	return true, nil
}

// writeBrokenReports lists a check's result for a person: a summary line,
// then each broken report with its owner and problems.
func writeBrokenReports(out io.Writer, res *CheckResult) {
	fmt.Fprintf(out, "checked %d saved reports, %d broken\n", res.Checked, len(res.Broken))
	for _, b := range res.Broken {
		fmt.Fprintf(out, "%s  %q  owner %s\n", b.ID, b.Title, ownerLabel(b.Owner))
		for _, k := range slices.Sorted(maps.Keys(b.Broken.Problems)) {
			p := b.Broken.Problems[k]
			fmt.Fprintf(out, "  %s: %s: %s\n", k, p.Code, p.Message)
		}
		if b.Broken.More > 0 {
			fmt.Fprintf(out, "  and %d more\n", b.Broken.More)
		}
	}
}

func ownerLabel(owner *core.Record) string {
	if owner == nil {
		return "(unknown)"
	}
	return fmt.Sprintf("%s <%s>", owner.GetString("name"), owner.Email())
}
