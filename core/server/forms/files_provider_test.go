package forms

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/filesystem"
	"tinycld.org/core/automation"
	"tinycld.org/core/rlstest"
)

func TestFilesProviderReadsTheHostsFilesContract(t *testing.T) {
	t.Cleanup(automation.ResetRegistriesForTest)
	app := rlstest.NewAssembledApp(t)
	widgets := core.NewBaseCollection("gizmos_widgets")
	widgets.Fields.Add(&core.TextField{Name: "form"}, &core.JSONField{Name: "fields"})
	if err := app.Save(widgets); err != nil {
		t.Fatal(err)
	}
	files := core.NewBaseCollection("gizmos_files")
	files.Fields.Add(
		&core.TextField{Name: "widget"}, &core.TextField{Name: "field"},
		&core.NumberField{Name: "size"}, &core.TextField{Name: "name"},
		&core.TextField{Name: "uploaded_by"}, &core.FileField{Name: "file", MaxSelect: 1, MaxSize: 1 << 20},
	)
	if err := app.Save(files); err != nil {
		t.Fatal(err)
	}
	defs, err := ParseHosts([]byte(`{"hosts":[{"pkgSlug":"gizmos","key":"gizmos:widget","label":"Widget",
		"collection":"gizmos_widgets","formColumn":"form","fieldsColumn":"fields","columns":["id","form","fields"],
		"builtin":[],"files":{"collection":"gizmos_files","recordColumn":"widget","fieldColumn":"field",
		"sizeColumn":"size","uploaderColumn":"uploaded_by"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	registerFilesProviders(defs)

	w := core.NewRecord(widgets)
	if err := app.Save(w); err != nil {
		t.Fatal(err)
	}
	f := core.NewRecord(files)
	blob, _ := filesystem.NewFileFromBytes([]byte("hello"), "note.txt")
	f.Set("widget", w.Id)
	f.Set("field", "doc")
	f.Set("size", 5)
	f.Set("name", "note.txt")
	f.Set("file", blob)
	if err := app.Save(f); err != nil {
		t.Fatal(err)
	}

	got, err := automation.TriggerFiles(app, w)
	if err != nil || len(got) != 1 || got[0].Field != "doc" || got[0].Name != "note.txt" || got[0].Size != 5 {
		t.Fatalf("files not found: %#v %v", got, err)
	}
	read, err := automation.ReadTriggerFile(app, got[0])
	if err != nil || read.OriginalName != "note.txt" || read.Size != 5 {
		t.Fatalf("file not readable: %#v %v", read, err)
	}
}
