package markdown

import (
	"strings"
	"testing"
)

// Escaping is the part of this package most likely to regress into data
// corruption, because the failure is cumulative: an over-eager escaper adds a
// backslash on every save, so a card that is merely opened and closed a few
// times ends up full of them. These cases pin the exact behavior.

func TestEscapingDoesNotCompoundAcrossSaves(t *testing.T) {
	// The motivating bug: `\~2s` gained a backslash per round trip.
	const src = "Timings: \\~2s and \\~6s.\n"
	out := src
	for i := 0; i < 5; i++ {
		next := FromPM(ToPM(out))
		if i > 0 && next != out {
			t.Fatalf("save %d changed the text\n before: %q\n  after: %q", i+1, out, next)
		}
		out = next
	}
	if strings.Contains(out, `\\`) {
		t.Errorf("escapes accumulated: %q", out)
	}
}

func TestIntraWordUnderscoresAreNotEscaped(t *testing.T) {
	// snake_case is literal in CommonMark; escaping it would put backslashes
	// into every identifier a developer types into a description.
	out := FromPM(ToPM("Call some_helper_name now.\n"))
	if strings.Contains(out, `\_`) {
		t.Errorf("escaped an intra-word underscore: %q", out)
	}
	if !strings.Contains(out, "some_helper_name") {
		t.Errorf("mangled the identifier: %q", out)
	}
}

func TestStructuralCharactersStayEscaped(t *testing.T) {
	// The other direction: a user who typed literal emphasis markers must not
	// have them become formatting on the next parse.
	const src = "Literal \\*stars\\* stay literal.\n"
	once := FromPM(ToPM(src))
	twice := FromPM(ToPM(once))
	if once != twice {
		t.Fatalf("not idempotent:\n once: %q\ntwice: %q", once, twice)
	}
	if strings.Contains(once, "<em>") || !strings.Contains(once, `\*stars\*`) {
		t.Errorf("literal stars were reinterpreted: %q", once)
	}
}

func TestCodeSpansAreVerbatim(t *testing.T) {
	// Escaping inside a code span would render the backslashes literally.
	out := FromPM(ToPM("Use `a * b` and `x_y`.\n"))
	if strings.Contains(out, `\*`) || strings.Contains(out, `\_`) {
		t.Errorf("escaped inside a code span: %q", out)
	}
}

func TestBackslashBeforeNonPunctuationSurvives(t *testing.T) {
	// `\d` is not an escape in CommonMark — it is a literal backslash, and a
	// regex in a description depends on that.
	out := FromPM(ToPM("Match \\d+ digits.\n"))
	if !strings.Contains(out, `\d`) {
		t.Errorf("lost a literal backslash: %q", out)
	}
}

func TestEmptyDocumentSerializesToSingleNewline(t *testing.T) {
	// Flush compares against this for a card with no description; anything
	// else would make an empty card look permanently dirty.
	if got := FromPM(ToPM("")); got != "\n" {
		t.Errorf("empty document = %q, want %q", got, "\n")
	}
	if got := FromPM(nil); got != "\n" {
		t.Errorf("nil document = %q, want %q", got, "\n")
	}
}

func TestTableCellPipesAreEscaped(t *testing.T) {
	// An unescaped pipe would split the cell and shift every column after it.
	out := FromPM(ToPM("| a | b |\n| - | - |\n| x \\| y | z |\n"))
	if !strings.Contains(out, `\|`) {
		t.Errorf("cell pipe was not escaped: %q", out)
	}
	if FromPM(ToPM(out)) != out {
		t.Errorf("table with an escaped pipe is not stable: %q", out)
	}
}

func TestHeadingLevelsClamp(t *testing.T) {
	// A level outside 1..6 has no markdown spelling; clamping keeps the
	// document renderable instead of emitting `####### `.
	doc := &PMNode{Type: NodeDoc, Content: []PMNode{{
		Type:    NodeHeading,
		Attrs:   map[string]any{"level": 9},
		Content: []PMNode{{Type: NodeText, Text: "deep"}},
	}}}
	if got := FromPM(doc); got != "###### deep\n" {
		t.Errorf("level 9 heading = %q", got)
	}
}

func TestCodeFenceGrowsPastInnerBackticks(t *testing.T) {
	// A body containing ``` would otherwise terminate its own fence.
	doc := &PMNode{Type: NodeDoc, Content: []PMNode{{
		Type:    NodeCodeBlock,
		Content: []PMNode{{Type: NodeText, Text: "a\n```\nb"}},
	}}}
	out := FromPM(doc)
	if !strings.HasPrefix(out, "````") {
		t.Errorf("fence did not grow: %q", out)
	}
	if FromPM(ToPM(out)) != out {
		t.Errorf("grown fence is not stable: %q", out)
	}
}
