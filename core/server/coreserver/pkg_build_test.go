package coreserver

import (
	"testing"
)

func TestParseLogLine(t *testing.T) {
	cases := []struct {
		line    string
		wantPct int
		wantStp string
		wantMsg string
		wantOk  bool
	}{
		{"[15%] Downloading package: Running npm pack", 15, "Downloading package", "Running npm pack", true},
		{"[5%] Validating package name: Checking todo", 5, "Validating package name", "Checking todo", true},
		{"[100%] Done: ", 100, "Done", "", true},
		{"[55%] Reversing migrations", 55, "Reversing migrations", "", true}, // no ": " → step only
		{"not a log line", 0, "", "", false},
		{"[abc%] x: y", 0, "", "", false},
	}
	for _, c := range cases {
		t.Run(c.line, func(t *testing.T) {
			pct, step, msg, ok := parseLogLine(c.line)
			if ok != c.wantOk || pct != c.wantPct || step != c.wantStp || msg != c.wantMsg {
				t.Fatalf("parseLogLine(%q) = (%d, %q, %q, %v), want (%d, %q, %q, %v)",
					c.line, pct, step, msg, ok, c.wantPct, c.wantStp, c.wantMsg, c.wantOk)
			}
		})
	}
}
