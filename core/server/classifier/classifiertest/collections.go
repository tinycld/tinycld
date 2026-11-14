// Package classifiertest builds the classifier collections in a Go test app,
// the way package tests build their own collections, because Go tests do not
// run the JS migrations.
package classifiertest

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// CreateCollections saves classifier_labels, classifications and
// classifier_usage. It creates a minimal "labels" collection when absent.
func CreateCollections(t testing.TB, app core.App) {
	t.Helper()
	labels, err := app.FindCollectionByNameOrId("labels")
	if err != nil {
		labels = core.NewBaseCollection("labels")
		labels.Fields.Add(&core.TextField{Name: "name", Required: true})
		labels.Fields.Add(&core.TextField{Name: "color"})
		labels.Fields.Add(&core.TextField{Name: "user"})
		must(t, app.Save(labels))
	}

	tax := core.NewBaseCollection("classifier_labels")
	tax.Fields.Add(&core.TextField{Name: "app", Required: true})
	tax.Fields.Add(&core.TextField{Name: "name", Required: true})
	// No cascade: a referenced label must not be deletable, or any member
	// could remove an org option by deleting the core label.
	tax.Fields.Add(&core.RelationField{Name: "label", Required: true, CollectionId: labels.Id, MaxSelect: 1, CascadeDelete: false})
	tax.Fields.Add(&core.TextField{Name: "assign_when", Required: true})
	tax.Fields.Add(&core.BoolField{Name: "auto_action"})
	tax.Fields.Add(&core.NumberField{Name: "order"})
	tax.Fields.Add(&core.TextField{Name: "user"})
	must(t, app.Save(tax))

	res := core.NewBaseCollection("classifications")
	for _, name := range []string{"app", "collection", "record_id", "user", "classifier_label", "label_name", "stage", "model", "action", "error"} {
		res.Fields.Add(&core.TextField{Name: name})
	}
	res.Fields.Add(&core.NumberField{Name: "confidence"})
	res.Fields.Add(&core.JSONField{Name: "probabilities"})
	res.Fields.Add(&core.NumberField{Name: "cost_usd"})
	res.Fields.Add(&core.BoolField{Name: "preview"})
	res.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	res.AddIndex("idx_classifications_record_user", true, "`collection`, `record_id`, `user`", "")
	must(t, app.Save(res))

	usage := core.NewBaseCollection("classifier_usage")
	usage.Fields.Add(&core.TextField{Name: "month", Required: true})
	usage.Fields.Add(&core.NumberField{Name: "cost_usd"})
	usage.Fields.Add(&core.NumberField{Name: "calls"})
	usage.AddIndex("idx_classifier_usage_month", true, "`month`", "")
	must(t, app.Save(usage))
}

func must(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("classifiertest: %v", err)
	}
}
