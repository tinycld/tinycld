package textpreview

import (
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFirstRegionTextShort(t *testing.T) {
	input := "a short document"

	region, hash := FirstRegionText(input)
	if region != input {
		t.Fatalf("region = %q, want whole input %q", region, input)
	}
	if hash == "" {
		t.Fatal("hash is empty for short input")
	}
}

func TestFirstRegionTextTruncatesLongInput(t *testing.T) {
	input := strings.Repeat("x", 5000)

	region, _ := FirstRegionText(input)
	if got := len([]rune(region)); got > 3600 {
		t.Fatalf("region rune length = %d, want <= 3600", got)
	}
	if !strings.HasPrefix(input, region) {
		t.Fatal("region is not a prefix of the input")
	}
}

func TestFirstRegionTextDoesNotSplitMultibyteRunes(t *testing.T) {
	// Each "世" is a 3-byte rune. 4000 of them exceeds the 3600-rune cap, so
	// truncation must happen on a rune boundary, never mid-byte-sequence.
	input := strings.Repeat("世", 4000)

	region, _ := FirstRegionText(input)
	if !utf8.ValidString(region) {
		t.Fatal("truncated region is not valid UTF-8 (a multibyte rune was split)")
	}
	if got := len([]rune(region)); got > 3600 {
		t.Fatalf("region rune length = %d, want <= 3600", got)
	}
	if !strings.HasPrefix(input, region) {
		t.Fatal("region is not a prefix of the input")
	}
}

var hexOnly = regexp.MustCompile(`^[0-9a-f]+$`)

func TestHashStableAndDeterministic(t *testing.T) {
	a := Hash("same input")
	b := Hash("same input")
	if a != b {
		t.Fatalf("Hash not stable: %q != %q", a, b)
	}
}

func TestHashDistinguishesInput(t *testing.T) {
	if Hash("one") == Hash("two") {
		t.Fatal("Hash returned same output for different inputs")
	}
}

func TestHashFormat(t *testing.T) {
	h := Hash("anything")
	if len(h) != 64 {
		t.Fatalf("Hash length = %d, want 64 (sha256 hex)", len(h))
	}
	if !hexOnly.MatchString(h) {
		t.Fatalf("Hash %q contains non-hex characters", h)
	}
}
