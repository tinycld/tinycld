package forms

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const cliDir = "../../../cli/"

// The CLI's example definition is a valid web form, as the server decides.
func TestCLIExampleDefinitionIsValid(t *testing.T) {
	raw, err := os.ReadFile(cliDir + "form_examples/definition.json")
	if err != nil {
		t.Fatal(err)
	}
	def, err := ParseDefinition(raw)
	if err != nil {
		t.Fatal(err)
	}
	// Mirrors core:web_form in core/lib/forms/core-hosts.ts (WEB_FORM_KINDS
	// and its record columns): the example must pass that host, not an
	// open one. Change both together.
	host := &Host{Key: "core:web_form", Label: "Web form",
		Kinds:   []string{"text", "textarea", "number", "date", "toggle", "select", "checklist", "file", "computed", "table"},
		Columns: []string{"id", "form", "publication", "values", "client_id", "ip_hash", "user_agent", "created"}}
	if errs := ValidateDefinition(def, host); errs != nil {
		t.Fatalf("the example the CLI prints is refused: %v", errs)
	}
}

// The CLI's definition help names every top-level key and field property
// the server reads, so an agent learns the whole format from --help.
func TestCLIDefinitionHelpNamesEveryKey(t *testing.T) {
	help, err := os.ReadFile(cliDir + "form_help/definition.txt")
	if err != nil {
		t.Fatal(err)
	}
	text := string(help)
	for _, typ := range []reflect.Type{reflect.TypeOf(Definition{}), reflect.TypeOf(Field{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
			if name == "" || name == "expression" || name == "columns" || name == "collaborative" {
				continue
			}
			if !strings.Contains(text, `"`+name+`"`) {
				t.Errorf("definition help does not name %q (%s)", name, typ.Name())
			}
		}
	}
	for _, kind := range CustomKinds() {
		if !strings.Contains(text, kind) {
			t.Errorf("definition help does not name the kind %q", kind)
		}
	}
}
