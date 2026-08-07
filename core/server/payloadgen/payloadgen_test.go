package payloadgen

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// go test ./payloadgen/ -update rewrites the golden files from current output.
var update = flag.Bool("update", false, "rewrite golden files")

func TestGenerateGolden(t *testing.T) {
	cases, err := os.ReadDir(filepath.Join("testdata", "golden"))
	if err != nil {
		t.Fatalf("read golden dir: %v", err)
	}
	for _, c := range cases {
		t.Run(c.Name(), func(t *testing.T) {
			dir := filepath.Join("testdata", "golden", c.Name())
			got, err := Generate(dir)
			if err != nil {
				t.Fatalf("Generate(%s) = %v", dir, err)
			}
			goldenPath := filepath.Join(dir, "expected.ts")
			if *update {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("update golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden (run with -update to create): %v", err)
			}
			if got != string(want) {
				t.Errorf("output drifted from %s — regenerate with -update if intended.\ngot:\n%s\nwant:\n%s", goldenPath, got, want)
			}
		})
	}
}

// Every construct the generator cannot map faithfully must be a loud error —
// a payload contract is never emitted best-effort.
func TestGenerateErrors(t *testing.T) {
	wantErr := map[string]string{
		"no-json-tag":       "no json tag",
		"any-field":         "`any` defeats the typed contract",
		"embedded":          "embedded fields are not supported",
		"foreign-selector":  "cross-package type time.Duration",
		"bad-import":        `imports "net/http"`,
		"unexported-ref":    "type inner is unexported",
		"map-int-key":       "map key int is not string",
		"non-struct-type":   "type Role is not a struct",
		"const-non-literal": "not a basic literal",
		"undeclared-type":   "type Missing is not declared",
	}
	cases, err := os.ReadDir(filepath.Join("testdata", "errors"))
	if err != nil {
		t.Fatalf("read errors dir: %v", err)
	}
	if len(cases) != len(wantErr) {
		t.Errorf("testdata/errors has %d cases, wantErr covers %d — keep them in sync", len(cases), len(wantErr))
	}
	for _, c := range cases {
		t.Run(c.Name(), func(t *testing.T) {
			want, ok := wantErr[c.Name()]
			if !ok {
				t.Fatalf("no expected error registered for case %s", c.Name())
			}
			_, err := Generate(filepath.Join("testdata", "errors", c.Name()))
			if err == nil {
				t.Fatalf("Generate succeeded, want error containing %q", want)
			}
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), want)
			}
		})
	}
}

// The emitted golden output for the multifile case proves declaration order is
// deterministic: files sort lexically, so Ack (b_second.go) emits after Reply
// (a_first.go) even though Reply references it.
func TestGenerateMultifileOrder(t *testing.T) {
	got, err := Generate(filepath.Join("testdata", "golden", "multifile"))
	if err != nil {
		t.Fatalf("Generate = %v", err)
	}
	reply := strings.Index(got, "export interface Reply")
	ack := strings.Index(got, "export interface Ack")
	if reply == -1 || ack == -1 {
		t.Fatalf("missing interfaces in output:\n%s", got)
	}
	if reply > ack {
		t.Errorf("Reply (a_first.go) should emit before Ack (b_second.go); output:\n%s", got)
	}
}
