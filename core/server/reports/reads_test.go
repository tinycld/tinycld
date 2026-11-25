package reports

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
)

// feedingsOfOne is feedingsFlat with a choice param over the feedings whose
// label crosses the link to the animals, which no column of the report reads.
const feedingsOfOne = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"params":[{"id":"one","type":"choice","label":"Feeding","header":"Feeding: {label}",
		"optionsFrom":{"source":"zoo.feedings","value":[{"field":"id"}],"label":[{"link":"animal"},{"field":"name"}]}}],
	"sort":[{"col":"note"}]}`

// A run names the collections its statements selected rows from, and not a
// collection that only a list rule read.
func TestARunNamesTheCollectionsItReadRowsFrom(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", feedingsByDay, Options{}, nil)
	if want := []string{"zoo_animals", "zoo_feedings"}; !slices.Equal(res.collections, want) {
		t.Errorf("collections = %v, want %v (zoo_keepers is read only by the feedings' list rule)", res.collections, want)
	}
	res = z.run(t, "ann", feedingsFlat, Options{}, nil)
	if want := []string{"zoo_feedings"}; !slices.Equal(res.collections, want) {
		t.Errorf("collections = %v, want %v", res.collections, want)
	}
	// The header line of a picked choice reads the pick's label by a statement
	// of its own.
	res = z.run(t, "ann", feedingsOfOne, Options{}, map[string]string{"one": strconv.Quote(z.id("f1"))})
	if want := []string{"Feeding: Leo"}; !slices.Equal(res.HeaderLines, want) {
		t.Fatalf("header lines = %v, want %v", res.HeaderLines, want)
	}
	if want := []string{"zoo_animals", "zoo_feedings"}; !slices.Equal(res.collections, want) {
		t.Errorf("collections = %v, want %v (the label of the pick is an animal's name)", res.collections, want)
	}
}

// The options of a choice param are read by statements of their own: the
// read names what they selected rows from.
func TestAnOptionsReadNamesTheCollectionsItReadRowsFrom(t *testing.T) {
	z := newZoo(t)
	spec, _, err := Load([]byte(feedingsOfOne))
	if err != nil {
		t.Fatal(err)
	}
	page, err := ReadParamOptions(context.Background(), z.app, OptionsRequest{Spec: spec, Param: "one", Auth: z.recs["ann"]})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Options) == 0 {
		t.Fatal("no options")
	}
	if want := []string{"zoo_animals", "zoo_feedings"}; !slices.Equal(page.collections, want) {
		t.Errorf("collections = %v, want %v", page.collections, want)
	}
}

// A failed options read still names what its statement selected from.
func TestAFailedOptionsReadNamesWhatItRead(t *testing.T) {
	z := newZoo(t)
	spec, _, err := Load([]byte(feedingsOfOne))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page, err := ReadParamOptions(ctx, z.app, OptionsRequest{Spec: spec, Param: "one", Auth: z.recs["ann"]})
	if err == nil {
		t.Fatal("a read under a cancelled context gave no error")
	}
	if want := []string{"zoo_animals", "zoo_feedings"}; page == nil || !slices.Equal(page.collections, want) || len(page.Options) != 0 {
		t.Errorf("the failed read gives %+v, want only the collections %v", page, want)
	}
}

// A run that stops because it has more rows than the read takes (the export's
// "too many rows") still names what it read up to there.
func TestARunPastItsRowsNamesWhatItRead(t *testing.T) {
	z := newZoo(t)
	// A grouped report: its group statement counts the rows before the details
	// are read.
	const byNote = `{"version":4,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
		"groups":[{"id":"g","col":"note"}]}`
	spec, info, err := Load([]byte(byNote))
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(context.Background(), z.app, Request{
		Spec: spec, Loaded: info, Auth: z.recs["ann"],
		Options: Options{Limit: 2, ExpandAll: true}, RowCap: 2, PageCap: 2, refuseOver: 1,
	})
	if !errors.Is(err, errPastRows) {
		t.Fatalf("err = %v, want errPastRows", err)
	}
	if res == nil || !slices.Contains(res.collections, "zoo_feedings") {
		t.Errorf("the stopped run names %v, want zoo_feedings in it", res)
	}
}

// pricedAnimal is feedingsFlat with a choice param over the animals whose
// label is an amount in the animals' fixed currency, which the source's
// Currency hook gives.
const pricedAnimal = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"note","path":[{"field":"note"}]},
	{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}],
	"params":[{"id":"animal","type":"choice","label":"Animal","header":"Worth {label}",
		"optionsFrom":{"source":"zoo.animals","value":[{"field":"id"}],"label":[{"field":"price"}]}}],
	"sort":[{"col":"note"}]}`

