package reports

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// CheckSources checks every registered source against app's database: its
// collection and columns exist with matching types, no column is hidden from
// the API, each link follows a single relation to its target (or, for a
// to-one link between two unions, a text column that holds the target row's
// id), and the collection's list rule can be built for a reader. A source
// that fails, or links to one that fails, is refused for app: Sources(app)
// leaves it out and a report on app that names it gets an unknown-source
// error. Each call replaces app's refusals and changes no other app's. The
// result names every refused source and why.
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
		var problems []string
		problems = append(problems, checkSource(app, sources[key])...)
		problems = append(problems, checkSameCurrency(sources[key])...)
		if len(problems) > 0 {
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

// checkSameCurrency checks each SameCurrencyAs of src: it names a text field
// of a registered source that a money field there names as its currency, and
// that field declares none itself, so two fields that share a currency have
// one name (currencyField). It runs under mu (read) and reads no database.
func checkSameCurrency(src *Source) []string {
	var problems []string
	for _, f := range src.Fields {
		if f.SameCurrencyAs == "" {
			continue
		}
		dot := strings.LastIndex(f.SameCurrencyAs, ".")
		target, key := sources[f.SameCurrencyAs[:dot]], f.SameCurrencyAs[dot+1:]
		msg := ""
		switch {
		case target == nil:
			msg = fmt.Sprintf("same-currency source %q is not registered", f.SameCurrencyAs[:dot])
		case target.field(key) == nil || target.field(key).Type != Text ||
			!slices.ContainsFunc(target.Fields, func(m Field) bool { return m.Type == Money && m.Currency == key }):
			msg = fmt.Sprintf("%q is not the currency field of a money field", f.SameCurrencyAs)
		case target.field(key).SameCurrencyAs != "":
			msg = fmt.Sprintf("%q names %q itself: name that field", f.SameCurrencyAs, target.field(key).SameCurrencyAs)
		}
		if msg != "" {
			problems = append(problems, fmt.Sprintf("field %q: %s", f.Key, msg))
		}
	}
	return problems
}

// checkSource runs under mu (read).
func checkSource(app core.App, src *Source) []string {
	if len(src.Parts) > 0 {
		return checkUnionSource(app, src)
	}
	col, msg := partCollection(app, src.Collection)
	if msg != "" {
		return []string{msg}
	}
	var problems []string
	for _, f := range src.Fields {
		if msg := checkField(col, f); msg != "" {
			problems = append(problems, fmt.Sprintf("field %q: %s", f.Key, msg))
		}
	}
	for _, l := range src.Links {
		if msg := checkLink(app, src, l); msg != "" {
			problems = append(problems, fmt.Sprintf("link %q: %s", l.Key, msg))
		}
	}
	problems = append(problems, checkWatch(app, src)...)
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
	want := compatible[f.Type]
	if f.TextDay {
		want = []string{core.FieldTypeText}
	}
	if !slices.Contains(want, pf.Type()) {
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
	// A view's computed column is text, never select (T3); its stored values
	// are not checked against the options, and one no option names shows as
	// stored (R16).
	Select:   {core.FieldTypeSelect, core.FieldTypeText},
	Relation: {core.FieldTypeRelation},
}

// partCollection finds a collection a source reads, by its name, and refuses
// a system collection.
func partCollection(app core.App, name string) (*core.Collection, string) {
	col, err := app.FindCollectionByNameOrId(name)
	if err != nil {
		return nil, fmt.Sprintf("collection %q does not exist", name)
	}
	if col.Name != name {
		return nil, fmt.Sprintf("collection %q must be named %q, not by id", name, col.Name)
	}
	if col.System {
		return nil, fmt.Sprintf("collection %q is a system collection", col.Name)
	}
	return col, ""
}

func checkWatch(app core.App, src *Source) []string {
	var problems []string
	for _, w := range src.Watch {
		wc, err := app.FindCollectionByNameOrId(w)
		switch {
		case err != nil:
			problems = append(problems, fmt.Sprintf("watched collection %q does not exist", w))
		case wc.Name != w:
			problems = append(problems, fmt.Sprintf("watched collection %q must be named %q, not by id", w, wc.Name))
		case wc.System:
			problems = append(problems, fmt.Sprintf("watched collection %q is a system collection", w))
		}
	}
	return problems
}

// checkUnionSource checks each part of a union as checkSource checks a
// source's one collection, and that the parts agree: every part that maps a
// key maps it to a column of one PocketBase type, so the union's column has
// one type. A part collection must have a list rule (R2).
func checkUnionSource(app core.App, src *Source) []string {
	cols := make([]*core.Collection, len(src.Parts))
	for i, p := range src.Parts {
		col, msg := partCollection(app, p.Collection)
		if msg != "" {
			return []string{msg}
		}
		cols[i] = col
	}
	var problems []string
	for _, f := range src.Fields {
		if f.Key == idField.Key {
			continue
		}
		first, firstType := "", ""
		for i, p := range src.Parts {
			column, ok := p.Columns[f.Key]
			if !ok {
				continue
			}
			mapped := f
			mapped.Column = column
			if msg := checkField(cols[i], mapped); msg != "" {
				problems = append(problems, fmt.Sprintf("field %q: part %q: %s", f.Key, p.Collection, msg))
				continue
			}
			typ := cols[i].Fields.GetByName(column).Type()
			if first == "" {
				first, firstType = fmt.Sprintf("column %q of %q", column, p.Collection), typ
			} else if typeClass(typ) != typeClass(firstType) {
				problems = append(problems, fmt.Sprintf("field %q: column %q of %q is of type %s, but %s is of type %s",
					f.Key, column, p.Collection, typ, first, firstType))
			}
		}
	}
	// A key that is not a field (a link's Field, a BackField of a link from
	// another source) is projected too, so its column must exist.
	isField := map[string]bool{}
	for _, f := range src.Fields {
		isField[f.Key] = true
	}
	for i, p := range src.Parts {
		for _, key := range slices.Sorted(maps.Keys(p.Columns)) {
			column := p.Columns[key]
			if !isField[key] && cols[i].Fields.GetByName(column) == nil {
				problems = append(problems, fmt.Sprintf("key %q: part %q: column %q does not exist", key, p.Collection, column))
			}
		}
	}
	for _, l := range src.Links {
		if msg := checkLink(app, src, l); msg != "" {
			problems = append(problems, fmt.Sprintf("link %q: %s", l.Key, msg))
		}
	}
	problems = append(problems, checkWatch(app, src)...)
	for _, col := range cols {
		if col.ListRule == nil {
			problems = append(problems, fmt.Sprintf("part collection %q has no list rule: only superusers may list it", col.Name))
			continue
		}
		if msg := checkRule(app, col); msg != "" {
			problems = append(problems, msg)
		}
	}
	return problems
}

// typeClass groups the PocketBase types that fit one report Field type, so
// parts may mix them: days stored as date or autodate, strings stored as
// text, email or url. Any other type is its own class.
func typeClass(t string) string {
	switch t {
	case core.FieldTypeDate, core.FieldTypeAutodate:
		return core.FieldTypeDate
	case core.FieldTypeText, core.FieldTypeEmail, core.FieldTypeURL:
		return core.FieldTypeText
	}
	return t
}

// ownedColumn is a stored column (relation, or text id) a link follows: its
// collection and column name.
type ownedColumn struct{ collection, column string }

// linkOwners are the stored columns a source's key reads: on a one-collection
// source the column of that name (a link's Field or BackField is a column
// there), on a union each part's mapped column (a part that maps none has
// none).
func linkOwners(src *Source, key string) []ownedColumn {
	if len(src.Parts) == 0 {
		return []ownedColumn{{src.Collection, key}}
	}
	var out []ownedColumn
	for _, p := range src.Parts {
		if column, ok := p.Columns[key]; ok {
			out = append(out, ownedColumn{p.Collection, column})
		}
	}
	return out
}

func sourceCollections(app core.App, src *Source) ([]*core.Collection, string) {
	names := src.collections()
	out := make([]*core.Collection, 0, len(names))
	for _, name := range names {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			return nil, fmt.Sprintf("collection %q does not exist", name)
		}
		out = append(out, col)
	}
	return out, ""
}

// checkLink checks the stored relations a link follows: a to-one link's Field
// on this source, a to-many link's BackField on its target, each a single
// visible relation that points at the other side. On a union the relation is
// every part's column for that key, and the other side's collection is any of
// its parts'. A to-one link between two unions may instead read a visible
// text column of a part that holds the id of a row of the target.
func checkLink(app core.App, src *Source, l Link) string {
	target := sources[l.Target]
	if target == nil {
		return fmt.Sprintf("target source %q is not registered", l.Target)
	}
	targetCols, msg := sourceCollections(app, target)
	if msg != "" {
		return "target " + msg
	}
	srcCols, msg := sourceCollections(app, src)
	if msg != "" {
		return msg
	}
	owners, pointsAt := linkOwners(src, l.Field), targetCols
	if l.Cardinality == ToMany {
		owners, pointsAt = linkOwners(target, l.BackField), srcCols
		if len(owners) == 0 {
			return fmt.Sprintf("no part of %q maps %q", l.Target, l.BackField)
		}
	} else if len(owners) == 0 {
		return fmt.Sprintf("no part of %q maps %q", src.Key, l.Field)
	}
	// A relation points at one collection, so a union part cannot link by one
	// to a row of whichever target part holds it. On a to-one link between
	// two unions a part may store that id in a text column instead. The run
	// joins it to the target union's rule rows by id, as it joins a relation,
	// so the target parts' list rules still decide whether the row links.
	textOK := l.Cardinality == ToOne && len(src.Parts) > 0 && len(target.Parts) > 0
	for _, o := range owners {
		owner, err := app.FindCollectionByNameOrId(o.collection)
		if err != nil {
			return fmt.Sprintf("collection %q does not exist", o.collection)
		}
		if msg := relationCheck(owner, o.column, pointsAt, textOK); msg != "" {
			return msg
		}
	}
	return ""
}

// relationCheck checks one stored column a link follows: a single visible
// relation that points at a collection of pointsAt, or, when textOK, a
// visible text column that holds such a row's id.
func relationCheck(owner *core.Collection, column string, pointsAt []*core.Collection, textOK bool) string {
	field := owner.Fields.GetByName(column)
	if field == nil {
		return fmt.Sprintf("column %q of %q does not exist", column, owner.Name)
	}
	if text, isText := field.(*core.TextField); isText && textOK {
		if text.GetHidden() {
			return fmt.Sprintf("text %q of %q is hidden from the API", column, owner.Name)
		}
		return ""
	}
	rel, ok := field.(*core.RelationField)
	if !ok && textOK {
		return fmt.Sprintf("%q is not a relation or text field of %q", column, owner.Name)
	}
	if !ok {
		return fmt.Sprintf("%q is not a relation field of %q", column, owner.Name)
	}
	if rel.IsMultiple() {
		return fmt.Sprintf("relation %q of %q holds many values", column, owner.Name)
	}
	if rel.GetHidden() {
		return fmt.Sprintf("relation %q of %q is hidden from the API", column, owner.Name)
	}
	if !slices.ContainsFunc(pointsAt, func(c *core.Collection) bool { return c.Id == rel.CollectionId }) {
		names := make([]string, len(pointsAt))
		for i, c := range pointsAt {
			names[i] = strconv.Quote(c.Name)
		}
		return fmt.Sprintf("relation %q of %q does not point at %s", column, owner.Name, strings.Join(names, " or "))
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
