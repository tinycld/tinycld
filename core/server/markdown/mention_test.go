package markdown

import "testing"

// A mention is an ATOM: it carries a user id and has no children. The
// serializer's default branch renders an unknown inline node's descendants, so
// without an explicit case a mention emits NOTHING — the description flushes
// back to storage with the mention silently deleted, while every other edit in
// the same paragraph is saved. That is the bug these tests exist to prevent.

func TestMentionRoundTrip(t *testing.T) {
	const md = "owner is [[@u1]]\n"

	doc := ToPM(md)
	got := FromPM(doc)
	if got != md {
		t.Fatalf("round trip lost the mention:\n got %q\nwant %q", got, md)
	}
}

func TestMentionParsesToAnAtom(t *testing.T) {
	doc := ToPM("hi [[@abc123]] there\n")
	para := doc.Content[0]

	var found *PMNode
	for i := range para.Content {
		if para.Content[i].Type == NodeMention {
			found = &para.Content[i]
		}
	}
	if found == nil {
		t.Fatal("no mention node produced; the raw token would be visible to readers")
	}
	if got := attrString(found.Attrs, "userId"); got != "abc123" {
		t.Fatalf("userId = %q, want %q", got, "abc123")
	}
	if len(found.Content) != 0 {
		t.Fatalf("mention should be an atom, got %d children", len(found.Content))
	}
}

func TestMentionSurvivesSurroundingText(t *testing.T) {
	// The text on either side must not be swallowed by the split.
	doc := ToPM("a [[@u1]] b [[@u2]] c\n")
	if got, want := FromPM(doc), "a [[@u1]] b [[@u2]] c\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestTextWithoutMentionsIsUnchanged(t *testing.T) {
	const md = "just ordinary prose\n"
	if got := FromPM(ToPM(md)); got != md {
		t.Fatalf("got %q, want %q", got, md)
	}
}

// A bracketed phrase that is not a mention must be left alone.
func TestNonMentionBracketsAreNotConsumed(t *testing.T) {
	doc := ToPM("see [[not a mention]] here\n")
	for _, n := range doc.Content[0].Content {
		if n.Type == NodeMention {
			t.Fatal("plain bracketed text was parsed as a mention")
		}
	}
}

// The name rides in the token so a mention stays readable when the id cannot be
// resolved — the person left the board, or the roster has not loaded yet.
func TestMentionCarriesTheDisplayName(t *testing.T) {
	const md = "hi [[@u1|Ada Lovelace]]\n"
	if got := FromPM(ToPM(md)); got != md {
		t.Fatalf("round trip lost the name:\n got %q\nwant %q", got, md)
	}

	doc := ToPM(md)
	var mention *PMNode
	for i := range doc.Content[0].Content {
		if doc.Content[0].Content[i].Type == NodeMention {
			mention = &doc.Content[0].Content[i]
		}
	}
	if mention == nil {
		t.Fatal("no mention node")
	}
	if got := attrString(mention.Attrs, "name"); got != "Ada Lovelace" {
		t.Fatalf("name = %q, want %q", got, "Ada Lovelace")
	}
	if got := attrString(mention.Attrs, "userId"); got != "u1" {
		t.Fatalf("userId = %q, want %q", got, "u1")
	}
}

// The bare form predates the name half and is already in stored documents.
func TestLegacyTokenWithoutNameStillParses(t *testing.T) {
	doc := ToPM("hi [[@u1]]\n")
	for _, n := range doc.Content[0].Content {
		if n.Type == NodeMention {
			if name := attrString(n.Attrs, "name"); name != "" {
				t.Fatalf("expected no name, got %q", name)
			}
			return
		}
	}
	t.Fatal("legacy token did not parse into a mention")
}

// A name containing `]` or `|` would otherwise end the token early and spill
// the rest of it into the document as visible text.
func TestMentionNameWithDelimitersRoundTrips(t *testing.T) {
	doc := &PMNode{Type: NodeDoc, Content: []PMNode{{
		Type: NodeParagraph,
		Content: []PMNode{{
			Type:  NodeMention,
			Attrs: map[string]any{"userId": "u1", "name": "a]b|c"},
		}},
	}}}

	md := FromPM(doc)
	back := ToPM(md)
	var got *PMNode
	for i := range back.Content[0].Content {
		if back.Content[0].Content[i].Type == NodeMention {
			got = &back.Content[0].Content[i]
		}
	}
	if got == nil {
		t.Fatalf("mention did not survive %q", md)
	}
	if name := attrString(got.Attrs, "name"); name != "a]b|c" {
		t.Fatalf("name = %q, want %q (serialized: %q)", name, "a]b|c", md)
	}
}
