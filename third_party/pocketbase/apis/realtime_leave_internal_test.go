package apis

import (
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestRealtimeExprIdentifierRoots(t *testing.T) {
	scenarios := []struct {
		expr     string
		expected []string
	}{
		{"", nil},
		{"invalid ==== expr", nil},
		{"status = 'open'", []string{"status"}},
		{"rel.name ~ 'x' && tags:length > 1", []string{"rel", "tags"}},
		{"@request.auth.id != '' && @collection.users.role ?= 'admin'", nil},
		{"posts_via_author.title = 'a' || (a.b = 1 && c = 2)", []string{"a", "c", "posts_via_author"}},
		{"// comment\n status = 'open'", []string{"status"}},
	}

	for _, s := range scenarios {
		t.Run(s.expr, func(t *testing.T) {
			roots := realtimeExprIdentifierRoots(s.expr)
			slices.Sort(roots)

			if !slices.Equal(roots, s.expected) {
				t.Fatalf("expected %v, got %v", s.expected, roots)
			}
		})
	}
}

func TestRealtimeVisibilityDependsOn(t *testing.T) {
	changed := map[string]struct{}{"status": {}, "updated": {}}

	scenarios := []struct {
		name     string
		rule     *string
		filter   string
		expected bool
	}{
		{"superuser only, no filter", nil, "", false},
		{"superuser only, filter reads status", nil, "status = 'x'", true},
		{"public, no filter", types.Pointer(""), "", false},
		{"rule reads note", types.Pointer("note = 'x'"), "", false},
		{"rule reads status", types.Pointer("status = 'x'"), "", true},
		{"rule reads auth, filter reads note", types.Pointer("@request.auth.id != ''"), "note = 'x'", false},
		{"rule reads auth, filter reads status", types.Pointer("@request.auth.id != ''"), "status = 'x'", true},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			if got := realtimeVisibilityDependsOn(s.rule, s.filter, changed); got != s.expected {
				t.Fatalf("expected %v, got %v", s.expected, got)
			}
		})
	}
}

func TestRealtimeChangedFields(t *testing.T) {
	collection := core.NewBaseCollection("test")
	collection.Fields.Add(
		&core.TextField{Name: "title"},
		&core.TextField{Name: "note"},
		&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
	)

	record := core.NewRecord(collection)
	record.SetRaw("id", "abc")
	record.Set("title", "a")
	record.Set("note", "n")
	if err := record.PostScan(); err != nil {
		t.Fatal(err)
	}

	assertChanged := func(label string, expected []string) {
		t.Helper()

		changed := realtimeChangedFields(record)

		actual := make([]string, 0, len(changed))
		for name := range changed {
			actual = append(actual, name)
		}
		slices.Sort(actual)

		if !slices.Equal(actual, expected) {
			t.Fatalf("[%s] expected %v, got %v", label, expected, actual)
		}
	}

	// autodate OnUpdate fields are always reported as changed
	assertChanged("untouched", []string{"updated"})

	record.Set("title", "a")
	assertChanged("same value", []string{"updated"})

	record.Set("title", "b")
	assertChanged("new value", []string{"title", "updated"})
}
