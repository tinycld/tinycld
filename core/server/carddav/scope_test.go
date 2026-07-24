package carddav

import "testing"

func TestSingleOrgScope_BookPath(t *testing.T) {
	s := singleOrgScope{bookSegment: "default"}
	b := s.book()
	if b.Path != "/carddav/u/ab/default/" {
		t.Errorf("book path = %q, want /carddav/u/ab/default/", b.Path)
	}
	if b.Name == "" {
		t.Error("book name should be non-empty")
	}
}

func TestHandlerFor_NilWhenNoSources(t *testing.T) {
	if h := HandlerFor(nil, nil); h != nil {
		t.Error("HandlerFor with no sources should return nil")
	}
}

func TestPrefixes(t *testing.T) {
	got := Prefixes()
	want := map[string]bool{"/carddav": true, "/.well-known/carddav": true}
	if len(got) != len(want) {
		t.Fatalf("Prefixes() = %v", got)
	}
	for _, p := range got {
		if !want[p] {
			t.Errorf("unexpected prefix %q", p)
		}
	}
}
