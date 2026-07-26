package console

import (
	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/util"
)

const ModuleName = "console"

type Console struct {
	runtime *sobek.Runtime
	util    *sobek.Object
	printer Printer
}

type Printer interface {
	Log(string)
	Warn(string)
	Error(string)
}

func (c *Console) log(p func(string)) func(sobek.FunctionCall) sobek.Value {
	return func(call sobek.FunctionCall) sobek.Value {
		if format, ok := sobek.AssertFunction(c.util.Get("format")); ok {
			ret, err := format(c.util, call.Arguments...)
			if err != nil {
				panic(err)
			}

			p(ret.String())
		} else {
			panic(c.runtime.NewTypeError("util.format is not a function"))
		}

		return nil
	}
}

func Require(runtime *sobek.Runtime, module *sobek.Object) {
	requireWithPrinter(defaultStdPrinter)(runtime, module)
}

func RequireWithPrinter(printer Printer) require.ModuleLoader {
	return requireWithPrinter(printer)
}

func requireWithPrinter(printer Printer) require.ModuleLoader {
	return func(runtime *sobek.Runtime, module *sobek.Object) {
		c := &Console{
			runtime: runtime,
			printer: printer,
		}

		c.util = require.Require(runtime, util.ModuleName).(*sobek.Object)

		o := module.Get("exports").(*sobek.Object)
		o.Set("log", c.log(c.printer.Log))
		o.Set("error", c.log(c.printer.Error))
		o.Set("warn", c.log(c.printer.Warn))
		o.Set("info", c.log(c.printer.Log))
		o.Set("debug", c.log(c.printer.Log))
	}
}

func Enable(runtime *sobek.Runtime) {
	runtime.Set("console", require.Require(runtime, ModuleName))
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
