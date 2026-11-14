package classifier

import "github.com/pocketbase/pocketbase/core"

type Row struct {
	App, Collection, RecordID, User string
	ClassifierLabelID, LabelName    string
	Stage, Model, Action, Error     string
	Confidence, CostUSD             float64
	Probabilities                   map[string]float64
	Preview                         bool
}

// SaveRow writes the one result row for (collection, record, user). A later
// run for the same record (a backfill, a retry) replaces it.
func SaveRow(app core.App, row Row) error {
	rec, err := app.FindFirstRecordByFilter("classifications",
		"collection = {:c} && record_id = {:r} && user = {:u}",
		map[string]any{"c": row.Collection, "r": row.RecordID, "u": row.User})
	if err != nil {
		col, cerr := app.FindCollectionByNameOrId("classifications")
		if cerr != nil {
			return cerr
		}
		rec = core.NewRecord(col)
	}
	rec.Set("app", row.App)
	rec.Set("collection", row.Collection)
	rec.Set("record_id", row.RecordID)
	rec.Set("user", row.User)
	rec.Set("classifier_label", row.ClassifierLabelID)
	rec.Set("label_name", row.LabelName)
	rec.Set("confidence", row.Confidence)
	rec.Set("probabilities", row.Probabilities)
	rec.Set("stage", row.Stage)
	rec.Set("model", row.Model)
	rec.Set("cost_usd", row.CostUSD)
	rec.Set("preview", row.Preview)
	rec.Set("action", row.Action)
	rec.Set("error", row.Error)
	return app.Save(rec)
}
