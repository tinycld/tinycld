package fts

import "testing"

func TestOwnerScopeClause(t *testing.T) {
	s := OwnerScope{Field: "owner"}
	if got := s.clause(); got != "c.owner IN ({:scopeUser})" {
		t.Errorf("clause() = %q", got)
	}
	if got := s.params("u1")["scopeUser"]; got != "u1" {
		t.Errorf("params()[scopeUser] = %v", got)
	}
}

func TestMemberScopeClause(t *testing.T) {
	s := MemberScope{
		Table:       "cards_project_members",
		MemberField: "project",
		UserField:   "user",
		RecordField: "project",
	}
	want := "c.project IN (SELECT project FROM cards_project_members WHERE user = {:scopeUser})"
	if got := s.clause(); got != want {
		t.Errorf("clause() = %q, want %q", got, want)
	}
	if got := s.params("u1")["scopeUser"]; got != "u1" {
		t.Errorf("params()[scopeUser] = %v", got)
	}
}

func TestExcludeClause(t *testing.T) {
	if got := excludeClause(Config{}); got != "" {
		t.Errorf("no ExcludeField should emit nothing, got %q", got)
	}
	if got := excludeClause(Config{ExcludeField: "archived"}); got != " AND c.archived != true" {
		t.Errorf("excludeClause = %q", got)
	}
}

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
