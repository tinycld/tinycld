package goutil

import (
	"math/big"

	"github.com/grafana/sobek"
	"github.com/pocketbase/pocketbase/plugins/jsvm/internal/nodejs/errors"
)

func RequiredIntegerArgument(r *sobek.Runtime, call sobek.FunctionCall, name string, argIndex int) int64 {
	arg := call.Argument(argIndex)
	if sobek.IsNumber(arg) {
		return arg.ToInteger()
	}
	if sobek.IsUndefined(arg) {
		panic(errors.NewTypeError(r, errors.ErrCodeInvalidArgType, "The \"%s\" argument is required.", name))
	}

	panic(errors.NewArgumentNotNumberTypeError(r, name))
}

func RequiredFloatArgument(r *sobek.Runtime, call sobek.FunctionCall, name string, argIndex int) float64 {
	arg := call.Argument(argIndex)
	if sobek.IsNumber(arg) {
		return arg.ToFloat()
	}
	if sobek.IsUndefined(arg) {
		panic(errors.NewTypeError(r, errors.ErrCodeInvalidArgType, "The \"%s\" argument is required.", name))
	}

	panic(errors.NewArgumentNotNumberTypeError(r, name))
}

func CoercedIntegerArgument(call sobek.FunctionCall, argIndex int, defaultValue int64, typeMistMatchValue int64) int64 {
	arg := call.Argument(argIndex)
	if sobek.IsNumber(arg) {
		return arg.ToInteger()
	}
	if sobek.IsUndefined(arg) {
		return defaultValue
	}

	return typeMistMatchValue
}

func OptionalIntegerArgument(r *sobek.Runtime, call sobek.FunctionCall, name string, argIndex int, defaultValue int64) int64 {
	arg := call.Argument(argIndex)
	if sobek.IsNumber(arg) {
		return arg.ToInteger()
	}
	if sobek.IsUndefined(arg) {
		return defaultValue
	}

	panic(errors.NewArgumentNotNumberTypeError(r, name))
}

func RequiredBigIntArgument(r *sobek.Runtime, call sobek.FunctionCall, name string, argIndex int) *big.Int {
	arg := call.Argument(argIndex)
	if sobek.IsUndefined(arg) {
		panic(errors.NewTypeError(r, errors.ErrCodeInvalidArgType, "The \"%s\" argument is required.", name))
	}
	if !sobek.IsBigInt(arg) {
		panic(errors.NewArgumentNotBigIntTypeError(r, name))
	}

	n, _ := arg.Export().(*big.Int)
	if n == nil {
		n = new(big.Int)
	}
	return n
}

func RequiredStringArgument(r *sobek.Runtime, call sobek.FunctionCall, name string, argIndex int) string {
	arg := call.Argument(argIndex)
	if sobek.IsString(arg) {
		return arg.String()
	}
	if sobek.IsUndefined(arg) {
		panic(errors.NewTypeError(r, errors.ErrCodeInvalidArgType, "The \"%s\" argument is required.", name))
	}

	panic(errors.NewArgumentNotStringTypeError(r, name))
}
