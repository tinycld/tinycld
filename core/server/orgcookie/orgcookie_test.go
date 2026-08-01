package orgcookie

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func decodeValue(t *testing.T, value string) []Entry {
	t.Helper()
	raw, err := url.QueryUnescape(value)
	if err != nil {
		t.Fatalf("unescape: %v", err)
	}
	var entries []Entry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		t.Fatalf("unmarshal %q: %v", raw, err)
	}
	return entries
}

func encodeValue(t *testing.T, entries any) string {
	t.Helper()
	body, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return url.QueryEscape(string(body))
}

func TestMerge_AddsToEmpty(t *testing.T) {
	v := Merge("", Entry{Slug: "acme", Name: "Acme Inc"})
	entries := decodeValue(t, v)
	if len(entries) != 1 || entries[0].Slug != "acme" || entries[0].Name != "Acme Inc" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestMerge_UpsertsAndFronts(t *testing.T) {
	v := Merge("", Entry{Slug: "acme", Name: "Old Name"})
	v = Merge(v, Entry{Slug: "beta", Name: "Beta"})
	// Re-auth on acme with a renamed org: entry updates and moves to front.
	v = Merge(v, Entry{Slug: "acme", Name: "New Name"})

	entries := decodeValue(t, v)
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %+v", entries)
	}
	if entries[0].Slug != "acme" || entries[0].Name != "New Name" {
		t.Fatalf("upserted entry should lead with the new name: %+v", entries[0])
	}
	if entries[1].Slug != "beta" {
		t.Fatalf("existing entry lost: %+v", entries)
	}
}

func TestMerge_DiscardsMalformedExisting(t *testing.T) {
	v := Merge("%%%not-a-cookie", Entry{Slug: "acme", Name: "Acme"})
	entries := decodeValue(t, v)
	if len(entries) != 1 || entries[0].Slug != "acme" {
		t.Fatalf("entries = %+v", entries)
	}
}

// The cookie is writable by JS on any sibling tenant. An entry whose slug is
// not a single DNS label ("evil.example/x", "a.b") is a planted value aimed at
// steering the client's slug→URL derivation off the parent domain — it must
// not survive a merge.
func TestMerge_DropsEntriesWithNonLabelSlugs(t *testing.T) {
	existing := encodeValue(t, []Entry{
		{Slug: "evil.example/x", Name: "Evil"},
		{Slug: "UPPER", Name: "Shouty"},
		{Slug: "-lead", Name: "Bad hyphen"},
		{Slug: "beta", Name: "Beta"},
	})
	v := Merge(existing, Entry{Slug: "acme", Name: "Acme"})
	entries := decodeValue(t, v)
	if len(entries) != 2 {
		t.Fatalf("entries = %+v, want only acme + beta to survive", entries)
	}
	if entries[0].Slug != "acme" || entries[1].Slug != "beta" {
		t.Fatalf("entries = %+v", entries)
	}
}

// Older cookies carry a url field per entry; it is ignored (and shed) rather
// than breaking the parse — the wire shape deliberately no longer stores one.
func TestMerge_ShedsLegacyURLField(t *testing.T) {
	existing := encodeValue(t, []map[string]string{
		{"slug": "beta", "name": "Beta", "url": "https://evil.example/login"},
	})
	v := Merge(existing, Entry{Slug: "acme", Name: "Acme"})
	decoded, err := url.QueryUnescape(v)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(decoded, "evil.example") || strings.Contains(decoded, `"url"`) {
		t.Fatalf("re-encoded cookie still carries a url: %s", decoded)
	}
	entries := decodeValue(t, v)
	if len(entries) != 2 || entries[1].Slug != "beta" {
		t.Fatalf("entries = %+v, want the legacy entry kept minus its url", entries)
	}
}

func TestMerge_CapsEntries(t *testing.T) {
	v := ""
	for i := 0; i < 30; i++ {
		slug := "org" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		v = Merge(v, Entry{Slug: slug, Name: slug})
	}
	if entries := decodeValue(t, v); len(entries) > 20 {
		t.Fatalf("cookie grew past the cap: %d entries", len(entries))
	}
}

func TestCookie_ParentDomainScopedAndReadable(t *testing.T) {
	c := Cookie("v", "tinycld.org")
	if c.Domain != ".tinycld.org" || c.Path != "/" {
		t.Fatalf("cookie scope: %+v", c)
	}
	if c.HttpOnly {
		t.Fatal("must NOT be HttpOnly — the switcher UI reads it from document.cookie")
	}
	if !c.Secure {
		t.Fatal("must be Secure — it only travels over the router's https origins")
	}
	if !strings.Contains(c.String(), Name+"=") {
		t.Fatalf("serialized cookie: %s", c.String())
	}
}
