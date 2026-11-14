// Package classifier sorts one record into one label of an app's taxonomy
// with the TypeSafe Jev model, through OpenRouter's Decisions API. It is
// generic: an app supplies the taxonomy and the evidence, and decides what to
// do with the result.
package classifier

const (
	Model         = "~typesafe/jev-latest"
	StageMetadata = "metadata"
	StageFull     = "full"
)

// TaxonomyLabel is one option the model may choose. ID is the
// classifier_labels row, LabelID the core labels row it applies.
type TaxonomyLabel struct {
	ID         string
	LabelID    string
	Name       string
	AssignWhen string
	AutoAction bool
}

// Result is the model's decision for one record. Probabilities are keyed by
// label Name. Confidence is Jev's separation measure, not the chosen
// option's probability.
type Result struct {
	Label          TaxonomyLabel
	Confidence     float64
	Probabilities  map[string]float64
	ArgmaxMismatch bool
	Stage          string
	Model          string
	CostUSD        float64
}
