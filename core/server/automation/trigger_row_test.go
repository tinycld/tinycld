package automation

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func TestTriggerRowGivesLabelledTypedValues(t *testing.T) {
	app, _, u := engineApp(t)
	col := core.NewBaseCollection("gizmos")
	col.Fields.Add(&core.TextField{Name: "title"})
	col.Fields.Add(&core.NumberField{Name: "qty"})
	col.Fields.Add(&core.BoolField{Name: "urgent"})
	col.Fields.Add(&core.DateField{Name: "due"})
	users, _ := app.FindCollectionByNameOrId("users")
	col.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	u.Set("name", "Ada")
	if err := app.Save(u); err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("title", "Bolt")
	rec.Set("qty", 2.5)
	rec.Set("urgent", true)
	due, _ := types.ParseDateTime("2026-11-04 00:00:00.000Z")
	rec.Set("due", due)
	rec.Set("user", u.Id)
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	trigger := TriggerDef{ID: "gizmo", Collection: "gizmos", On: "create", Fields: []FieldRef{
		{Key: "title", Label: "Title"}, {Key: "qty"}, {Key: "urgent"}, {Key: "due"}, {Key: "user", Label: "Owner"},
	}}
	req := ActionRequest{OwnerID: u.Id, Record: rec, Trigger: trigger, Ctx: context.Background(),
		Fields: []DynamicField{{Key: "email", Label: "Email", Type: "text", Value: "a@b.example"}}}

	got := TriggerRow(app, req)
	want := []RowValue{
		{Label: "Title", Type: "text", Value: "Bolt"},
		{Label: "Qty", Type: "number", Value: 2.5},
		{Label: "Urgent", Type: "boolean", Value: true},
		{Label: "Due", Type: "date", Value: "2026-11-04"},
		{Label: "Owner", Type: "relation", Value: "Ada"},
		{Label: "Email", Type: "text", Value: "a@b.example"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d values %+v, want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("value %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestTriggerRowWithNoRecordIsOnlyTheDynamicFields(t *testing.T) {
	app, _, u := engineApp(t)
	req := ActionRequest{OwnerID: u.Id, Fields: []DynamicField{{Key: "a", Label: "A", Type: "number", Value: 3.0}}}
	got := TriggerRow(app, req)
	if len(got) != 1 || got[0].Value != 3.0 {
		t.Fatalf("got %+v", got)
	}
}

// An answer left empty, or one that is not a number, leaves its cell empty
// rather than writing 0 or FALSE.
func TestTriggerRowEmptyAnswersStayEmpty(t *testing.T) {
	app, _, u := engineApp(t)
	req := ActionRequest{OwnerID: u.Id, Fields: []DynamicField{
		{Key: "qty", Label: "Qty", Type: "number", Value: ""},
		{Key: "size", Label: "Size", Type: "number", Value: "big"},
		{Key: "ok", Label: "Ok", Type: "boolean", Value: nil},
		{Key: "n", Label: "N", Type: "number", Value: "4.5"},
	}}
	got := TriggerRow(app, req)
	if got[0].Value != nil || got[1].Value != nil || got[2].Value != nil {
		t.Fatalf("empty or unreadable answers give nil, got %+v", got)
	}
	if got[3].Value != 4.5 {
		t.Fatalf("a number given as text is read, got %+v", got[3])
	}
}
