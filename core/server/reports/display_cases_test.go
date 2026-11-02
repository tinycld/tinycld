package reports

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
)

// The table the client runs too (core/reports/lib/__tests__/table-lines.test.ts):
// both sides must place, refuse and cut every case the same way.
const displayCasesPath = "../../reports/lib/fixtures/display-cells.json"

type displayCase struct {
	Name   string        `json:"name"`
	Tracks int           `json:"tracks"`
	Cells  []DisplayCell `json:"cells"`
	Placed []struct {
		Start int `json:"start"`
		Span  int `json:"span"`
		Line  int `json:"line"`
		Lines int `json:"lines"`
	} `json:"placed"`
	Problem *struct {
		Index   int    `json:"index"`
		Message string `json:"message"`
	} `json:"problem"`
}

func TestDisplayCellsSharedTable(t *testing.T) {
	raw, err := os.ReadFile(displayCasesPath)
	if err != nil {
		t.Fatal(err)
	}
	var table struct {
		Cases []displayCase `json:"cases"`
	}
	if err := json.Unmarshal(raw, &table); err != nil {
		t.Fatal(err)
	}
	// Keep equal to the count in the client test.
	if len(table.Cases) != 19 {
		t.Fatalf("cases = %d, want 19", len(table.Cases))
	}
	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			got, err := placeCells(c.Tracks, c.Cells)
			if c.Problem != nil {
				var se *SpecError
				if !errors.As(err, &se) {
					t.Fatalf("err = %v, want a *SpecError", err)
				}
				at := fmt.Sprintf("cells.%d", c.Problem.Index)
				problem, ok := se.Problems[at]
				if !ok {
					t.Fatalf("no problem at %q: %v", at, se)
				}
				if problem.Message() != c.Problem.Message {
					t.Fatalf("message = %q, want %q", problem.Message(), c.Problem.Message)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := make([]placed, len(c.Placed))
			for i, p := range c.Placed {
				want[i] = placed{p.Start, p.Span, p.Line, p.Lines}
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("placed = %+v, want %+v", got, want)
			}
		})
	}
}
