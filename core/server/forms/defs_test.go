package forms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadHosts_ReadsTheFixture(t *testing.T) {
	defs, err := LoadHosts(filepath.Join("testdata", "form_hosts.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := defs.Host("gizmos:widget")
	if h == nil || h.Collection != "gizmos_widgets" {
		t.Fatalf("host = %+v", h)
	}
	if b := h.Builtin("tasks"); b == nil || b.Storage != "rows" {
		t.Fatalf("tasks builtin = %+v", b)
	}
	if b := h.Builtin("notes"); b == nil || b.Chrome != "bare" {
		t.Fatalf("notes builtin keeps its chrome: %+v", b)
	}
	if h.InsertCustomBefore != "notes" {
		t.Fatalf("insertCustomBefore = %q", h.InsertCustomBefore)
	}
	if h.Builtin("ghost") != nil {
		t.Fatal("unknown builtin must be nil")
	}
	if o := defs.Host("gizmos:other"); o == nil || o.Collection != "gizmos_others" {
		t.Fatalf("other host = %+v", o)
	}
}

func TestLoadHosts_MissingFileIsEmpty(t *testing.T) {
	defs, err := LoadHosts(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || len(defs.Hosts) != 0 {
		t.Fatalf("defs=%+v err=%v", defs, err)
	}
}

func TestLoadHosts_InvalidJSONIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "form_hosts.json")
	if err := os.WriteFile(path, []byte(`{"hosts":[`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadHosts(path); err == nil {
		t.Fatal("a hosts file that does not parse must be an error")
	}
}

func TestResolveHosts(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.json")
	broken := filepath.Join(dir, "broken.json")
	if err := os.WriteFile(broken, []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	embedded := []byte(`{"hosts":[{"key":"gizmos:widget","collection":"gizmos_widgets"}]}`)
	if defs, err := resolveHosts(Options{HostsPath: missing}); err != nil || len(defs.Hosts) != 0 {
		t.Fatalf("a missing file is a lean shell: defs=%+v err=%v", defs, err)
	}
	if defs, err := resolveHosts(Options{HostsPath: missing, HostsJSON: embedded}); err != nil || len(defs.Hosts) != 1 {
		t.Fatalf("the embedded copy stands in for a missing file: defs=%+v err=%v", defs, err)
	}
	if _, err := resolveHosts(Options{HostsPath: broken, HostsJSON: embedded}); err == nil {
		t.Fatal("a file that does not parse must be an error, not a fallback")
	}
	if _, err := resolveHosts(Options{HostsPath: missing, HostsJSON: []byte(`{`)}); err == nil {
		t.Fatal("an embedded copy that does not parse must be an error")
	}
}

func TestParseHosts_DefaultsTheFileColumns(t *testing.T) {
	defs, err := ParseHosts([]byte(`{"hosts":[
		{"key":"gizmos:a","files":{"collection":"gizmos_files"}},
		{"key":"gizmos:b","files":{"collection":"gizmos_files","nameColumn":"title","fileColumn":"blob"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if f := defs.Host("gizmos:a").Files; f.NameColumn != "name" || f.FileColumn != "file" {
		t.Fatalf("defaults = %+v", f)
	}
	if f := defs.Host("gizmos:b").Files; f.NameColumn != "title" || f.FileColumn != "blob" {
		t.Fatalf("declared = %+v", f)
	}
}

func loadHostsJSON(t *testing.T, body string) (*Defs, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "form_hosts.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadHosts(path)
}

// linesHost is one host with a `lines` table built-in; builtins are spliced in.
func linesHost(builtins string) string {
	return `{"hosts":[{"key":"gizmos:bill","label":"Bill","collection":"gizmos_bills","formColumn":"form",` +
		`"fieldsColumn":"fields","columns":["name","form","fields"],"builtin":[` + builtins + `]}]}`
}

const linesColumns = `"columns":[{"key":"product","kind":"text","label":"Product","required":true},` +
	`{"key":"qty","kind":"number","label":"Qty","addOnSubmit":true},{"key":"text","kind":"text","label":"Text"}]`

func TestLoadHosts_RefusesARowKindOnAColumnTheTableLacks(t *testing.T) {
	body := linesHost(`{"key":"lines","label":"Lines","kind":"table","storage":"rows",` + linesColumns +
		`,"rowKinds":[{"key":"item","label":"Item","addLabel":"Add line","columns":["product","ghost"]}]}`)
	if _, err := loadHostsJSON(t, body); err == nil {
		t.Fatal("a row kind naming a column the table lacks must be refused")
	}
}

func TestLoadHosts_RefusesRowKindsOnABuiltinThatIsNotATable(t *testing.T) {
	body := linesHost(`{"key":"name","label":"Name","kind":"text","storage":"column",` +
		`"rowKinds":[{"key":"item","label":"Item","addLabel":"Add line","columns":["name"]}]}`)
	if _, err := loadHostsJSON(t, body); err == nil {
		t.Fatal("row kinds on a built-in that is not a table must be refused")
	}
}

func TestLoadHosts_DecodesLockedRowKindsAndAddOnSubmit(t *testing.T) {
	body := linesHost(`{"key":"name","label":"Name","kind":"text","storage":"column","locked":true},` +
		`{"key":"lines","label":"Lines","kind":"table","storage":"rows",` + linesColumns + `,"rowShortcuts":true,` +
		`"rowKinds":[{"key":"item","label":"Item","addLabel":"Add line","columns":["product","qty"],"shortcut":"Alt+Enter"},` +
		`{"key":"note","label":"Note","addLabel":"Note line","columns":["text"],"span":true,"shortcut":"Alt+t"},` +
		`{"key":"pct","label":"Percent","addLabel":"Percent line","columns":["product","text"],"columnAt":{"text":"qty"}}]}`)
	defs, err := loadHostsJSON(t, body)
	if err != nil {
		t.Fatal(err)
	}
	h := defs.Host("gizmos:bill")
	if b := h.Builtin("name"); b == nil || !b.Locked {
		t.Fatalf("name must be locked: %+v", b)
	}
	lines := h.Builtin("lines")
	if lines == nil || !lines.RowShortcuts || len(lines.RowKinds) != 3 {
		t.Fatalf("lines = %+v", lines)
	}
	item, note, pct := lines.RowKinds[0], lines.RowKinds[1], lines.RowKinds[2]
	if item.Key != "item" || item.Label != "Item" || item.AddLabel != "Add line" || item.Shortcut != "Alt+Enter" ||
		len(item.Columns) != 2 || item.Columns[0] != "product" || item.Columns[1] != "qty" || item.Span {
		t.Fatalf("item kind = %+v", item)
	}
	if !note.Span || note.Shortcut != "Alt+t" || len(note.Columns) != 1 || note.Columns[0] != "text" {
		t.Fatalf("note kind = %+v", note)
	}
	if pct.ColumnAt["text"] != "qty" {
		t.Fatalf("pct kind = %+v", pct)
	}
	if !lines.Columns[1].AddOnSubmit || lines.Columns[0].AddOnSubmit {
		t.Fatalf("addOnSubmit is on qty only: %+v", lines.Columns)
	}
}

func TestLoadHosts_DecodesTheInsertShortcutsOfATable(t *testing.T) {
	body := linesHost(`{"key":"lines","label":"Lines","kind":"table","storage":"rows",` + linesColumns +
		`,"rowShortcuts":true,"insertShortcuts":{"below":"Alt+Enter"}},` +
		`{"key":"notes","label":"Notes","kind":"table","storage":"rows",` + linesColumns + `}`)
	defs, err := loadHostsJSON(t, body)
	if err != nil {
		t.Fatal(err)
	}
	h := defs.Host("gizmos:bill")
	lines := h.Builtin("lines")
	if lines == nil || lines.InsertShortcuts == nil {
		t.Fatalf("lines must keep its insert shortcuts: %+v", lines)
	}
	if lines.InsertShortcuts.Below != "Alt+Enter" || lines.InsertShortcuts.Above != "" {
		t.Fatalf("insert shortcuts = %+v", *lines.InsertShortcuts)
	}
	if notes := h.Builtin("notes"); notes == nil || notes.InsertShortcuts != nil {
		t.Fatalf("a table without insertShortcuts decodes them as nil: %+v", notes)
	}
}

func TestParseHosts_DecodesAHostColumnFlexWeight(t *testing.T) {
	defs, err := ParseHosts([]byte(linesHost(`{"key":"lines","label":"Lines","kind":"table","storage":"rows",` +
		`"columns":[{"key":"product","kind":"text","label":"Product"},` +
		`{"key":"text","kind":"text","label":"Text","flex":3}]}`)))
	if err != nil {
		t.Fatal(err)
	}
	lines := defs.Host("gizmos:bill").Builtin("lines")
	if lines == nil || len(lines.Columns) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines.Columns[0].Flex != nil {
		t.Fatalf("a column without flex decodes no weight: %v", *lines.Columns[0].Flex)
	}
	if got := lines.Columns[1].Flex; got == nil || *got != 3 {
		t.Fatalf("text column flex = %v, want 3", got)
	}
}

func TestParseHosts_DecodesAHostColumnMinWidth(t *testing.T) {
	defs, err := ParseHosts([]byte(linesHost(`{"key":"lines","label":"Lines","kind":"table","storage":"rows",` +
		`"columns":[{"key":"product","kind":"text","label":"Product"},` +
		`{"key":"rate","kind":"number","label":"Rate","minWidth":96}]}`)))
	if err != nil {
		t.Fatal(err)
	}
	lines := defs.Host("gizmos:bill").Builtin("lines")
	if lines == nil || len(lines.Columns) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	if lines.Columns[0].MinWidth != nil {
		t.Fatalf("a column without minWidth decodes no floor: %v", *lines.Columns[0].MinWidth)
	}
	if got := lines.Columns[1].MinWidth; got == nil || *got != 96 {
		t.Fatalf("rate column minWidth = %v, want 96", got)
	}
}

// requiredLines is a `lines` table whose description kind must fill its description;
// product is required on every kind (table-wide), note is hidden, taxable a toggle.
const requiredLinesColumns = `"columns":[{"key":"product","kind":"text","label":"Product","required":true},` +
	`{"key":"description","kind":"text","label":"Description"},` +
	`{"key":"rate","kind":"number","label":"Rate"},` +
	`{"key":"note","kind":"text","label":"Note","hidden":true},` +
	`{"key":"taxable","kind":"toggle","label":"Tax"},` +
	`{"key":"amount","kind":"computed","label":"Amount","expression":{"var":"rate"}}]`

func requiredLinesHost(kinds string) string {
	return linesHost(`{"key":"lines","label":"Lines","kind":"table","storage":"rows",` + requiredLinesColumns +
		`,"rowKinds":[` + kinds + `]}`)
}

const (
	itemKindJSON        = `{"key":"item","label":"Item","addLabel":"Add line","columns":["product","description","rate","taxable","amount"]}`
	descriptionKindJSON = `{"key":"description","label":"Description","addLabel":"Description line","columns":["description"],"span":true,"required":["description"]}`
	percentKindJSON     = `{"key":"percent","label":"Percent","addLabel":"Percent line","columns":["product","rate","note","taxable","amount"],"required":["rate","note","taxable","amount"]}`
)

func requiredLines(t *testing.T) *Builtin {
	t.Helper()
	defs, err := loadHostsJSON(t, requiredLinesHost(itemKindJSON+","+descriptionKindJSON+","+percentKindJSON))
	if err != nil {
		t.Fatal(err)
	}
	lines := defs.Host("gizmos:bill").Builtin("lines")
	if lines == nil {
		t.Fatal("no lines built-in")
	}
	return lines
}

func TestParseHosts_DecodesTheRequiredColumnsOfARowKind(t *testing.T) {
	lines := requiredLines(t)
	if got := lines.RowKinds[0].Required; len(got) != 0 {
		t.Fatalf("item kind requires nothing of its own: %v", got)
	}
	if got := lines.RowKinds[1].Required; len(got) != 1 || got[0] != "description" {
		t.Fatalf("description kind required = %v, want [description]", got)
	}
}

func TestParseHosts_RefusesARequiredKeyThatIsNotAColumnOfTheKind(t *testing.T) {
	for name, kind := range map[string]string{
		"not a table column":    `{"key":"description","label":"Description","addLabel":"Description line","columns":["description"],"span":true,"required":["ghost"]}`,
		"another kind's column": `{"key":"description","label":"Description","addLabel":"Description line","columns":["description"],"span":true,"required":["product"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseHosts([]byte(requiredLinesHost(itemKindJSON + "," + kind)))
			if err == nil {
				t.Fatal("a required key that is not one of the kind's columns must be refused")
			}
			if !strings.Contains(err.Error(), "is not a column of this kind") {
				t.Fatalf("error = %q, want it to contain %q", err, "is not a column of this kind")
			}
		})
	}
}

func TestParseHosts_RefusesARequiredKeyNamedTwice(t *testing.T) {
	kind := `{"key":"description","label":"Description","addLabel":"Description line","columns":["description"],"span":true,"required":["description","description"]}`
	_, err := ParseHosts([]byte(requiredLinesHost(itemKindJSON + "," + kind)))
	if err == nil {
		t.Fatal("a required key named twice must be refused")
	}
	if !strings.Contains(err.Error(), "duplicate required column description") {
		t.Fatalf("error = %q, want it to contain %q", err, "duplicate required column description")
	}
}

func TestRowError_RequiresTheKindsOwnRequiredColumns(t *testing.T) {
	lines := requiredLines(t)
	for name, row := range map[string]map[string]any{
		"empty":  {"id": "d1", "kind": "description", "description": ""},
		"spaces": {"id": "d1", "kind": "description", "description": "   "},
		"nil":    {"id": "d1", "kind": "description", "description": nil},
		"absent": {"id": "d1", "kind": "description"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := lines.RowError(row); got != "Description is required" {
				t.Fatalf("RowError = %q, want %q", got, "Description is required")
			}
		})
	}
	if got := lines.RowError(map[string]any{"id": "d1", "kind": "description", "description": "Labor"}); got != "" {
		t.Fatalf("a filled description row: RowError = %q, want empty", got)
	}
}

func TestRowError_LetsAnItemRowLeaveTheDescriptionBlank(t *testing.T) {
	lines := requiredLines(t)
	row := map[string]any{"id": "i1", "kind": "item", "product": "Widget", "description": "  ", "rate": 5.0, "taxable": false}
	if got := lines.RowError(row); got != "" {
		t.Fatalf("RowError = %q, want empty", got)
	}
}

func TestRowError_StillRequiresATableWideRequiredColumn(t *testing.T) {
	lines := requiredLines(t)
	for name, row := range map[string]map[string]any{
		"item":    {"id": "i1", "kind": "item", "product": "", "rate": 5.0},
		"percent": {"id": "p1", "kind": "percent", "rate": 10.0, "taxable": true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := lines.RowError(row); got != "Product is required" {
				t.Fatalf("RowError = %q, want %q", got, "Product is required")
			}
		})
	}
}

func TestRowError_ReportsTheFirstMissingColumnInTheKindsOrder(t *testing.T) {
	lines := requiredLines(t)
	row := map[string]any{"id": "p1", "kind": "percent", "product": " ", "rate": nil}
	if got := lines.RowError(row); got != "Product is required" {
		t.Fatalf("RowError = %q, want %q", got, "Product is required")
	}
}

func TestRowError_NeverRequiresAHiddenToggleOrComputedColumn(t *testing.T) {
	lines := requiredLines(t)
	// The percent kind lists note (hidden), taxable (toggle) and amount (computed) as required.
	row := map[string]any{"id": "p1", "kind": "percent", "product": "Discount", "rate": 10.0, "note": "", "taxable": false}
	if got := lines.RowError(row); got != "" {
		t.Fatalf("RowError = %q, want empty", got)
	}
	row["rate"] = nil
	if got := lines.RowError(row); got != "Rate is required" {
		t.Fatalf("RowError = %q, want %q", got, "Rate is required")
	}
}

func TestRowError_RefusesARowWithAMissingOrUnknownKind(t *testing.T) {
	lines := requiredLines(t)
	for name, row := range map[string]map[string]any{
		"absent":  {"id": "r1", "product": "Widget", "description": "x"},
		"blank":   {"id": "r1", "kind": "", "product": "Widget", "description": "x"},
		"unknown": {"id": "r1", "kind": "other", "product": "Widget", "description": "x"},
	} {
		t.Run(name, func(t *testing.T) {
			if got := lines.RowError(row); got == "" {
				t.Fatal("a row whose kind is missing or unknown must be refused")
			}
		})
	}
}

func TestRowError_ChecksEveryColumnOfATableWithoutKinds(t *testing.T) {
	defs, err := loadHostsJSON(t, linesHost(`{"key":"lines","label":"Lines","kind":"table","storage":"rows",`+linesColumns+`}`))
	if err != nil {
		t.Fatal(err)
	}
	lines := defs.Host("gizmos:bill").Builtin("lines")
	if got := lines.RowError(map[string]any{"id": "r1", "product": " ", "qty": 1.0}); got != "Product is required" {
		t.Fatalf("RowError = %q, want %q", got, "Product is required")
	}
	if got := lines.RowError(map[string]any{"id": "r1", "product": "Widget"}); got != "" {
		t.Fatalf("RowError = %q, want empty", got)
	}
}

func TestParseHosts_DecodesALayoutNarrowWidth(t *testing.T) {
	defs, err := ParseHosts([]byte(`{"hosts":[{"key":"gizmos:gadget","label":"Gadget","collection":"gizmos_gadgets",` +
		`"formColumn":"form","fieldsColumn":"fields","columns":["name","tax","due","form","fields"],"builtin":[` +
		`{"key":"name","label":"Name","kind":"text","storage":"column"},` +
		`{"key":"tax","label":"Tax","kind":"text","storage":"column"},` +
		`{"key":"due","label":"Due","kind":"date","storage":"column"}],` +
		`"layout":{"pages":[{"key":"main","title":"Main"}],"fields":{` +
		`"tax":{"width":"auto","narrowWidth":"full"},"due":{"width":"quarter"}},"rules":[]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	layout := defs.Host("gizmos:gadget").Layout
	if layout == nil {
		t.Fatal("the host must keep its layout")
	}
	if tax := layout.Fields["tax"]; tax.Width != "auto" || tax.NarrowWidth != "full" {
		t.Fatalf("tax layout = %+v, want width auto and narrowWidth full", tax)
	}
	if due := layout.Fields["due"]; due.Width != "quarter" || due.NarrowWidth != "" {
		t.Fatalf("due layout = %+v, want width quarter and no narrowWidth", due)
	}
}
