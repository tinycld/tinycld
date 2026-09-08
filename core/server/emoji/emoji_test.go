package emoji

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The fixture is the contract between this package and
// core/lib/emoji/normalize.ts. Both sides assert it, so a rule changed in one
// language and not the other fails here rather than silently splitting one
// reaction into two chips in production.
func TestNormalizeMatchesTheSharedFixture(t *testing.T) {
	path := filepath.Join("..", "..", "lib", "emoji", "__fixtures__", "normalize-cases.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the shared fixture: %v", err)
	}

	var fixture struct {
		Cases []struct {
			Raw  string  `json:"raw"`
			Want *string `json:"want"`
			Why  string  `json:"why"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("parsing the shared fixture: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("the shared fixture is empty; it is supposed to be the contract")
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Why, func(t *testing.T) {
			got, ok := Normalize(tc.Raw)
			if tc.Want == nil {
				if ok {
					t.Fatalf("Normalize(%q) = %q, want rejected", tc.Raw, got)
				}
				return
			}
			if !ok {
				t.Fatalf("Normalize(%q) rejected, want %q", tc.Raw, *tc.Want)
			}
			if got != *tc.Want {
				t.Fatalf("Normalize(%q) = %q, want %q", tc.Raw, got, *tc.Want)
			}
		})
	}
}

func TestNormalizeCanonicalizesTheVariationSelector(t *testing.T) {
	// The motivating case for the whole design: NFC leaves these as two
	// distinct strings, and the unique index compares bytes.
	got, ok := Normalize("❤")
	if !ok || got != "❤️" {
		t.Fatalf(`Normalize("❤") = %q, %v; want "❤️", true`, got, ok)
	}
}

func TestNormalizeRejectsWhatIsNotAnEmoji(t *testing.T) {
	// The column is free text. Without this the API could store a chip
	// nobody can toggle off.
	for _, raw := range []string{"not an emoji", "hello", "a", "5", "#", "", "   "} {
		if got, ok := Normalize(raw); ok {
			t.Errorf("Normalize(%q) = %q, want rejected", raw, got)
		}
	}
}

func TestNormalizeAcceptsKeycapsWhichARegexWouldReject(t *testing.T) {
	// Why the vocabulary is a table: a sequence regex over Unicode emoji
	// properties rejects all 15 keycaps while accepting bare digits.
	for _, raw := range []string{"5️⃣", "#️⃣"} {
		if got, ok := Normalize(raw); !ok || got != raw {
			t.Errorf("Normalize(%q) = %q, %v; want it kept", raw, got, ok)
		}
	}
}

func TestNormalizeHandlesSkinTones(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"👍🏽", "👍🏽"},
		{"✌🏻", "✌🏻"}, // base carries FE0F; the toned form must not
	}
	for _, tc := range cases {
		got, ok := Normalize(tc.raw)
		if !ok || got != tc.want {
			t.Errorf("Normalize(%q) = %q, %v; want %q", tc.raw, got, ok, tc.want)
		}
	}

	// A tone on something that does not take one is not a reaction anyone
	// could have picked.
	if got, ok := Normalize("🎉🏽"); ok {
		t.Errorf(`Normalize("🎉🏽") = %q, want rejected`, got)
	}
}

func TestNormalizeRejectsCountryFlags(t *testing.T) {
	// Deliberately not shipped: the picker cannot find them, so nothing
	// should be able to store one.
	for _, raw := range []string{"🇺🇦", "🇺🇸", "🇯🇵"} {
		if got, ok := Normalize(raw); ok {
			t.Errorf("Normalize(%q) = %q, want rejected", raw, got)
		}
	}
	// The non-country flags stay.
	for _, raw := range []string{"🏳️‍🌈", "🏴‍☠️", "🏁", "🏴󠁧󠁢󠁳󠁣󠁴󠁿"} {
		if got, ok := Normalize(raw); !ok || got != raw {
			t.Errorf("Normalize(%q) = %q, %v; want it kept", raw, got, ok)
		}
	}
}

func TestIsCanonical(t *testing.T) {
	for _, raw := range []string{"👍", "❤️", "👍🏽"} {
		if !IsCanonical(raw) {
			t.Errorf("IsCanonical(%q) = false, want true", raw)
		}
	}
	for _, raw := range []string{"❤", "not an emoji", " 👍 "} {
		if IsCanonical(raw) {
			t.Errorf("IsCanonical(%q) = true, want false", raw)
		}
	}
}

func TestVocabularyIsLoaded(t *testing.T) {
	// A silently empty embed would make every check above vacuous.
	if len(canonical) < 1000 {
		t.Fatalf("canonical vocabulary has %d entries, expected ~1647", len(canonical))
	}
	if len(tonable) < 300 {
		t.Fatalf("tonable set has %d entries, expected ~310", len(tonable))
	}
}
