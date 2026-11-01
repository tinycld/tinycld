package reports

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// CheckSources checks every registered source against app's database: its
// collection and columns exist with matching types, no column is hidden from
// the API, each link follows a single relation to its target, and the
// collection's list rule can be built for a reader. A source that fails, or
// links to one that fails, is refused for app: Sources(app) leaves it out and
// a report on app that names it gets an unknown-source error. Each call
// replaces app's refusals and changes no other app's. The result names every
// refused source and why.
//
// The server runs it once for each app at start, after migrations. A package
// calls it in its tests after its migrations apply, so a broken source fails
// there.
func CheckSources(app core.App) error {
	mu.RLock()
	defer mu.RUnlock()
	refused := map[string]string{}
	keys := make([]string, 0, len(sources))
	for key := range sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if problems := checkSource(app, sources[key]); len(problems) > 0 {
			refused[key] = strings.Join(problems, "; ")
		}
	}
	// A link to a refused source is a broken link.
	for changed := true; changed; {
		changed = false
		for _, key := range keys {
			if _, bad := refused[key]; bad {
				continue
			}
			for _, l := range sources[key].Links {
				if reason, bad := refused[l.Target]; bad && sources[l.Target] != nil {
					refused[key] = fmt.Sprintf("link %q targets source %q, which is refused (%s)", l.Key, l.Target, reason)
					changed = true
					break
				}
			}
		}
	}
	refusedKey.Set(app, refused)
	if len(refused) == 0 {
		return nil
	}
	lines := make([]string, 0, len(refused))
	for _, key := range keys {
		if reason, bad := refused[key]; bad {
			lines = append(lines, fmt.Sprintf("source %q: %s", key, reason))
		}
	}
	return errors.New("reports: " + strings.Join(lines, "; "))
}

// checkSource runs under mu (read).
func checkSource(app core.App, src *Source) []string {
	col, err := app.FindCollectionByNameOrId(src.Collection)
	if err != nil {
		return []string{fmt.Sprintf("collection %q does not exist", src.Collection)}
	}
	if col.Name != src.Collection {
		return []string{fmt.Sprintf("collection %q must be named %q, not by id", src.Collection, col.Name)}
	}
	if col.System {
		return []string{fmt.Sprintf("collection %q is a system collection", col.Name)}
	}
	var problems []string
	for _, f := range src.Fields {
		if msg := checkField(col, f); msg != "" {
			problems = append(problems, fmt.Sprintf("field %q: %s", f.Key, msg))
		}
	}
	for _, l := range src.Links {
		if msg := checkLink(app, src, col, l); msg != "" {
			problems = append(problems, fmt.Sprintf("link %q: %s", l.Key, msg))
		}
	}
	if msg := checkRule(app, col); msg != "" {
		problems = append(problems, msg)
	}
	return problems
}

func checkField(col *core.Collection, f Field) string {
	if f.Column == "id" {
		if f.Type != Relation && f.Type != Text {
			return "the id column is a relation or text field"
		}
		return ""
	}
	if col.IsAuth() && f.Column == core.FieldNameEmail {
		// The rule CTE selects every column, but PocketBase shows an email
		// only when its owner allows it (emailVisibility).
		return `column "email" is visible only by emailVisibility`
	}
	pf := col.Fields.GetByName(f.Column)
	if pf == nil {
		return fmt.Sprintf("column %q does not exist", f.Column)
	}
	if pf.GetHidden() {
		return fmt.Sprintf("column %q is hidden from the API", f.Column)
	}
	if !slices.Contains(compatible[f.Type], pf.Type()) {
		return fmt.Sprintf("column %q is a %s field, which a %s field cannot read", f.Column, pf.Type(), f.Type)
	}
	switch x := pf.(type) {
	case *core.SelectField:
		if x.IsMultiple() {
			return fmt.Sprintf("column %q holds many values", f.Column)
		}
		for _, o := range f.Options {
			if !slices.Contains(x.Values, o.Value) {
				return fmt.Sprintf("option %q is not a value of column %q", o.Value, f.Column)
			}
		}
	case *core.RelationField:
		if x.IsMultiple() {
			return fmt.Sprintf("column %q holds many values", f.Column)
		}
	}
	return ""
}

var compatible = map[FieldType][]string{
	Text:     {core.FieldTypeText, core.FieldTypeEmail, core.FieldTypeURL, core.FieldTypeEditor},
	Number:   {core.FieldTypeNumber},
	Money:    {core.FieldTypeNumber},
	Date:     {core.FieldTypeDate, core.FieldTypeAutodate},
	DateTime: {core.FieldTypeDate, core.FieldTypeAutodate},
	Bool:     {core.FieldTypeBool},
	Select:   {core.FieldTypeSelect},
	Relation: {core.FieldTypeRelation},
}

func checkLink(app core.App, src *Source, col *core.Collection, l Link) string {
	target := sources[l.Target]
	if target == nil {
		return fmt.Sprintf("target source %q is not registered", l.Target)
	}
	targetCol, err := app.FindCollectionByNameOrId(target.Collection)
	if err != nil {
		return fmt.Sprintf("target collection %q does not exist", target.Collection)
	}
	owner, column, pointsAt := col, l.Field, targetCol
	if l.Cardinality == ToMany {
		owner, column, pointsAt = targetCol, l.BackField, col
	}
	rel, ok := owner.Fields.GetByName(column).(*core.RelationField)
	if !ok {
		return fmt.Sprintf("%q is not a relation field of %q", column, owner.Name)
	}
	if rel.IsMultiple() {
		return fmt.Sprintf("relation %q of %q holds many values", column, owner.Name)
	}
	if rel.GetHidden() {
		return fmt.Sprintf("relation %q of %q is hidden from the API", column, owner.Name)
	}
	if rel.CollectionId != pointsAt.Id {
		return fmt.Sprintf("relation %q of %q does not point at %q", column, owner.Name, pointsAt.Name)
	}
	return ""
}

// checkRule builds the list rule for a reader who exists only for the check,
// then has SQLite compile the result.
func checkRule(app core.App, col *core.Collection) string {
	users, err := app.FindCachedCollectionByNameOrId("users")
	if err != nil {
		return "no users collection to check the list rule with"
	}
	reader := core.NewRecord(users)
	reader.Id = "reportcheck0000"
	sql, params, err := ruleSelect(app, col, listRequestInfo(reader, nil))
	if err != nil {
		return err.Error()
	}
	if _, err := app.DB().NewQuery("EXPLAIN " + sql).Bind(params).Execute(); err != nil {
		return fmt.Sprintf("the list rule of %q does not compile: %v", col.Name, err)
	}
	return ""
}
