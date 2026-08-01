package process

import (
	"os"
	"strings"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
)

const ModuleName = "process"

type Process struct {
	env  map[string]string
	argv []string
}

func Require(runtime *sobek.Runtime, module *sobek.Object) {
	p := &Process{
		env: make(map[string]string),
	}

	for _, e := range os.Environ() {
		envKeyValue := strings.SplitN(e, "=", 2)
		p.env[envKeyValue[0]] = envKeyValue[1]
	}

	o := module.Get("exports").(*sobek.Object)
	o.Set("env", p.env)
	o.Set("argv", p.argv)
}

func Enable(runtime *sobek.Runtime) {
	runtime.Set("process", require.Require(runtime, ModuleName))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
