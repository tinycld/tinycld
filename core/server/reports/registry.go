// Package reports is core's report engine. Packages register report sources
// (a collection, its fields and its links); a report spec picks a source and
// columns along those links, and the engine compiles it to one SQL statement
// per pass on the PocketBase database. Every collection the statement reads is
// filtered by the reader's own list rule, so a report never shows a number the
// reader's rules would hide.
//
// Core names no package; tests use fictional zoo_* collections.
package reports

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// FieldType sets which formats and aggregates a field offers.
type FieldType string

const (
	Text     FieldType = "text"
	Number   FieldType = "number"
	Money    FieldType = "money" // integer cents
	Date     FieldType = "date"
	DateTime FieldType = "datetime"
	Bool     FieldType = "bool"
	Select   FieldType = "select"
	Relation FieldType = "relation" // a record id
	// Percent is never a field's type: percentOfParent and percentOfTotal
	// return it, and the shell formats it.
	Percent FieldType = "percent"
)

// Aggregate is how a column summarises in group, subtotal and total rows.
type Aggregate string

const (
	Sum           Aggregate = "sum"
	Count         Aggregate = "count"
	CountDistinct Aggregate = "countDistinct"
	Min           Aggregate = "min"
	Max           Aggregate = "max"
	Avg           Aggregate = "avg"
)

// Common aggregate sets. A field with no Aggregates can be counted.
var (
	SumAll    = []Aggregate{Sum, Avg, Min, Max, Count, CountDistinct}
	MinMax    = []Aggregate{Min, Max, Count, CountDistinct}
	CountOnly = []Aggregate{Count, CountDistinct}
)

// Cardinality says how many target rows a link reaches from one source row.
type Cardinality string

const (
	ToOne  Cardinality = "toOne"
	ToMany Cardinality = "toMany"
)

// Option is one choice of a Select field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field is one column a report may show.
type Field struct {
	Key    string // stable; renaming it needs an alias
	Column string // the collection column; "" means Key
	Label  string
	Type   FieldType
	// Aggregates this field offers; nil means CountOnly.
	Aggregates []Aggregate
	// Currency, for a Money field, is the key of a Text field of the same
	// source that holds each row's currency code. "" uses Source.Currency.
	Currency string
	// Options label a Select field's values.
	Options []Option
}

// Link is a path from one source to another.
type Link struct {
	Key         string
	Label       string
	Target      string // the target source's key
	Cardinality Cardinality
	// Field is, for ToOne, the single relation column on this source.
	Field string
	// BackField is, for ToMany, the single relation column on the target
	// that points back at this source.
	BackField string
}

// Source is a collection a report can start from or reach through a link.
type Source struct {
	Key        string // "<package>.<name>", stable
	Collection string
	Label      string
	// PrimaryKey must be "" or "id": PocketBase relations point at id.
	PrimaryKey string
	Fields     []Field
	Links      []Link
	// Aliases maps an old field or link key to its new key, so a saved
	// report keeps working after a rename.
	Aliases map[string]string
	// Currency is the fixed currency of Money fields that name no Currency
	// field. nil means the currency is unknown to the engine.
	Currency func(app core.App) string
	// FiscalStartMonth (1-12) drives fiscalYear, fiscalPeriod and the
	// fiscal period presets when this source is a report's base. nil means 1.
	FiscalStartMonth func(app core.App) int
}

