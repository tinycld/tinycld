package reports

import (
	"strings"
	"testing"
)

func validZooSource() Source {
	return Source{
		Key:        "zoo.feedings",
		Collection: "zoo_feedings",
		Label:      "Feedings",
		Fields: []Field{
			{Key: "cost", Label: "Cost", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "fed", Label: "Fed", Type: Date, Aggregates: MinMax},
			{Key: "kind", Label: "Kind", Type: Select, Options: []Option{{Value: "hay", Label: "Hay"}}},
		},
		Links: []Link{
			{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"},
		},
		Aliases: map[string]string{"price": "cost"},
	}
}

func TestRegisterSourceAddsTheIDFieldAndDefaultColumns(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	src := validZooSource()
	RegisterSource(src)
	got := lookupSource(bareApp(t), "zoo.feedings")
	if got == nil {
		t.Fatal("source not registered")
	}
	if id := got.field("id"); id == nil || id.Type != Relation || !id.allows(Count) || id.allows(Sum) {
		t.Fatalf("id field = %+v", id)
	}
	if c := got.field("cost"); c.Column != "cost" {
		t.Fatalf("cost column = %q, want the key", c.Column)
	}
	if src.Fields[0].Column != "" {
		t.Fatal("RegisterSource changed the caller's field slice")
	}
	if l := got.link("animal"); l == nil || l.Cardinality != ToOne {
		t.Fatalf("animal link = %+v", l)
	}
}

func TestRegisterSourceReplacesTheSameKey(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	RegisterSource(validZooSource())
	again := validZooSource()
	again.Label = "Meals"
	RegisterSource(again)
	all := Sources(bareApp(t))
	if len(all) != 1 || all[0].Label != "Meals" {
		t.Fatalf("Sources = %+v", all)
	}
}

func TestRegisterSourceRefusesMalformedSources(t *testing.T) {
	cases := map[string]struct {
		edit func(*Source)
		want string
	}{
		"key without package":    {func(s *Source) { s.Key = "feedings" }, "<package>.<name>"},
		"no collection":          {func(s *Source) { s.Collection = "" }, "collection"},
		"primary key":            {func(s *Source) { s.PrimaryKey = "code" }, "primary key"},
		"duplicate key":          {func(s *Source) { s.Links[0].Key = "cost" }, "used twice"},
		"unknown type":           {func(s *Source) { s.Fields[0].Type = "blob" }, "unknown type"},
		"sum of a date":          {func(s *Source) { s.Fields[2].Aggregates = SumAll }, "needs a number or money"},
		"select without options": {func(s *Source) { s.Fields[3].Options = nil }, "lists options"},
		"currency not text":      {func(s *Source) { s.Fields[0].Currency = "fed" }, "not a text field"},
		"money with no currency": {func(s *Source) { s.Fields[0].Currency = "" }, "names its currency"},
		"no cardinality":         {func(s *Source) { s.Links[0].Cardinality = "" }, "cardinality"},
		"no target":              {func(s *Source) { s.Links[0].Target = "" }, "target source"},
		"to-many without back field": {func(s *Source) {
			s.Links[0].Cardinality = ToMany
		}, "BackField"},
		"to-one with back field": {func(s *Source) { s.Links[0].BackField = "x" }, "no BackField"},
		"alias still a key":      {func(s *Source) { s.Aliases = map[string]string{"cost": "fed"} }, "still a key"},
		"alias to nowhere":       {func(s *Source) { s.Aliases = map[string]string{"price": "nope"} }, "not a field or link"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			src := validZooSource()
			src.Fields = append([]Field(nil), src.Fields...)
			src.Links = append([]Link(nil), src.Links...)
			c.edit(&src)
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("RegisterSource did not panic")
				}
				if msg, _ := r.(string); !strings.Contains(msg, c.want) {
					t.Fatalf("panic %q does not mention %q", r, c.want)
				}
			}()
			RegisterSource(src)
		})
	}
}

// Core itself registers no source: every source belongs to a package.
func TestCoreRegistersNoSources(t *testing.T) {
	if got := Sources(bareApp(t)); len(got) != 0 {
		t.Fatalf("core registered sources: %v", got)
	}
}
