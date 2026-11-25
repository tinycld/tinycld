package oauth

import (
	"net/url"
	"strings"

	"github.com/ganigeorgiev/fexpr"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/search"
)

// A list request may filter and sort by a field of a related collection
// ("owner.email ~ 'a'"). PocketBase joins the related collection and applies
// its list rule as the user. No related record is returned, but which rows
// match, and their order, tell the caller the related values. The check in
// this file applies the token's scopes to each collection such a path joins,
// as checkExpand does for an expand.

// checkFilterAndSort refuses a records request whose filter or sort reads a
// collection the granted scopes may not read.
func checkFilterAndSort(app core.App, collection string, query url.Values, granted []string) error {
	base, err := app.FindCachedCollectionByNameOrId(collection)
	if err != nil {
		// No such collection: the records handler answers 404.
		return nil
	}
	for _, raw := range query[search.FilterQueryParam] {
		groups, err := fexpr.Parse(raw)
		if err != nil {
			// PocketBase parses with the same parser and answers 400.
			continue
		}
		for _, path := range filterIdentifiers(groups) {
			if err := checkFieldPath(app, base, search.FilterQueryParam, path, granted); err != nil {
				return err
			}
		}
	}
	for _, raw := range query[search.SortQueryParam] {
		for _, field := range search.ParseSortFromString(raw) {
			if err := checkFieldPath(app, base, search.SortQueryParam, field.Name, granted); err != nil {
				return err
			}
		}
	}
	return nil
}

// filterIdentifiers returns every identifier of a parsed filter: both sides of
// each expression, the nested groups, and the arguments of functions.
func filterIdentifiers(groups []fexpr.ExprGroup) []string {
	var out []string
	var fromToken func(t fexpr.Token)
	fromToken = func(t fexpr.Token) {
		switch t.Type {
		case fexpr.TokenIdentifier:
			out = append(out, t.Literal)
		case fexpr.TokenFunction:
			args, _ := t.Meta.([]fexpr.Token)
			for _, arg := range args {
				fromToken(arg)
			}
		}
	}
	for _, group := range groups {
		switch item := group.Item.(type) {
		case fexpr.Expr:
			fromToken(item.Left)
			fromToken(item.Right)
		case fexpr.ExprGroup:
			out = append(out, filterIdentifiers([]fexpr.ExprGroup{item})...)
		case []fexpr.ExprGroup:
			out = append(out, filterIdentifiers(item)...)
		}
	}
	return out
}

// checkFieldPath follows a dotted field path the way PocketBase's record field
// resolver does, and checks each collection the path joins. Only a segment
// before the last one joins: the last segment is a field of the collection
// reached so far.
func checkFieldPath(app core.App, base *core.Collection, param, path string, granted []string) error {
	if strings.HasPrefix(path, "@") {
		// A macro (@now) or a special sort key (@random). PocketBase refuses
		// @request. and @collection. paths from every caller but a superuser.
		return nil
	}
	props := strings.Split(path, ".")
	current := base
	for i, prop := range props[:len(props)-1] {
		field := current.Fields.GetByName(prop)
		var target *core.Collection
		switch {
		case field == nil:
			target = backRelationTarget(app, current, prop)
		case field.Type() == core.FieldTypeRelation:
			relation, _ := field.(*core.RelationField)
			if relation == nil {
				return nil
			}
			// "owner.id" reads the relation column itself and joins nothing.
			if !relation.IsMultiple() && i == len(props)-2 && props[i+1] == core.FieldNameId {
				return nil
			}
			target, _ = app.FindCachedCollectionByNameOrId(relation.CollectionId)
		}
		if target == nil {
			// A JSON path, or a path PocketBase cannot resolve: no join.
			return nil
		}
		if err := checkRelatedRead(target, param, path, granted); err != nil {
			return err
		}
		current = target
	}
	return nil
}

// backRelationTarget resolves "<collection>_via_<field>" to the collection
// that holds the relation back to from, or nil.
func backRelationTarget(app core.App, from *core.Collection, prop string) *core.Collection {
	matches := backRelationPattern.FindStringSubmatch(prop)
	if len(matches) != 3 {
		return nil
	}
	back, err := app.FindCachedCollectionByNameOrId(matches[1])
	if err != nil {
		return nil
	}
	field, _ := back.Fields.GetByName(matches[2]).(*core.RelationField)
	if field == nil || field.CollectionId != from.Id {
		return nil
	}
	return back
}
