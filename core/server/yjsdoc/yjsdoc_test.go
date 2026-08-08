package yjsdoc

import (
	"encoding/json"
	"testing"
	"time"

	ycrdt "github.com/skyterra/y-crdt"

	"tinycld.org/core/markdown"
)

func pmDoc(t *testing.T, text string) []byte {
	t.Helper()
	b, err := json.Marshal(markdown.PMNode{
		Type: markdown.NodeDoc,
		Content: []markdown.PMNode{{
			Type:    markdown.NodeParagraph,
			Content: []markdown.PMNode{{Type: markdown.NodeText, Text: text}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func firstParagraphText(t *testing.T, pmJSON []byte) string {
	t.Helper()
	var doc markdown.PMNode
	if err := json.Unmarshal(pmJSON, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(doc.Content) == 0 || len(doc.Content[0].Content) == 0 {
		return ""
	}
	return doc.Content[0].Content[0].Text
}

// The whole cards design rests on one document holding many independent
// editors. If fragments leaked into each other, every card on a board would
// show the same description.
func TestFragmentsAreIndependent(t *testing.T) {
	doc := ycrdt.NewDoc("board", false, nil, nil, false)
	InstallPatcher(doc)

	if err := SeedFragmentFromPMJSON(doc, "card:aaa", pmDoc(t, "first card")); err != nil {
		t.Fatalf("seed aaa: %v", err)
	}
	if err := SeedFragmentFromPMJSON(doc, "card:bbb", pmDoc(t, "second card")); err != nil {
		t.Fatalf("seed bbb: %v", err)
	}

	for name, want := range map[string]string{
		"card:aaa": "first card",
		"card:bbb": "second card",
	} {
		got, err := PMJSONFromFragment(doc, name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if text := firstParagraphText(t, got); text != want {
			t.Errorf("%s = %q, want %q", name, text, want)
		}
	}
}

func TestFragmentNamesFindsSeededCards(t *testing.T) {
	doc := ycrdt.NewDoc("board", false, nil, nil, false)
	InstallPatcher(doc)
	for _, id := range []string{"card:ccc", "card:aaa", "card:bbb"} {
		if err := SeedFragmentFromPMJSON(doc, id, pmDoc(t, "x")); err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	// A root that is not a card must not be reported as an editor.
	doc.GetMap("presence")

	got := FragmentNames(doc, "card:")
	want := []string{"card:aaa", "card:bbb", "card:ccc"}
	if len(got) != len(want) {
		t.Fatalf("FragmentNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FragmentNames = %v, want %v (sorted)", got, want)
		}
	}
}

func TestEmptyFragmentReadsAsEmptyDoc(t *testing.T) {
	// A card created while the room is live has no fragment until someone
	// types. Reading it must yield an empty doc, not an error — otherwise
	// flush would fail the whole board over one untouched card.
	doc := ycrdt.NewDoc("board", false, nil, nil, false)
	InstallPatcher(doc)

	got, err := PMJSONFromFragment(doc, "card:never-touched")
	if err != nil {
		t.Fatalf("read empty fragment: %v", err)
	}
	if md := markdown.FromPM(mustPM(t, got)); md != "\n" {
		t.Errorf("empty fragment serialized to %q, want %q", md, "\n")
	}
}

func mustPM(t *testing.T, raw []byte) *markdown.PMNode {
	t.Helper()
	var doc markdown.PMNode
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return &doc
}

// End-to-end for the flush path: markdown in, fragment, markdown out.
func TestMarkdownSurvivesTheFragmentRoundTrip(t *testing.T) {
	const src = "## Heading\n\nSome **bold** text.\n\n- one\n- two\n"

	doc := ycrdt.NewDoc("board", false, nil, nil, false)
	InstallPatcher(doc)

	pmJSON, err := json.Marshal(markdown.ToPM(src))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := SeedFragmentFromPMJSON(doc, "card:x", pmJSON); err != nil {
		t.Fatalf("seed: %v", err)
	}

	readBack, err := PMJSONFromFragment(doc, "card:x")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got := markdown.FromPM(mustPM(t, readBack)); got != src {
		t.Errorf("round trip through a fragment changed the text:\nwant %q\n got %q", src, got)
	}
}

func TestRuntimeRejectsDuplicateRoom(t *testing.T) {
	rt := NewRuntime()
	defer rt.Stop()
	if _, err := rt.NewDoc("room"); err != nil {
		t.Fatalf("first NewDoc: %v", err)
	}
	if _, err := rt.NewDoc("room"); err == nil {
		t.Error("second NewDoc for the same room should fail")
	}
}

func TestRuntimeBootstrapRunsBeforeTheHandleIsUsable(t *testing.T) {
	rt := NewRuntime()
	defer rt.Stop()
	rt.SetBootstrap(func(roomID string, doc *Doc) error {
		return SeedFragmentFromPMJSON(doc, "card:seeded", pmDoc(t, "from bootstrap"))
	})

	handle, err := rt.NewDoc("room")
	if err != nil {
		t.Fatalf("NewDoc: %v", err)
	}
	// The broker serves SyncReply from this; content must already be there.
	state, err := handle.EncodeStateAsUpdate()
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(state) == 0 {
		t.Fatal("bootstrapped room encoded no state")
	}

	var text string
	if err := rt.HandleFor("room").WithDoc(func(doc *Doc) error {
		raw, err := PMJSONFromFragment(doc, "card:seeded")
		if err != nil {
			return err
		}
		text = firstParagraphText(t, raw)
		return nil
	}); err != nil {
		t.Fatalf("WithDoc: %v", err)
	}
	if text != "from bootstrap" {
		t.Errorf("bootstrap content = %q", text)
	}
}

func TestBootstrapFailureStillYieldsAUsableRoom(t *testing.T) {
	// Refusing the room would take the feature down for everyone in it; an
	// empty document at least lets people connect and type.
	rt := NewRuntime()
	defer rt.Stop()
	rt.SetBootstrap(func(string, *Doc) error { return errBoom })

	handle, err := rt.NewDoc("room")
	if err != nil {
		t.Fatalf("NewDoc returned an error for a failed bootstrap: %v", err)
	}
	if _, err := handle.EncodeStateAsUpdate(); err != nil {
		t.Errorf("room unusable after bootstrap failure: %v", err)
	}
}

var errBoom = &boomError{}

type boomError struct{}

func (*boomError) Error() string { return "boom" }

func TestClosedHandleRejectsWork(t *testing.T) {
	rt := NewRuntime()
	defer rt.Stop()
	handle, err := rt.NewDoc("room")
	if err != nil {
		t.Fatalf("NewDoc: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := handle.Close(); err != nil {
		t.Errorf("Close is not idempotent: %v", err)
	}
	if err := handle.ApplyUpdate([]byte{1, 2}); err == nil {
		t.Error("ApplyUpdate on a closed handle should fail")
	}
	if _, err := handle.EncodeStateAsUpdate(); err == nil {
		t.Error("EncodeStateAsUpdate on a closed handle should fail")
	}
	// The room is free again once closed.
	if _, err := rt.NewDoc("room"); err != nil {
		t.Errorf("room not released after Close: %v", err)
	}
}

func TestApplyUpdateRejectsOversizePayload(t *testing.T) {
	rt := NewRuntime()
	defer rt.Stop()
	handle, err := rt.NewDoc("room")
	if err != nil {
		t.Fatalf("NewDoc: %v", err)
	}
	if err := handle.ApplyUpdate(make([]byte, MaxApplyUpdateBytes+1)); err == nil {
		t.Error("oversize payload should be rejected before it is decoded")
	}
}

func TestApplyUpdateSurvivesGarbage(t *testing.T) {
	// Hostile input must not take down the broker goroutine.
	rt := NewRuntime()
	defer rt.Stop()
	handle, err := rt.NewDoc("room")
	if err != nil {
		t.Fatalf("NewDoc: %v", err)
	}
	if err := handle.ApplyUpdate([]byte{0xff, 0xfe, 0x00, 0x42, 0x99}); err != nil {
		t.Logf("garbage rejected with %v (acceptable)", err)
	}
}

// The janitor must not evict a document out from under a room that is still
// live. A board can hold presence for hours with nobody editing a description;
// evicting there would strand the next edit against a closed handle.
func TestJanitorSpareLiveRooms(t *testing.T) {
	restore := now
	defer func() { now = restore }()

	rt := NewRuntime()
	defer rt.Stop()
	if _, err := rt.NewDoc("live"); err != nil {
		t.Fatalf("NewDoc live: %v", err)
	}
	if _, err := rt.NewDoc("abandoned"); err != nil {
		t.Fatalf("NewDoc abandoned: %v", err)
	}
	rt.MarkLive("live", true)

	now = func() time.Time { return restore().Add(MaxIdleDuration + time.Minute) }
	rt.evictIdleDocs()

	if rt.HandleFor("live") == nil {
		t.Error("janitor evicted a document belonging to a live room")
	}
	if rt.HandleFor("abandoned") != nil {
		t.Error("janitor kept an idle document with no room")
	}
}
