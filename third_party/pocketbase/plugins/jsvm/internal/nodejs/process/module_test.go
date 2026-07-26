package process

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
)

func TestProcessEnvStructure(t *testing.T) {
	vm := sobek.New()

	new(require.Registry).Enable(vm)
	Enable(vm)

	if c := vm.Get("process"); c == nil {
		t.Fatal("process not found")
	}

	if c, err := vm.RunString("process.env"); c == nil || err != nil {
		t.Fatal("error accessing process.env")
	}
}

func TestProcessEnvValuesArtificial(t *testing.T) {
	os.Setenv("sobek_IS_AWESOME", "true")
	defer os.Unsetenv("sobek_IS_AWESOME")

	vm := sobek.New()

	new(require.Registry).Enable(vm)
	Enable(vm)

	jsRes, err := vm.RunString("process.env['sobek_IS_AWESOME']")

	if err != nil {
		t.Fatalf("Error executing: %s", err)
	}

	if jsRes.String() != "true" {
		t.Fatalf("Error executing: got %s but expected %s", jsRes, "true")
	}
}

func TestProcessEnvValuesBrackets(t *testing.T) {
	vm := sobek.New()

	new(require.Registry).Enable(vm)
	Enable(vm)

	for _, e := range os.Environ() {
		envKeyValue := strings.SplitN(e, "=", 2)
		jsExpr := fmt.Sprintf("process.env['%s']", envKeyValue[0])

		jsRes, err := vm.RunString(jsExpr)

		if err != nil {
			t.Fatalf("Error executing %s: %s", jsExpr, err)
		}

		if jsRes.String() != envKeyValue[1] {
			t.Fatalf("Error executing %s: got %s but expected %s", jsExpr, jsRes, envKeyValue[1])
		}
	}
}
