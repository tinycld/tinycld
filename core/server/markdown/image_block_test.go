package markdown

import (
	"strings"
	"testing"
)

// tiptap's Image extension is a BLOCK node, so a flushed document can carry an
// image directly under doc — a description that is nothing but a picture, or
// one where an image was dropped between paragraphs (the drop splits the
// paragraph and leaves the image at the top level). The corpus cannot express
// this shape (it starts from markdown, which goldmark parses into a
// paragraph-wrapped image), so it is pinned here: renderBlock used to have no
// NodeImage case, the default branch walked the image's empty children, and
// the flush wrote an empty document — the description was gone on the next
// board open. Caught by cards' description-images e2e reload case.
func TestFromPMTopLevelImage(t *testing.T) {
	doc := &PMNode{
		Type: NodeDoc,
		Content: []PMNode{
			{
				Type: NodeImage,
				Attrs: map[string]any{
					"src": "/api/files/cards_attachments/rec1/pic_ab.png",
					"alt": "pic.png",
				},
			},
		},
	}
	got := FromPM(doc)
	want := "![pic.png](/api/files/cards_attachments/rec1/pic_ab.png)\n"
	if got != want {
		t.Fatalf("FromPM(top-level image) = %q; want %q", got, want)
	}
}

func TestFromPMImageBetweenParagraphs(t *testing.T) {
	doc := &PMNode{
		Type: NodeDoc,
		Content: []PMNode{
			{Type: NodeParagraph, Content: []PMNode{{Type: NodeText, Text: "before"}}},
			{Type: NodeImage, Attrs: map[string]any{"src": "/api/files/x/y.png"}},
			{Type: NodeParagraph, Content: []PMNode{{Type: NodeText, Text: "after"}}},
		},
	}
	got := FromPM(doc)
	for _, part := range []string{"before", "![](/api/files/x/y.png)", "after"} {
		if !strings.Contains(got, part) {
			t.Fatalf("FromPM output %q is missing %q", got, part)
		}
	}
	// And the round trip holds: re-parsing the flush output must keep the
	// image, or the next seed drops what the previous flush preserved.
	back := FromPM(ToPM(got))
	if back != got {
		t.Fatalf("round trip diverged:\n first: %q\nsecond: %q", got, back)
	}
}
