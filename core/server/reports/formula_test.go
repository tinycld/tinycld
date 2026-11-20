package reports

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pocketbase/dbx"
)

func parseNode(t *testing.T, raw string) *Node {
	t.Helper()
	var n Node
	if err := json.Unmarshal([]byte(raw), &n); err != nil {
		t.Fatalf("parse %s: %v", raw, err)
	}
	return &n
}

func TestCompileFormulaRefusesWhatCannotBeComputed(t *testing.T) {
	types := map[string]FieldType{"amount": Money, "n": Number, "name": Text, "day": Date, "ok": Bool}
	cases := map[string]struct {
		formula, at, want string
	}{
		"amount times amount":     {`{"op":"mul","args":[{"col":"amount"},{"col":"amount"}]}`, "f", "an amount times an amount"},
		"div without scale":       {`{"op":"div","args":[{"col":"n"},{"value":2}]}`, "f.scale", "div needs a scale from 0 to 6"},
		"scale too large":         {`{"op":"round","args":[{"col":"n"}],"scale":7}`, "f.scale", "needs a scale from 0 to 6"},
		"amount by number, cents": {`{"op":"div","args":[{"col":"amount"},{"col":"n"}],"scale":2}`, "f.scale", "use scale 0"},
		"and of a number":         {`{"op":"and","args":[{"col":"ok"},{"col":"n"}]}`, "f.args.1", "and needs conditions"},
		"not of a number":         {`{"op":"not","args":[{"col":"n"}]}`, "f.args.0", "not needs a condition"},
		"unknown operator":        {`{"op":"pow","args":[{"col":"n"},{"value":2}]}`, "f", `unknown operator "pow"`},
		"two kinds in one node":   {`{"col":"n","value":2}`, "f", "one of op, col, path"},
		"list outside in":         {`{"op":"add","args":[{"value":[1,2]},{"value":1}]}`, "f.args.0", "fits only between or in"},
		"if of two kinds":         {`{"op":"if","args":[{"col":"ok"},{"value":"x"},{"value":1}]}`, "f", "both results of if"},
		"eq of text and number":   {`{"op":"eq","args":[{"col":"name"},{"col":"n"}]}`, "f", "compares values of one kind"},
		"addDays of a number":     {`{"op":"addDays","args":[{"col":"n"},{"value":1}]}`, "f", "needs a date and a number"},
		"sum of text":             {`{"op":"sum","args":[{"col":"name"}]}`, "f.args.0", "sum needs numbers"},
		"avg of amounts to cents": {`{"op":"avg","args":[{"col":"amount"}],"scale":2}`, "f.args.0", "use scale 0"},
		"link on a plain op":      {`{"op":"eq","args":[{"col":"n"},{"value":1}],"link":[{"link":"animal"}]}`, "f", "only exists and notExists"},
		"window with two values":  {`{"op":"runningSum","args":[{"col":"n"},{"col":"n"}]}`, "f", "takes one value"},
		"wrong arity":             {`{"op":"sub","args":[{"col":"n"}]}`, "f", "sub takes 2 values"},
		"unknown column":          {`{"col":"nope"}`, "f", "unknown column nope"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			ctx := &tableCtx{types: types, params: dbx.Params{}}
			_, err := compileFormula(ctx, parseNode(t, c.formula), "f")
			requireSpecError(t, err, 400, c.at, c.want)
		})
	}
}

func TestCompileFormulaLimitsDepth(t *testing.T) {
	raw := `{"col":"n"}`
	for range maxFormulaDepth + 2 {
		raw = `{"op":"add","args":[` + raw + `,{"value":1}]}`
	}
	ctx := &tableCtx{types: map[string]FieldType{"n": Number}, params: dbx.Params{}}
	_, err := compileFormula(ctx, parseNode(t, raw), "f")
	if err == nil || !strings.Contains(err.Error(), "at most 32 levels") {
		t.Fatalf("err = %v", err)
	}
}

func TestCompileFormulaTypes(t *testing.T) {
	types := map[string]FieldType{"amount": Money, "n": Number, "day": DateTime}
	cases := map[string]FieldType{
		`{"op":"add","args":[{"col":"amount"},{"value":100}]}`:              Money,
		`{"op":"mul","args":[{"col":"amount"},{"col":"n"}]}`:                Money,
		`{"op":"div","args":[{"col":"amount"},{"col":"amount"}],"scale":2}`: Number,
		`{"op":"div","args":[{"col":"amount"},{"col":"n"}],"scale":0}`:      Money,
		`{"op":"avg","args":[{"col":"amount"}],"scale":0}`:                  Money,
		`{"op":"count","args":[{"col":"day"}]}`:                             Number,
		`{"op":"addMonths","args":[{"col":"day"},{"value":1}]}`:             Date,
		`{"op":"startOfMonth","args":[{"col":"day"}]}`:                      Date,
		`{"op":"coalesce","args":[{"value":null},{"col":"n"}]}`:             Number,
	}
	for raw, want := range cases {
		ctx := &tableCtx{types: types, params: dbx.Params{}}
		e, err := compileFormula(ctx, parseNode(t, raw), "f")
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if e.typ != want {
			t.Errorf("%s: type %q, want %q", raw, e.typ, want)
		}
	}
}

func TestLiteralsBindAndNeverInline(t *testing.T) {
	ctx := &tableCtx{types: map[string]FieldType{"name": Text}, params: dbx.Params{}}
	e, err := compileFormula(ctx, parseNode(t, `{"op":"eq","args":[{"col":"name"},{"value":"x'); DROP TABLE users; --"}]}`), "f")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(e.sql, "DROP") || ctx.params["p0"] != "x'); DROP TABLE users; --" {
		t.Fatalf("sql %q params %v", e.sql, ctx.params)
	}
}
