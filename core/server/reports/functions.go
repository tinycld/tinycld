package reports

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	sqlite "modernc.org/sqlite"

	"tinycld.org/core/forms"
)

// The SQLite functions the compiled SQL calls where SQLite has no exact
// equivalent. The modernc driver adds a registered function to every
// connection it opens afterwards, so they register at package init, before
// PocketBase opens its database.
func init() {
	sqlite.MustRegisterDeterministicScalarFunction("tc_div", 3, sqlDiv)
	sqlite.MustRegisterDeterministicScalarFunction("tc_pct", 3, sqlPct)
	sqlite.MustRegisterDeterministicScalarFunction("tc_round", 2, sqlRound)
	sqlite.MustRegisterDeterministicScalarFunction("tc_fiscal_year", 2, sqlFiscalYear)
	sqlite.MustRegisterDeterministicScalarFunction("tc_fiscal_period", 2, sqlFiscalPeriod)
	sqlite.MustRegisterDeterministicScalarFunction("tc_add_months", 2, sqlAddMonths)
}

// errOverflow has the text SQLite's own sum() uses, so the run maps both to
// one "amount too large" error.
var errOverflow = errors.New("integer overflow")

// MaxScale is the most decimal places a division or rounding keeps. The
// forms engine's round takes 0 to 6, and the shared formula table pins both.
const MaxScale = 6

// sqlDiv is tc_div(a, b, scale): a / b rounded half away from zero at scale
// decimal places. Two integers divide exactly (big-integer quotient and
// remainder, the rule of the shell's roundedQuotient); a real operand divides
// in float64, as the forms engine does. Division by zero is 0, as in the forms
// engine. NULL in, NULL out: the compiler coalesces formula operands to 0.
// Scale 0 returns an INTEGER, a larger scale a REAL.
func sqlDiv(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	scale, err := scaleArg(args[2])
	if err != nil {
		return nil, err
	}
	if args[0] == nil || args[1] == nil {
		return nil, nil
	}
	return divide(args[0], args[1], scale)
}

// sqlPct is tc_pct(part, whole, scale): part as a percent of whole, rounded
// like tc_div. With no whole (NULL or zero) there is no figure, so NULL: the
// shell shows nothing for a share of nothing.
func sqlPct(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	scale, err := scaleArg(args[2])
	if err != nil {
		return nil, err
	}
	if args[0] == nil || args[1] == nil || isZero(args[1]) {
		return nil, nil
	}
	part, err := times100(args[0])
	if err != nil {
		return nil, err
	}
	return divide(part, args[1], scale)
}

// sqlRound is tc_round(x, scale): x rounded half away from zero. An integer
// is already exact at any scale.
func sqlRound(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	scale, err := scaleArg(args[1])
	if err != nil {
		return nil, err
	}
	switch x := args[0].(type) {
	case nil:
		return nil, nil
	case int64:
		return x, nil
	case float64:
		return floatResult(forms.RoundHalfAway(x, scale), scale)
	}
	return nil, fmt.Errorf("tc_round: %T is not a number", args[0])
}

// sqlFiscalYear is tc_fiscal_year(day, startMonth): the fiscal year a day
// falls in, named by the calendar year the fiscal year ends in. With a
// January start it is the calendar year.
func sqlFiscalYear(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	day, start, ok, err := fiscalArgs(args)
	if !ok || err != nil {
		return nil, err
	}
	year := int64(day.Year())
	if start > 1 && int(day.Month()) >= start {
		year++
	}
	return year, nil
}

// sqlFiscalPeriod is tc_fiscal_period(day, startMonth): the month of the
// fiscal year a day falls in, 1 to 12.
func sqlFiscalPeriod(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	day, start, ok, err := fiscalArgs(args)
	if !ok || err != nil {
		return nil, err
	}
	return int64((int(day.Month())-start+12)%12 + 1), nil
}

