package coreserver

import (
	"encoding/json"
	"testing"
)

func TestRebuildManifest_RoundTrip(t *testing.T) {
	m := RebuildManifest{
		BuildID: "build-1234",
		Members: []MemberSpec{
			{Slug: "tinycld", Version: "1.2.0", Spec: "git+https://github.com/tinycld/tinycld#v1.2.0"},
			{Slug: "mail", Version: "0.3.1", Spec: "@tinycld/mail@0.3.1"},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got RebuildManifest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.BuildID != "build-1234" || len(got.Members) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Members[1].Slug != "mail" || got.Members[1].Spec != "@tinycld/mail@0.3.1" {
		t.Fatalf("member mismatch: %+v", got.Members[1])
	}
}

func TestRebuildManifest_MemberBySlug(t *testing.T) {
	m := RebuildManifest{Members: []MemberSpec{{Slug: "mail"}, {Slug: "calc"}}}
	if ms, ok := m.MemberBySlug("calc"); !ok || ms.Slug != "calc" {
		t.Fatalf("MemberBySlug(calc) failed: %+v ok=%v", ms, ok)
	}
	if _, ok := m.MemberBySlug("absent"); ok {
		t.Fatal("MemberBySlug(absent) should be !ok")
	}
}