// watchVault makes the animals' hooks read the vault, as a package's hooks
// read its settings.
func watchVault(t *testing.T) {
	t.Helper()
	src := *lookupRegistered("zoo.animals")
	src.Watch = []string{"zoo_vault"}
	RegisterSource(src)
}

// A source's hooks (Heading, FiscalStartMonth, Currency) read the collections
// its Watch names. A read names them when a hook ran, also for a source that
// only the options of a param read; it does not name them for a source whose
// hooks did not run.
func TestAReadNamesTheWatchOfASourceWhoseHookRan(t *testing.T) {
	z := newZoo(t)
	withHeading(t, "Zoo Ltd", "zoo_sites")
	watchVault(t)
	res := z.run(t, "ann", feedingsFlat, Options{}, nil)
	if want := []string{"zoo_feedings", "zoo_sites"}; !slices.Equal(res.collections, want) {
		t.Errorf("heading hook: collections = %v, want %v", res.collections, want)
	}

	res = z.run(t, "ann", pricedAnimal, Options{}, map[string]string{"animal": strconv.Quote(z.id("leo"))})
	if len(res.HeaderLines) != 1 || res.HeaderLines[0] == "" {
		t.Fatalf("header lines = %v", res.HeaderLines)
	}
	if want := []string{"zoo_animals", "zoo_feedings", "zoo_sites", "zoo_vault"}; !slices.Equal(res.collections, want) {
		t.Errorf("currency hook of the options source: collections = %v, want %v", res.collections, want)
	}

	spec, _, err := Load([]byte(pricedAnimal))
	if err != nil {
		t.Fatal(err)
	}
	page, err := ReadParamOptions(context.Background(), z.app, OptionsRequest{Spec: spec, Param: "animal", Auth: z.recs["ann"]})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(page.collections, "zoo_vault") {
		t.Errorf("options read: collections = %v, want zoo_vault in it", page.collections)
	}
}

// A union source is read from each of its parts: the read names every part's
// collection.
func TestAUnionRunNamesEveryPartsCollection(t *testing.T) {
	z := newUnionZoo(t)
	res := z.run(t, "ann", mealsReport, Options{}, nil)
	if want := []string{"zoo_feedings", "zoo_treats"}; !slices.Equal(res.collections, want) {
		t.Errorf("collections = %v, want %v (the parts of zoo.meals)", res.collections, want)
	}
}

// A run that fails after it read rows still names what it read: its error
// (an amount too large to total, a timeout) says something of those rows.
func TestAFailedRunNamesWhatItRead(t *testing.T) {
	z := newZoo(t)
	if _, err := z.app.DB().NewQuery("UPDATE zoo_feedings SET cost = 9223372036854775807 WHERE note IN ('f1', 'f2')").Execute(); err != nil {
		t.Fatal(err)
	}
	res, err := z.try(t, "ann", feedingsByDay, Options{}, nil)
	if !errors.Is(err, ErrAmountTooLarge) {
		t.Fatalf("err = %v, want ErrAmountTooLarge", err)
	}
	if res == nil || !slices.Contains(res.collections, "zoo_feedings") || len(res.Rows) != 0 {
		t.Errorf("the failed run gives %+v, want only the collections it read", res)
	}
}
