package reports

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// maxShippedKey is the reports.shipped_key column's max (the migration).
const maxShippedKey = 120

// Shipped is a report a package ships: a published report every user sees,
// owned by the first admin. Register it from the package's Register(app).
type Shipped struct {
	// Key names the report for good: "<package>.<name>".
	Key         string
	Title       string
	Description string
	Spec        json.RawMessage
	// Revision is 1 or more. Raise it whenever Spec, Title or Description
	// changes: the server then replaces the stored report's spec, title and
	// description at its next start.
	Revision int
}

var (
	shippedMu sync.Mutex
	shipped   = map[string]Shipped{}
)

// RegisterShipped adds a package's shipped report. Registering a key again
// replaces it. A malformed report panics at boot: it is a wiring mistake in
// the package's code. Its spec is checked against the sources when the
// server starts (ReconcileShipped).
func RegisterShipped(s Shipped) {
	if err := validateShipped(s); err != nil {
		panic(fmt.Sprintf("reports.RegisterShipped(%q): %v", s.Key, err))
	}
	s.Spec = slices.Clone(s.Spec)
	shippedMu.Lock()
	defer shippedMu.Unlock()
	shipped[s.Key] = s
}

func validateShipped(s Shipped) error {
	switch {
	case !sourceKeyRe.MatchString(s.Key) || len(s.Key) > maxShippedKey:
		return fmt.Errorf("key must look like \"<package>.<name>\" and have at most %d characters", maxShippedKey)
	case strings.TrimSpace(s.Title) == "" || len(s.Title) > 200:
		return fmt.Errorf("a title has 1 to 200 characters")
	case len(s.Description) > 2000:
		return fmt.Errorf("a description has at most 2000 characters")
	case s.Revision < 1:
		return fmt.Errorf("revision starts at 1")
	case len(s.Spec) > maxSpecBytes || !json.Valid(s.Spec):
		return fmt.Errorf("the spec is not JSON of at most %d bytes", maxSpecBytes)
	}
	return nil
}

func isShipped(key string) bool {
	shippedMu.Lock()
	defer shippedMu.Unlock()
	_, ok := shipped[key]
	return ok
}

// ReconcileShipped makes the stored reports match the shipped ones. A
// missing report is created, published, owned and published by the first
// admin; with no admin yet it waits until one appears (shipForAdmin). A
// stored report whose revision is older gets the shipped spec, title and
// description; its owner and whether it is published stay as people left
// them. A spec that does not validate is skipped and named in the returned
// error, so one broken report does not stop the others.
func ReconcileShipped(app core.App) error {
	shippedMu.Lock()
	defer shippedMu.Unlock()
	var errs []error
	var admin *core.Record
	for _, key := range slices.Sorted(maps.Keys(shipped)) {
		s := shipped[key]
		rec, err := app.FindFirstRecordByFilter(reportsCollection, "shipped_key = {:k}", dbx.Params{"k": key})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			errs = append(errs, fmt.Errorf("shipped report %q: %w", key, err))
			continue
		}
		if rec != nil && rec.GetInt("shipped_revision") >= s.Revision {
			continue
		}
		stored, source, err := shippedSpec(s)
		if err != nil {
			errs = append(errs, fmt.Errorf("shipped report %q: %w", key, err))
			continue
		}
		if rec == nil {
			if admin == nil {
				if admin, err = firstAdmin(app, ""); err != nil {
					return errors.Join(append(errs, err)...)
				}
				if admin == nil {
					return errors.Join(errs...)
				}
			}
			col, err := app.FindCachedCollectionByNameOrId(reportsCollection)
			if err != nil {
				return errors.Join(append(errs, err)...)
			}
			rec = core.NewRecord(col)
			rec.Set("owner", admin.Id)
			rec.Set("published", true)
			rec.Set("published_by", admin.Id)
			rec.Set("shipped_key", key)
		}
		rec.Set("title", s.Title)
		rec.Set("description", s.Description)
		rec.Set("spec", stored)
		rec.Set("source", source)
		rec.Set("shipped_revision", s.Revision)
		rec.Set("broken", nil)
		if err := app.Save(rec); err != nil {
			errs = append(errs, fmt.Errorf("shipped report %q: %w", key, err))
		}
	}
	return errors.Join(errs...)
}

// shippedSpec loads and validates a shipped spec, as a client save does.
func shippedSpec(s Shipped) (types.JSONRaw, string, error) {
	spec, stored, err := loadStored(s.Spec)
	if err != nil {
		return nil, "", err
	}
	if err := Validate(spec); err != nil {
		return nil, "", err
	}
	return stored, spec.Source, nil
}

// shipForAdmin creates the shipped reports when a users record that is an
// active admin is saved: the setup wizard's first owner is the first
// moment a fresh install has someone to own them. Every save of an admin
// runs it; with nothing missing it reads one row per shipped report.
func shipForAdmin(e *core.RecordEvent) error {
	if isActiveAdmin(e.Record) && hasShipped() {
		if err := ReconcileShipped(e.App); err != nil {
			log.Error("shipped reports not reconciled", "err", err)
		}
	}
	return e.Next()
}

func hasShipped() bool {
	shippedMu.Lock()
	defer shippedMu.Unlock()
	return len(shipped) > 0
}

// onReportDelete refuses to delete a shipped report while its package
// ships it: the next start would make it again. Its owner can unpublish it,
// which hides it from everyone else. Once the package is gone, the report
// can be deleted.
func onReportDelete(e *core.RecordRequestEvent) error {
	if e.HasSuperuserAuth() {
		return e.Next()
	}
	if key := e.Record.GetString("shipped_key"); key != "" && isShipped(key) {
		const msg = "This report ships with a package. Its owner can unpublish it instead."
		return e.BadRequestError(msg, fieldError("shipped_key", "shipped", msg))
	}
	return e.Next()
}

// firstAdmin is the oldest active owner, else the oldest active admin,
// leaving out the user except (when set). nil when there is none.
func firstAdmin(app core.App, except string) (*core.Record, error) {
	for _, role := range []string{"owner", "admin"} {
		recs, err := app.FindRecordsByFilter("users",
			"role = {:role} && disabled != true && id != {:except}", "created,id", 1, 0,
			dbx.Params{"role": role, "except": except})
		if err != nil {
			return nil, fmt.Errorf("find the first %s: %w", role, err)
		}
		if len(recs) == 1 {
			return recs[0], nil
		}
	}
	return nil, nil
}
