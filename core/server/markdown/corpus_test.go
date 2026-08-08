package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus is the contract between this package and the TypeScript editor:
// every file must survive Markdown → ProseMirror → Markdown byte-for-byte.
//
// That property is what the flush path depends on. Flush decides whether a card
// changed by comparing freshly serialized output against a stored baseline, so
// an unstable serializer would rewrite every card on every flush — and, worse,
// would churn the FTS index and every collaborator's view of the text.
//
// The same directory is read by the client-side vitest suite, so a divergence
// between the two serializers fails on both sides rather than silently
// producing two spellings of the same document.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.md"))
	if err != nil {
		t.Fatalf("glob corpus: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("corpus is empty — the round-trip guarantee would be vacuous")
	}
	return paths
}

func TestCorpusRoundTripIsByteStable(t *testing.T) {
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			src := string(raw)

			got := FromPM(ToPM(src))
			if got != src {
				t.Errorf("round trip changed the document.\n--- want ---\n%s\n--- got ---\n%s", src, got)
			}
		})
	}
}

func TestCorpusSecondPassIsIdempotent(t *testing.T) {
	// Even if a file's first pass differs from its source (an author wrote
	// `*x*` where we emit `*x*` differently), the SECOND pass must equal the
	// first. Flush compares serialized-to-serialized, so this is the property
	// that keeps a quiet document quiet.
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			once := FromPM(ToPM(string(raw)))
			twice := FromPM(ToPM(once))
			if once != twice {
				t.Errorf("serialization is not idempotent.\n--- first ---\n%s\n--- second ---\n%s", once, twice)
			}
		})
	}
}

func TestCorpusPreservesLoadBearingContent(t *testing.T) {
	// Round-trip equality above would also pass if both sides dropped the same
	// content, so assert the constructs that silently vanished when the editor
	// was configured without the matching extensions.
	seed := readCorpus(t, "060-seed-description.md")
	out := FromPM(ToPM(seed))

	for _, want := range []string{
		"## What we know",
		"### Suspects",
		"**200+ cards**",
		"`useActiveBoard`",
		"| Board size | First paint |",
		"| 500 cards  | \\~6s        |",
		"> Measure first.",
		"[the board query notes](https://example.com/notes)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q from the seeded description", want)
		}
	}
}

func TestTaskListCheckboxesSurvive(t *testing.T) {
	out := FromPM(ToPM(readCorpus(t, "030-lists.md")))
	if !strings.Contains(out, "- [ ] unchecked task") {
		t.Error("unchecked task item lost its checkbox")
	}
	if !strings.Contains(out, "- [x] checked task") {
		t.Error("checked task item lost its checked state")
	}
}

func TestModifierGlyphsSurviveVerbatim(t *testing.T) {
	// A user typed these; unlike an authored help topic they must never be
	// rewritten to Ctrl/Shift/Alt.
	out := FromPM(ToPM(readCorpus(t, "070-glyphs-and-edges.md")))
	for _, glyph := range []string{"⌘K", "⇧⌥P"} {
		if !strings.Contains(out, glyph) {
			t.Errorf("glyph %q did not survive", glyph)
		}
	}
}

func readCorpus(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "corpus", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(raw)
}