var (
	sourceKeyRe = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`)
	memberKeyRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
)

// idField is offered on every source, so a report can count linked rows.
var idField = Field{Key: "id", Column: "id", Label: "ID", Type: Relation, Aggregates: CountOnly}

var (
	mu      sync.RWMutex
	sources = map[string]*Source{}
	refused = map[string]string{}
)

// RegisterSource adds a package's source. Call it from the package's
// Register(app). Registering a key again replaces the source, so a dev
// reload does not duplicate it. A malformed source panics at boot: it is a
// wiring mistake in the package's code. What needs the database (the
// collection, its columns, its list rule) is checked when the server starts;
// see CheckSources.
func RegisterSource(src Source) {
	// The registry keeps its own copies: validation fills in default columns.
	src.Fields = slices.Clone(src.Fields)
	src.Links = slices.Clone(src.Links)
	src.Aliases = maps.Clone(src.Aliases)
	if err := validateSource(&src); err != nil {
		panic(fmt.Sprintf("reports.RegisterSource(%q): %v", src.Key, err))
	}
	if !slices.ContainsFunc(src.Fields, func(f Field) bool { return f.Key == idField.Key }) {
		src.Fields = append([]Field{idField}, src.Fields...)
	}
	mu.Lock()
	defer mu.Unlock()
	sources[src.Key] = &src
	delete(refused, src.Key)
}

func validateSource(src *Source) error {
	if !sourceKeyRe.MatchString(src.Key) {
		return fmt.Errorf("key must look like \"<package>.<name>\"")
	}
	if src.Collection == "" || src.Label == "" {
		return fmt.Errorf("a source needs a collection and a label")
	}
	if src.PrimaryKey != "" && src.PrimaryKey != "id" {
		return fmt.Errorf("primary key must be id: PocketBase relations point at id")
	}
	keys := map[string]bool{}
	claim := func(key string) error {
		if !memberKeyRe.MatchString(key) {
			return fmt.Errorf("invalid key %q", key)
		}
		if keys[key] {
			return fmt.Errorf("key %q is used twice", key)
		}
		keys[key] = true
		return nil
	}
	for i := range src.Fields {
		f := &src.Fields[i]
		if err := claim(f.Key); err != nil {
			return err
		}
		if f.Column == "" {
			f.Column = f.Key
		}
		if err := validateField(src, f); err != nil {
			return fmt.Errorf("field %q: %w", f.Key, err)
		}
	}
	for _, l := range src.Links {
		if err := claim(l.Key); err != nil {
			return err
		}
		if err := validateLink(l); err != nil {
			return fmt.Errorf("link %q: %w", l.Key, err)
		}
	}
	for from, to := range src.Aliases {
		if keys[from] {
			return fmt.Errorf("alias %q is still a key", from)
		}
		if !keys[to] && to != idField.Key {
			return fmt.Errorf("alias %q points at %q, which is not a field or link", from, to)
		}
	}
	return nil
}

func validateField(src *Source, f *Field) error {
	if f.Label == "" {
		return fmt.Errorf("a field needs a label")
	}
	switch f.Type {
	case Text, Number, Money, Date, DateTime, Bool, Select, Relation:
	default:
		return fmt.Errorf("unknown type %q", f.Type)
	}
	numeric := f.Type == Number || f.Type == Money
	for _, a := range f.Aggregates {
		switch a {
		case Sum, Avg:
			if !numeric {
				return fmt.Errorf("%s needs a number or money field", a)
			}
		case Count, CountDistinct, Min, Max:
		default:
			return fmt.Errorf("unknown aggregate %q", a)
		}
	}
	if (f.Type == Select) != (len(f.Options) > 0) {
		return fmt.Errorf("a select field, and only a select field, lists options")
	}
	if f.Currency != "" {
		if f.Type != Money {
			return fmt.Errorf("only a money field names a currency field")
		}
		i := slices.IndexFunc(src.Fields, func(c Field) bool { return c.Key == f.Currency })
		if i < 0 || src.Fields[i].Type != Text {
			return fmt.Errorf("currency field %q is not a text field of this source", f.Currency)
		}
	}
	// The engine never adds amounts it cannot tell apart by currency.
	if f.Type == Money && f.Currency == "" && src.Currency == nil {
		return fmt.Errorf("a money field names its currency field, or the source sets Currency")
	}
	return nil
}

func validateLink(l Link) error {
	if l.Label == "" || l.Target == "" {
		return fmt.Errorf("a link needs a label and a target source")
	}
	switch l.Cardinality {
	case ToOne:
		if l.Field == "" || l.BackField != "" {
			return fmt.Errorf("a to-one link names its relation Field and no BackField")
		}
	case ToMany:
		if l.BackField == "" || l.Field != "" {
			return fmt.Errorf("a to-many link names the target's BackField and no Field")
		}
	default:
		return fmt.Errorf("a link declares its cardinality (ToOne or ToMany)")
	}
	return nil
}

// Sources returns the sources a report may use, sorted by key: every
// registered source CheckSources has not refused.
func Sources() []Source {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Source, 0, len(sources))
	for key, src := range sources {
		if _, bad := refused[key]; !bad {
			out = append(out, *src)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// Refused returns the sources CheckSources refused, each with its reason.
func Refused() map[string]string {
	mu.RLock()
	defer mu.RUnlock()
	return maps.Clone(refused)
}

// lookupSource returns a usable source, or nil for an unknown or refused key.
func lookupSource(key string) *Source {
	mu.RLock()
	defer mu.RUnlock()
	if _, bad := refused[key]; bad {
		return nil
	}
	return sources[key]
}

// field and link find a member by key. They do not follow aliases.
func (s *Source) field(key string) *Field {
	for i := range s.Fields {
		if s.Fields[i].Key == key {
			return &s.Fields[i]
		}
	}
	return nil
}

func (s *Source) link(key string) *Link {
	for i := range s.Links {
		if s.Links[i].Key == key {
			return &s.Links[i]
		}
	}
	return nil
}

func (f *Field) allows(a Aggregate) bool {
	if f.Aggregates == nil {
		return slices.Contains(CountOnly, a)
	}
	return slices.Contains(f.Aggregates, a)
}

// ResetForTesting empties the registry and the shipped reports. Tests
// register their own.
func ResetForTesting() {
	mu.Lock()
	sources = map[string]*Source{}
	refused = map[string]string{}
	mu.Unlock()
	shippedMu.Lock()
	shipped = map[string]Shipped{}
	shippedMu.Unlock()
}
