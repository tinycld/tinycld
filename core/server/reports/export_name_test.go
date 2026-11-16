package reports

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// The file name of a download, an email attachment and the dialog's line:
// the report's title as words joined by "-", then its period, its as-of day
// or the reader's day. The cases are format-cases.json's fileNames, which the
// client's exportFileName runs too, so the dialog names the file the server
// sends.
func TestExportFileNameMatchesTheSharedCases(t *testing.T) {
	for _, c := range loadFormatCases(t).FileNames {
		t.Run(c.Name, func(t *testing.T) {
			got := exportFileName(c.Title, c.Defs, c.Params, c.Today, c.Ext)
			if got != c.Text {
				t.Fatalf("exportFileName(%q) = %q, want %q", c.Title, got, c.Text)
			}
		})
	}
}

// Whatever the title, the name part is at most 80 characters, never starts or
// ends with "-", and holds nothing that climbs a directory or breaks a header.
func TestExportFileNameIsAlwaysSafe(t *testing.T) {
	titles := []string{
		strings.Repeat("Zookeeper ", 30),
		strings.Repeat("ж", 200),
		`../..\..//"quoted"/'single'`,
		"-Leading and trailing-",
		"   ",
		"",
		"Tab\tand\nnew line",
	}
	for _, title := range titles {
		got := exportFileName(title, nil, nil, "2026-11-16", "csv")
		stem, ok := strings.CutSuffix(got, "-2026-11-16.csv")
		if !ok {
			t.Fatalf("exportFileName(%q) = %q, want the reader's day and .csv at the end", title, got)
		}
		if n := utf8.RuneCountInString(stem); n == 0 || n > 80 {
			t.Fatalf("exportFileName(%q): name part %q has %d characters, want 1 to 80", title, stem, n)
		}
		if strings.HasPrefix(stem, "-") || strings.HasSuffix(stem, "-") || strings.Contains(stem, "--") {
			t.Fatalf("exportFileName(%q): name part %q has a stray -", title, stem)
		}
		if strings.ContainsAny(stem, `/\"'. `+"\t\n") {
			t.Fatalf("exportFileName(%q): name part %q holds a character the rule removes", title, stem)
		}
	}
}
