package fts

import "testing"

func TestCoerce(t *testing.T) {
	cases := []struct {
		raw  string
		typ  string
		want any
	}{
		{"1", "bool", true},
		{"0", "bool", false},
		{"true", "bool", true},
		{"false", "bool", false},
		{"", "bool", false},
		{"42", "number", float64(42)},
		{"3.5", "number", 3.5},
		{"", "number", 0},
		{"nope", "number", 0},
		{"hi", "", "hi"},
		{"hi", "string", "hi"},
	}
	for _, tc := range cases {
		if got := coerce(tc.raw, tc.typ); got != tc.want {
			t.Errorf("coerce(%q, %q) = %v (%T), want %v (%T)", tc.raw, tc.typ, got, got, tc.want, tc.want)
		}
	}
}