// sqlAddMonths is tc_add_months(day, n): the day n months later, held to the
// last day of a shorter month (Jan 31 + 1 month is Feb 28 or 29). SQLite's
// date(day, '+1 month') rolls over into the next month instead.
func sqlAddMonths(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	day, ok := parseDay(args[0])
	if !ok {
		return nil, nil
	}
	n, ok := args[1].(int64)
	if !ok {
		if args[1] != nil {
			return nil, fmt.Errorf("tc_add_months: %T is not a whole number of months", args[1])
		}
		n = 0
	}
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, int(n), 0)
	last := first.AddDate(0, 1, -1).Day()
	return time.Date(first.Year(), first.Month(), min(day.Day(), last), 0, 0, 0, 0, time.UTC).Format(time.DateOnly), nil
}

func scaleArg(v driver.Value) (int, error) {
	s, ok := v.(int64)
	if !ok || s < 0 || s > MaxScale {
		return 0, fmt.Errorf("scale must be a whole number from 0 to %d", MaxScale)
	}
	return int(s), nil
}

func isZero(v driver.Value) bool {
	switch x := v.(type) {
	case int64:
		return x == 0
	case float64:
		return x == 0
	}
	return false
}

func times100(v driver.Value) (driver.Value, error) {
	switch x := v.(type) {
	case int64:
		if x > math.MaxInt64/100 || x < math.MinInt64/100 {
			return nil, errOverflow
		}
		return x * 100, nil
	case float64:
		return x * 100, nil
	}
	return nil, fmt.Errorf("%T is not a number", v)
}

func divide(a, b driver.Value, scale int) (driver.Value, error) {
	ai, aInt := a.(int64)
	bi, bInt := b.(int64)
	if aInt && bInt {
		if bi == 0 {
			return int64(0), nil
		}
		return bigResult(exactQuotient(big.NewInt(ai), big.NewInt(bi), scale), scale)
	}
	af, ok := toFloat(a)
	if !ok {
		return nil, fmt.Errorf("%T is not a number", a)
	}
	bf, ok := toFloat(b)
	if !ok {
		return nil, fmt.Errorf("%T is not a number", b)
	}
	if bf == 0 {
		return int64(0), nil
	}
	return floatResult(forms.RoundHalfAway(af/bf, scale), scale)
}

func toFloat(v driver.Value) (float64, bool) {
	switch x := v.(type) {
	case int64:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

// exactQuotient is a*10^scale / b rounded half away from zero, in integers.
func exactQuotient(a, b *big.Int, scale int) *big.Int {
	num := new(big.Int).Mul(a, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	q, r := new(big.Int).QuoRem(num, b, new(big.Int))
	twice := new(big.Int).Mul(new(big.Int).Abs(r), big.NewInt(2))
	if twice.Cmp(new(big.Int).Abs(b)) >= 0 {
		if (num.Sign() < 0) != (b.Sign() < 0) {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

func bigResult(q *big.Int, scale int) (driver.Value, error) {
	if scale == 0 {
		if !q.IsInt64() {
			return nil, errOverflow
		}
		return q.Int64(), nil
	}
	f, _ := new(big.Rat).SetFrac(q, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)).Float64()
	return f, nil
}

func floatResult(x float64, scale int) (driver.Value, error) {
	if scale > 0 {
		return x, nil
	}
	if x >= math.MaxInt64 || x < math.MinInt64 {
		return nil, errOverflow
	}
	return int64(x), nil
}

func fiscalArgs(args []driver.Value) (time.Time, int, bool, error) {
	start, ok := args[1].(int64)
	if !ok || start < 1 || start > 12 {
		return time.Time{}, 0, false, errors.New("fiscal start month must be 1 to 12")
	}
	day, ok := parseDay(args[0])
	return day, int(start), ok, nil
}

// parseDay reads the date part of a PocketBase date ("2026-01-31 00:00:00.000Z")
// or a plain day. Anything else, a blank date included, is no day.
func parseDay(v driver.Value) (time.Time, bool) {
	s, ok := v.(string)
	if !ok || len(s) < len(time.DateOnly) {
		return time.Time{}, false
	}
	day, err := time.Parse(time.DateOnly, s[:len(time.DateOnly)])
	return day, err == nil
}
