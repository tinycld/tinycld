package forms

import (
	"os"
	"path/filepath"
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
