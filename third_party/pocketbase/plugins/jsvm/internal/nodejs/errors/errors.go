package errors

import (
	"fmt"

	"github.com/grafana/sobek"
)

const (
	ErrCodeInvalidArgType  = "ERR_INVALID_ARG_TYPE"
	ErrCodeInvalidArgValue = "ERR_INVALID_ARG_VALUE"
	ErrCodeInvalidThis     = "ERR_INVALID_THIS"
	ErrCodeMissingArgs     = "ERR_MISSING_ARGS"
	ErrCodeOutOfRange      = "ERR_OUT_OF_RANGE"
)

func error_toString(call sobek.FunctionCall, r *sobek.Runtime) sobek.Value {
	this := call.This.ToObject(r)
	var name, msg string
	if n := this.Get("name"); n != nil && !sobek.IsUndefined(n) {
		name = n.String()
	} else {
		name = "Error"
	}
	if m := this.Get("message"); m != nil && !sobek.IsUndefined(m) {
		msg = m.String()
	}
	if code := this.Get("code"); code != nil && !sobek.IsUndefined(code) {
		if name != "" {
			name += " "
		}
		name += "[" + code.String() + "]"
	}
	if msg != "" {
		if name != "" {
			name += ": "
		}
		name += msg
	}
	return r.ToValue(name)
}

func addProps(r *sobek.Runtime, e *sobek.Object, code string) {
	e.Set("code", code)
	e.DefineDataProperty("toString", r.ToValue(error_toString), sobek.FLAG_TRUE, sobek.FLAG_TRUE, sobek.FLAG_FALSE)
}

func NewTypeError(r *sobek.Runtime, code string, params ...interface{}) *sobek.Object {
	e := r.NewTypeError(params...)
	addProps(r, e, code)
	return e
}

func NewRangeError(r *sobek.Runtime, code string, params ...interface{}) *sobek.Object {
	ctor, _ := r.Get("RangeError").(*sobek.Object)
	return NewError(r, ctor, code, params...)
}

func NewError(r *sobek.Runtime, ctor *sobek.Object, code string, args ...interface{}) *sobek.Object {
	if ctor == nil {
		ctor, _ = r.Get("Error").(*sobek.Object)
	}
	if ctor == nil {
		return nil
	}
	msg := ""
	if len(args) > 0 {
		f, _ := args[0].(string)
		msg = fmt.Sprintf(f, args[1:]...)
	}
	o, err := r.New(ctor, r.ToValue(msg))
	if err != nil {
		panic(err)
	}
	addProps(r, o, code)
	return o
}

func NewArgumentNotBigIntTypeError(r *sobek.Runtime, name string) *sobek.Object {
	return NewNotCorrectTypeError(r, name, "BigInt")
}

func NewArgumentNotStringTypeError(r *sobek.Runtime, name string) *sobek.Object {
	return NewNotCorrectTypeError(r, name, "string")
}

func NewArgumentNotNumberTypeError(r *sobek.Runtime, name string) *sobek.Object {
	return NewNotCorrectTypeError(r, name, "number")
}

func NewNotCorrectTypeError(r *sobek.Runtime, name, _type string) *sobek.Object {
	return NewTypeError(r, ErrCodeInvalidArgType, "The \"%s\" argument must be of type %s.", name, _type)
}

func NewArgumentOutOfRangeError(r *sobek.Runtime, name string, v any) *sobek.Object {
	return NewRangeError(r, ErrCodeOutOfRange, "The value of \"%s\" %v is out of range.", name, v)
}
