package coreserver

import (
	"reflect"
	"testing"
)

func TestNewMigrationFiles(t *testing.T) {
	cases := []struct {
		name   string
		before []string
		after  []string
		want   []string
	}{
		{
			name:   "one new migration on top",
			before: []string{"1713_b.js", "1712_a.js"},
			after:  []string{"1714_c.js", "1713_b.js", "1712_a.js"},
			want:   []string{"1714_c.js"},
		},
		{
			name:   "several new migrations newest-first",
			before: []string{"1712_a.js"},
			after:  []string{"1715_d.js", "1714_c.js", "1713_b.js", "1712_a.js"},
			want:   []string{"1715_d.js", "1714_c.js", "1713_b.js"},
		},
		{
			name:   "nothing applied",
			before: []string{"1712_a.js"},
			after:  []string{"1712_a.js"},
			want:   []string{},
		},
		{
			name:   "empty before (first install)",
			before: nil,
			after:  []string{"1712_a.js"},
			want:   []string{"1712_a.js"},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := newMigrationFiles(c.before, c.after)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("newMigrationFiles() = %v, want %v", got, c.want)
			}
		})
	}
}

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

func TestTailMatches(t *testing.T) {
	cases := []struct {
		name     string
		applied  []string // newest-first live history
		expected []string // newest-first chain we plan to step down
		wantErr  bool
	}{
		{
			name:     "no migrations to reverse is always ok",
			applied:  []string{"1712_a.js"},
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "clean tail matches",
			applied:  []string{"1715_d.js", "1714_c.js", "1713_b.js", "1712_a.js"},
			expected: []string{"1715_d.js", "1714_c.js"},
			wantErr:  false,
		},
		{
			name:     "diverged tail (out-of-band migration on top) blocks",
			applied:  []string{"1716_x.js", "1715_d.js", "1714_c.js"},
			expected: []string{"1715_d.js", "1714_c.js"},
			wantErr:  true,
		},
		{
			name:     "history shorter than expected blocks",
			applied:  []string{"1715_d.js"},
			expected: []string{"1715_d.js", "1714_c.js"},
			wantErr:  true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := tailMatches(c.applied, c.expected)
			if (err != nil) != c.wantErr {
				t.Fatalf("tailMatches() err = %v, wantErr = %v", err, c.wantErr)
			}
		})
	}
}
