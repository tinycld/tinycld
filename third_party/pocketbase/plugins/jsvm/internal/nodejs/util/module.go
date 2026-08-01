package util

import (
	"bytes"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/require"
)

const ModuleName = "util"

type Util struct {
	runtime *sobek.Runtime
}

func (u *Util) format(f rune, val sobek.Value, w *bytes.Buffer) bool {
	switch f {
	case 's':
		w.WriteString(val.String())
	case 'd':
		w.WriteString(val.ToNumber().String())
	case 'j':
		if json, ok := u.runtime.Get("JSON").(*sobek.Object); ok {
			if stringify, ok := sobek.AssertFunction(json.Get("stringify")); ok {
				res, err := stringify(json, val)
				if err != nil {
					panic(err)
				}
				w.WriteString(res.String())
			}
		}
	case '%':
		w.WriteByte('%')
		return false
	default:
		w.WriteByte('%')
		w.WriteRune(f)
		return false
	}
	return true
}

func (u *Util) Format(b *bytes.Buffer, f string, args ...sobek.Value) {
	pct := false
	argNum := 0
	for _, chr := range f {
		if pct {
			if argNum < len(args) {
				if u.format(chr, args[argNum], b) {
					argNum++
				}
			} else {
				b.WriteByte('%')
				b.WriteRune(chr)
			}
			pct = false
		} else {
			if chr == '%' {
				pct = true
			} else {
				b.WriteRune(chr)
			}
		}
	}

	for _, arg := range args[argNum:] {
		b.WriteByte(' ')
		b.WriteString(arg.String())
	}
}

func (u *Util) js_format(call sobek.FunctionCall) sobek.Value {
	var b bytes.Buffer
	var fmt string

	if arg := call.Argument(0); !sobek.IsUndefined(arg) {
		fmt = arg.String()
	}

	var args []sobek.Value
	if len(call.Arguments) > 0 {
		args = call.Arguments[1:]
	}
	u.Format(&b, fmt, args...)

	return u.runtime.ToValue(b.String())
}

func Require(runtime *sobek.Runtime, module *sobek.Object) {
	u := &Util{
		runtime: runtime,
	}
	obj := module.Get("exports").(*sobek.Object)
	obj.Set("format", u.js_format)
}

func New(runtime *sobek.Runtime) *Util {
	return &Util{
		runtime: runtime,
	}
}

func init() {
	require.RegisterCoreModule(ModuleName, Require)
}
