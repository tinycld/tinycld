package quota

import (
	"fmt"
	"regexp"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// safeIdent guards the identifiers interpolated into the SUM queries. Collection
// and field names come from package config and cannot be bound as parameters, so
// they are validated once rather than trusted — same rule as core/webdav.
var safeIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ValidateSource rejects a Source whose identifiers could not be safely
// interpolated, or that is missing a required field.
func ValidateSource(src Source) error {
	for label, v := range map[string]string{
		"Collection": src.Collection,
		"SizeField":  src.SizeField,
	} {
		if v == "" {
			return fmt.Errorf("quota: Source.%s is required", label)
		}
		if !safeIdent.MatchString(v) {
			return fmt.Errorf("quota: Source.%s = %q is not a valid SQL identifier", label, v)
		}
	}
	if src.OwnerField != "" && !safeIdent.MatchString(src.OwnerField) {
		return fmt.Errorf("quota: Source.OwnerField = %q is not a valid SQL identifier", src.OwnerField)
	}
	return nil
}

// UserUsage returns the bytes owned by one user across every source that
// declares an owner. Sources without one are shared data and are excluded.
func UserUsage(app core.App, sources []Source, userID string) (int64, error) {
	var total int64
	for _, src := range sources {
		if src.OwnerField == "" {
			continue
		}
		n, err := sumCollection(app, src, userID)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// OrgUsage returns the bytes held by the whole deployment across every source.
func OrgUsage(app core.App, sources []Source) (int64, error) {
	var total int64
	for _, src := range sources {
		n, err := sumCollection(app, src, "")
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// sumCollection sums one source's size column, optionally scoped to an owner.
//
// A missing table is not an error: a package can declare a quota source and be
// absent from a given assembly (mail's tables do not exist in a drive-only
// deployment). Treating that as zero keeps the lean-shell guarantee — the sum
// must not fail because a feature is not installed.
func sumCollection(app core.App, src Source, ownerID string) (int64, error) {
	var result struct {
		Total int64 `db:"total"`
	}

	q := fmt.Sprintf("SELECT COALESCE(SUM(%s), 0) AS total FROM %s", src.SizeField, src.Collection)
	params := dbx.Params{}
	if ownerID != "" {
		q += fmt.Sprintf(" WHERE %s = {:owner}", src.OwnerField)
		params["owner"] = ownerID
	}

	if err := app.DB().NewQuery(q).Bind(params).One(&result); err != nil {
		if isMissingTable(app, src.Collection) {
			return 0, nil
		}
		return 0, fmt.Errorf("quota: sum %s: %w", src.Collection, err)
	}
	return result.Total, nil
}

// isMissingTable reports whether a collection simply is not part of this
// assembly, so a query failure can be distinguished from a real DB error.
func isMissingTable(app core.App, collection string) bool {
	_, err := app.FindCollectionByNameOrId(collection)
	return err != nil
}
