package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type widget struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

var (
	headers = []string{"NAME", "COUNT"}
	rows    = [][]string{{"alpha", "3"}, {"beta", "12"}}
	raw     = []widget{{Name: "alpha", Count: 3}, {Name: "beta", Count: 12}}
)

func TestParseFormat(t *testing.T) {
	for _, ok := range []string{"table", "json", "csv"} {
		if _, err := ParseFormat(ok); err != nil {
			t.Errorf("ParseFormat(%q): %v", ok, err)
		}
	}
	if _, err := ParseFormat("yaml"); err == nil {
		t.Error("ParseFormat(yaml) should fail")
	}
}

func TestWriteTable(t *testing.T) {
	var buf bytes.Buffer
	o := Options{Format: Table}
	if err := o.Write(&buf, headers, rows, raw); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), got)
	}
	if !strings.HasPrefix(lines[0], "NAME") || !strings.Contains(lines[0], "COUNT") {
		t.Fatalf("header line = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "alpha") {
		t.Fatalf("row line = %q", lines[1])
	}
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("non-TTY table output must not contain ANSI escapes: %q", got)
	}
}

func TestWriteJSONIsStable(t *testing.T) {
	var a, b bytes.Buffer
	o := Options{Format: JSON}
	if err := o.Write(&a, headers, rows, raw); err != nil {
		t.Fatal(err)
	}
	if err := o.Write(&b, headers, rows, raw); err != nil {
		t.Fatal(err)
	}
	if a.String() != b.String() {
		t.Fatal("json output is not stable across runs")
	}
	if !strings.Contains(a.String(), `"name": "alpha"`) {
		t.Fatalf("json = %s", a.String())
	}
	// table cosmetics must not leak into json
	if strings.Contains(a.String(), "NAME") {
		t.Fatalf("json contains table header: %s", a.String())
	}
}

func TestWriteCSV(t *testing.T) {
	var buf bytes.Buffer
	o := Options{Format: CSV}
	if err := o.Write(&buf, headers, rows, raw); err != nil {
		t.Fatal(err)
	}
	want := "NAME,COUNT\nalpha,3\nbeta,12\n"
	if buf.String() != want {
		t.Fatalf("csv = %q, want %q", buf.String(), want)
	}
}

func TestInfoRespectsQuiet(t *testing.T) {
	var buf bytes.Buffer
	Options{}.Info(&buf, "hello %s", "world")
	if buf.String() != "hello world\n" {
		t.Fatalf("info = %q", buf.String())
	}
	buf.Reset()
	Options{Quiet: true}.Info(&buf, "hello %s", "world")
	if buf.String() != "" {
		t.Fatalf("quiet info = %q", buf.String())
	}
}

func TestFromCommandReadsPersistentFlags(t *testing.T) {
	root := &cobra.Command{Use: "root", SilenceUsage: true, SilenceErrors: true}
	pf := root.PersistentFlags()
	pf.String("output", string(Table), "")
	pf.Bool("json", false, "")
	pf.Bool("quiet", false, "")
	pf.Bool("no-color", false, "")
	pf.Bool("yes", false, "")
	var got Options
	var gotYes bool
	sub := &cobra.Command{Use: "sub", RunE: func(cmd *cobra.Command, _ []string) error {
		var err error
		got, gotYes, err = FromCommand(cmd)
		return err
	}}
	root.AddCommand(sub)

	root.SetArgs([]string{"sub", "--json", "--quiet", "--yes"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Format != JSON || !got.Quiet || !gotYes {
		t.Fatalf("got = %+v yes=%v", got, gotYes)
	}
	// Tests never run on a TTY, so color must be off and TTY false — the
	// non-TTY degradation the spec requires.
	if got.TTY || !got.NoColor {
		t.Fatalf("non-TTY run must disable color: %+v", got)
	}

	root.SetArgs([]string{"sub", "--output", "nonsense"})
	if err := root.Execute(); err == nil {
		t.Fatal("invalid --output must error")
	}
}
