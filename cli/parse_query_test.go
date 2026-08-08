package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

type goldenCase struct {
	Name    string   `json:"name"`
	Input   string   `json:"input"`
	Chips   []string `json:"chips"`
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type goldenFile struct {
	Slugs []string     `json:"slugs"`
	Cases []goldenCase `json:"cases"`
}

// The fixture is shared with the TypeScript parser's test
// (core/tests/unit/search-query-grammar-golden.test.ts). A grammar change that
// lands in only one language fails here or there — which is the whole point of
// there being one file rather than two lists of cases.
func TestParseQueryMatchesGoldenFixture(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "query-grammar.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var golden goldenFile
	if err := json.Unmarshal(raw, &golden); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(golden.Cases) == 0 {
		t.Fatal("fixture has no cases")
	}

	for _, tc := range golden.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got := parseQuery(tc.Input, golden.Slugs)
			if !equalTerms(got.Chips, tc.Chips) {
				t.Errorf("chips = %#v, want %#v", got.Chips, tc.Chips)
			}
			if !equalTerms(got.Include, tc.Include) {
				t.Errorf("include = %#v, want %#v", got.Include, tc.Include)
			}
			if !equalTerms(got.Exclude, tc.Exclude) {
				t.Errorf("exclude = %#v, want %#v", got.Exclude, tc.Exclude)
			}
		})
	}
}

// equalTerms treats nil and empty as the same: the fixture writes `[]` for an
// empty list and the parser returns a nil slice, and that difference is not a
// grammar difference.
func equalTerms(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}
