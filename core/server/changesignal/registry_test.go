package changesignal

import (
	"reflect"
	"testing"
)

func TestTopic(t *testing.T) {
	if got := Topic("zoo_animals"); got != "collection/zoo_animals" {
		t.Fatalf("Topic = %q", got)
	}
}

func TestTopicsForAPlainCollectionIsItsOwnTopic(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	want := []string{"collection/zoo_animals"}
	if got := topicsFor("zoo_animals"); !reflect.DeepEqual(got, want) {
		t.Fatalf("topicsFor = %v, want %v", got, want)
	}
}

func TestTopicsForFansOutToViewsTransitively(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	RegisterView("zoo_fed", "zoo_animals", "zoo_feedings")
	RegisterView("zoo_fed_named", "zoo_fed")
	RegisterView("zoo_staff", "zoo_keepers")

	want := []string{"collection/zoo_feedings", "collection/zoo_fed", "collection/zoo_fed_named"}
	if got := topicsFor("zoo_feedings"); !reflect.DeepEqual(got, want) {
		t.Fatalf("topicsFor(zoo_feedings) = %v, want %v", got, want)
	}
	want = []string{"collection/zoo_keepers", "collection/zoo_staff"}
	if got := topicsFor("zoo_keepers"); !reflect.DeepEqual(got, want) {
		t.Fatalf("topicsFor(zoo_keepers) = %v, want %v", got, want)
	}
}

func TestRegisterViewUnionsRepeatedCalls(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	RegisterView("zoo_fed", "zoo_feedings")
	RegisterView("zoo_fed", "zoo_animals", "zoo_feedings")
	want := map[string][]string{"zoo_fed": {"zoo_animals", "zoo_feedings"}}
	if got := RegisteredViews(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredViews = %v, want %v", got, want)
	}
}

func TestCoveredByExpandsRegisteredViewSources(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	RegisterView("zoo_fed", "zoo_animals", "zoo_feedings")
	RegisterView("zoo_fed_named", "zoo_fed")
	want := map[string]bool{"zoo_fed": true, "zoo_animals": true, "zoo_feedings": true}
	if got := coveredBy("zoo_fed_named"); !reflect.DeepEqual(got, want) {
		t.Fatalf("coveredBy = %v, want %v", got, want)
	}
}

func TestRegisterViewRefusesMalformedDeclarations(t *testing.T) {
	cases := map[string]func(){
		"blank view":   func() { RegisterView("", "zoo_animals") },
		"no sources":   func() { RegisterView("zoo_fed") },
		"blank source": func() { RegisterView("zoo_fed", "") },
		"reads itself": func() { RegisterView("zoo_fed", "zoo_fed") },
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			defer func() {
				if recover() == nil {
					t.Fatal("RegisterView did not panic")
				}
			}()
			register()
		})
	}
}

// Core itself declares no views: every view belongs to a package, which
// registers it from its own Register(app).
func TestCoreRegistersNoViews(t *testing.T) {
	if got := RegisteredViews(); len(got) != 0 {
		t.Fatalf("core registered views: %v", got)
	}
}

// SQLite compares table names without case, so the registry keys a virtual
// table by its lowercased name.
func TestRegisterVirtualTableUnionsRepeatedCalls(t *testing.T) {
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	RegisterVirtualTable("fts_zoo", "zoo_keepers")
	RegisterVirtualTable("FTS_Zoo", "zoo_animals", "zoo_keepers")
	want := map[string][]string{"fts_zoo": {"zoo_animals", "zoo_keepers"}}
	if got := RegisteredVirtualTables(); !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredVirtualTables = %v, want %v", got, want)
	}
}

func TestRegisterVirtualTableRefusesMalformedDeclarations(t *testing.T) {
	cases := map[string]func(){
		"blank table":      func() { RegisterVirtualTable("", "zoo_animals") },
		"no collections":   func() { RegisterVirtualTable("fts_zoo") },
		"blank collection": func() { RegisterVirtualTable("fts_zoo", "") },
		"maps itself":      func() { RegisterVirtualTable("fts_zoo", "FTS_ZOO") },
	}
	for name, register := range cases {
		t.Run(name, func(t *testing.T) {
			ResetForTesting()
			t.Cleanup(ResetForTesting)
			defer func() {
				if recover() == nil {
					t.Fatal("RegisterVirtualTable did not panic")
				}
			}()
			register()
		})
	}
}

func TestResetForTestingEmptiesTheVirtualTables(t *testing.T) {
	RegisterVirtualTable("fts_zoo", "zoo_keepers")
	ResetForTesting()
	if got := RegisteredVirtualTables(); len(got) != 0 {
		t.Fatalf("RegisteredVirtualTables = %v", got)
	}
}

// Core itself maps no virtual table: an index belongs to a package, which
// registers it (fts.RegisterSync does it for a full-text index).
func TestCoreRegistersNoVirtualTables(t *testing.T) {
	if got := RegisteredVirtualTables(); len(got) != 0 {
		t.Fatalf("core registered virtual tables: %v", got)
	}
}
