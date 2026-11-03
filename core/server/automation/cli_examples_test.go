package automation

import (
	"encoding/json"
	"os"
	"testing"
)

// The CLI's example destination passes rule validation against core's own
// notify action.
func TestCLIExampleDestinationIsValid(t *testing.T) {
	app, _, _ := engineApp(t)
	raw, err := os.ReadFile("../../../cli/form_examples/destination.json")
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Actions json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		t.Fatal(err)
	}
	defs := &Defs{Packages: []PackageDefs{{Slug: "core", Actions: []ActionDef{{ID: "notify", Kind: "native", Params: []ParamDef{
		{Key: "title", Type: "text", Required: true}, {Key: "body", Type: "text"}, {Key: "url", Type: "text"},
	}}}}}}
	if errs := ValidateRuleActions(app, defs, file.Actions); errs != nil {
		t.Fatalf("the example the CLI prints is refused: %v", errs)
	}
}
