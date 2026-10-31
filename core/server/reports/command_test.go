package reports

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/rlstest"
)

// commandApp is a real PocketBase app on a data dir with core's migrations
// applied, as `tinycld reports validate` finds a deployment's pb_data.
func commandApp(t *testing.T) *pocketbase.PocketBase {
	t.Helper()
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	app := pocketbase.NewWithConfig(pocketbase.Config{DefaultDataDir: rlstest.AssembledDataDir(t), HideStartBanner: true})
	if err := app.Bootstrap(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.ResetBootstrapState() })
	return app
}

func runValidate(t *testing.T, app *pocketbase.PocketBase, args ...string) (string, error) {
	t.Helper()
	cmd := NewCommand(app)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(append([]string{"validate"}, args...))
	err := cmd.Execute()
	return out.String(), err
}

func TestReportsValidateListsBrokenReportsWithTheirOwner(t *testing.T) {
	app := commandApp(t)
	out, err := runValidate(t, app)
	if err != nil || !strings.Contains(out, "checked 0 saved reports, 0 broken") {
		t.Fatalf("an empty deployment: %v\n%s", err, out)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	ann := core.NewRecord(users)
	ann.SetEmail("ann@zoo.test")
	ann.SetPassword("password123")
	ann.Set("name", "Ann")
	ann.Set("username", "ann")
	ann.Set("role", "member")
	if err := app.Save(ann); err != nil {
		t.Fatal(err)
	}
	reports, err := app.FindCollectionByNameOrId(reportsCollection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(reports)
	r.Set("owner", ann.Id)
	r.Set("title", "Feed costs")
	r.Set("spec", goneSpec)
	r.Set("source", "zoo.gone")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}

	out, err = runValidate(t, app)
	if err == nil || err.Error() != "1 of 1 saved reports are broken" {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{
		"checked 1 saved reports, 1 broken",
		r.Id + `  "Feed costs"  owner Ann <ann@zoo.test>`,
		`  source: unknown_source: the source "zoo.gone" does not exist`,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	stored, _ := app.FindRecordById(reportsCollection, r.Id)
	if !strings.Contains(stored.GetString("broken"), `"unknown_source"`) {
		t.Fatalf("the command did not store the mark: %s", stored.GetString("broken"))
	}
}

func TestReportsValidateTakesNoArguments(t *testing.T) {
	app := commandApp(t)
	if _, err := runValidate(t, app, "extra"); err == nil {
		t.Fatal("validate accepted an argument")
	}
}
