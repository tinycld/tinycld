package reports

import (
	"fmt"
	"reflect"
	"testing"
)

// everyV4 uses every part version 4 adds: the one-line gate, every column
// fit setting, and a layout's own gate.
const everyV4 = `{"version":4,"source":"zoo.feedings","columns":[
	{"id":"animal","label":"Animal","path":[{"link":"animal"}]},
	{"id":"note","label":"Note","path":[{"field":"note"}]},
	{"id":"cost","label":"Cost","path":[{"field":"cost"}]}],
	"display":{"oneLine":{"minWidth":"1040px"},
		"columns":{
			"note":{"width":"20ch","minWidth":"12ch","maxWidth":"60ch","whiteSpace":"normal","lineClamp":2,"textOverflow":"ellipsis","flexGrow":2},
			"cost":{"minWidth":"max-content","flexGrow":0}},
		"layouts":[{"width":"tablet","minWidth":"600px","tracks":2,"cells":[
			{"col":"animal","line":1,"span":2},{"col":"note","line":2},{"col":"cost","line":2}]}]}}`

func TestASpecWithEveryV4FieldLoadsAndIsStoredUnchanged(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, stored := mustLoad(t, everyV4)
	if !reflect.DeepEqual(jsonDoc(t, []byte(everyV4)), jsonDoc(t, stored)) {
		t.Fatalf("stored spec differs from the posted one:\n%s", stored)
	}
	if err := Validate(app, spec); err != nil {
		t.Fatal(err)
	}
	d := spec.Display
	note := d.Columns["note"]
	if d.OneLine.MinWidth != "1040px" || d.Layouts[0].MinWidth != "600px" || note.LineClamp != 2 || note.FlexGrow == nil || *note.FlexGrow != 2 {
		t.Fatalf("display = %+v", d)
	}
	// A flexGrow of 0 is a setting (no share of the spare room), not "absent".
	if g := d.Columns["cost"].FlexGrow; g == nil || *g != 0 {
		t.Fatalf("cost flexGrow = %v", g)
	}
}

func TestColumnFitIsChecked(t *testing.T) {
	app := withZooRegistry(t)
	base := `{"version":4,"source":"zoo.feedings","columns":[{"id":"animal","path":[{"link":"animal"}]},{"id":"note","path":[{"field":"note"}]}],"display":%s}`
	note := func(fit string) string { return `{"columns":{"note":` + fit + `}}` }
	for name, c := range map[string]struct {
		display, at, want string
		status            int
	}{
		"a length with no unit":       {note(`{"minWidth":"12"}`), "display.columns.note.minWidth", "minWidth is auto, min-content, max-content or a length in ch, px or %", 400},
		"a keyword width":             {note(`{"width":"auto"}`), "display.columns.note.width", "width is a length in ch, px or %", 400},
		"maxWidth auto":               {note(`{"maxWidth":"auto"}`), "display.columns.note.maxWidth", "maxWidth is none or a length in ch, px or %", 400},
		"ch above 120":                {note(`{"minWidth":"121ch"}`), "display.columns.note.minWidth", "a length in ch is 1 to 120", 400},
		"px above 4000":               {note(`{"width":"4001px"}`), "display.columns.note.width", "a length in px is 1 to 4000", 400},
		"zero percent":                {note(`{"maxWidth":"0%"}`), "display.columns.note.maxWidth", "a length in % is 1 to 100", 400},
		"a negative length":           {note(`{"minWidth":"-8ch"}`), "display.columns.note.minWidth", "minWidth is auto", 400},
		"min above max":               {note(`{"minWidth":"30ch","maxWidth":"20ch"}`), "display.columns.note.minWidth", "minWidth is at most maxWidth", 400},
		"whiteSpace pre":              {note(`{"whiteSpace":"pre"}`), "display.columns.note.whiteSpace", "whiteSpace is nowrap or normal", 400},
		"lineClamp without normal":    {note(`{"lineClamp":2}`), "display.columns.note.lineClamp", "lineClamp goes with whiteSpace normal", 400},
		"lineClamp 5":                 {note(`{"whiteSpace":"normal","lineClamp":5}`), "display.columns.note.lineClamp", "lineClamp is 1 to 4", 400},
		"clip is not offered":         {note(`{"textOverflow":"clip"}`), "display.columns.note.textOverflow", "textOverflow is ellipsis", 400},
		"flexGrow 11":                 {note(`{"flexGrow":11}`), "display.columns.note.flexGrow", "flexGrow is 0 to 10", 400},
		"flexGrow below 0":            {note(`{"flexGrow":-1}`), "display.columns.note.flexGrow", "flexGrow is 0 to 10", 400},
		"an unknown column":           {`{"columns":{"zz":{"flexGrow":1}}}`, "display.columns.zz", "the report has no column", 422},
		"oneLine in ch":               {`{"oneLine":{"minWidth":"80ch"}}`, "display.oneLine.minWidth", "minWidth is a length in px", 400},
		"oneLine with no width":       {`{"oneLine":{}}`, "display.oneLine.minWidth", "minWidth is a length in px", 400},
		"a layout gate in percent":    {`{"layouts":[{"width":"tablet","minWidth":"50%","tracks":1,"cells":[{"col":"note","line":1}]}]}`, "display.layouts.0.minWidth", "minWidth is a length in px", 400},
		"a layout gate above 4000 px": {`{"layouts":[{"width":"tablet","minWidth":"5000px","tracks":1,"cells":[{"col":"note","line":1}]}]}`, "display.layouts.0.minWidth", "a length in px is 1 to 4000", 400},
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(base, c.display)))
			if err != nil {
				t.Fatal(err)
			}
			requireSpecError(t, Validate(app, spec), c.status, c.at, c.want)
		})
	}
	for name, display := range map[string]string{
		"keywords":                       note(`{"minWidth":"min-content","maxWidth":"none"}`),
		"a percent width":                note(`{"width":"25%"}`),
		"a decimal length":               note(`{"minWidth":"7.5ch"}`),
		"min and max in different units": note(`{"minWidth":"30ch","maxWidth":"200px"}`),
		"an empty fit":                   note(`{}`),
		"wrap with the default clamp":    note(`{"whiteSpace":"normal"}`),
	} {
		t.Run(name, func(t *testing.T) {
			spec, _, err := Load([]byte(fmt.Sprintf(base, display)))
			if err != nil {
				t.Fatal(err)
			}
			if err := Validate(app, spec); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Strict decode: a fit key the engine does not know is refused by Load.
	_, _, err := Load([]byte(fmt.Sprintf(base, note(`{"min":"8ch"}`))))
	requireSpecError(t, err, 400, "spec", "unknown field")
}
