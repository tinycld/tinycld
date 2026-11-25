package oauth

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// The records API returns the records of related collections when a request
// carries ?expand=. PocketBase checks each related collection's view rule as
// the user, but the user may read more than the token: without the check in
// this file, a token scoped to one package's collections reads every
// collection they link to.
//
// The check applies to every records route (list, view, and the create and
// update responses). The batch API and realtime, which also accept an expand,
// stay default-denied for tokens as a whole.

const expandQueryParam = "expand"

// The two back-relation spellings PocketBase accepts:
// "<collection>_via_<field>" and the deprecated "<collection>(<field>)".
var (
	backRelationPattern    = regexp.MustCompile(`^(\w+)_via_(\w+)$`)
	oldBackRelationPattern = regexp.MustCompile(`^(\w+)\((\w+)\)$`)
)

// checkExpand refuses a records request whose expand reaches a collection the
// granted scopes may not read. A collection no scope governs is refused too.
func checkExpand(app core.App, collection string, query url.Values, granted []string) error {
	for _, param := range query[expandQueryParam] {
		for _, path := range strings.Split(param, ",") {
			// PocketBase drops spaces and incomplete segments before it
			// resolves a path, so the check must read the same path.
			path = strings.Trim(strings.ReplaceAll(path, " ", ""), ".")
			if path == "" {
				continue
			}
			if err := checkExpandPath(app, collection, path, granted); err != nil {
				return err
			}
		}
	}
	return nil
}

func checkExpandPath(app core.App, collection, path string, granted []string) error {
	current, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		// No such collection: the records handler answers 404.
		return nil
	}
	for _, segment := range strings.Split(path, ".") {
		target := expandTarget(app, current, segment)
		if target == nil {
			// PocketBase stops at a segment it cannot resolve, so the rest of
			// the path returns no records.
			return nil
		}
		if err := checkRelatedRead(target, "expand", path, granted); err != nil {
			return err
		}
		current = target
	}
	return nil
}

// checkRelatedRead applies the read rule of a collection that a request
// reaches through a relation. param and path name the query parameter and the
// path for the error message.
func checkRelatedRead(target *core.Collection, param, path string, granted []string) error {
	access, known := lookupCollection(target.Name)
	if !known || len(access.read) == 0 {
		return fmt.Errorf("the %s %q is not available to API tokens", param, path)
	}
	if !access.read.SatisfiedBy(granted) {
		return fmt.Errorf("the %s %q requires the %s scope", param, path, access.read.describe())
	}
	return nil
}

// expandTarget resolves one expand segment to the collection whose records it
// returns, the way PocketBase's expandRecords does, or nil when the segment
// names no relation of the collection.
func expandTarget(app core.App, from *core.Collection, segment string) *core.Collection {
	pattern := backRelationPattern
	if strings.Contains(segment, "(") {
		pattern = oldBackRelationPattern
	}
	if matches := pattern.FindStringSubmatch(segment); len(matches) == 3 {
		return backRelationTarget(app, from, matches[1]+"_via_"+matches[2])
	}
	field, _ := from.Fields.GetByName(segment).(*core.RelationField)
	if field == nil {
		return nil
	}
	target, err := app.FindCachedCollectionByNameOrId(field.CollectionId)
	if err != nil {
		return nil
	}
	return target
}
