package reports

import (
	"database/sql/driver"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	sqlite "modernc.org/sqlite"
)

// sqlText is tc_text(value, type, currency): a value as the report shell
// shows it in a cell (core/reports/lib/format.ts, en-US), so a record shown
// by a field that is not text reads as its cell would: Yes or No, an amount
// with its currency, a grouped number, a short day. NULL in, NULL out.
func sqlText(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
	if args[0] == nil {
		return nil, nil
	}
	typ, _ := args[1].(string)
	code, _ := args[2].(string)
	return valueText(args[0], FieldType(typ), code), nil
}

func valueText(v driver.Value, typ FieldType, code string) string {
	switch typ {
	case Bool:
		if truthy(v) {
			return "Yes"
		}
		return "No"
	case Money:
		if cents, ok := toFloat(v); ok {
			return moneyText(cents, code)
		}
	case Number:
		if x, ok := toFloat(v); ok {
			return numberText(x)
		}
	case Percent:
		if x, ok := toFloat(v); ok {
			return strconv.FormatFloat(roundHalfAway(x*10)/10, 'f', 1, 64) + "%"
		}
	case Date, DateTime:
		if s, ok := v.(string); ok {
			return dayText(s)
		}
	}
	return plainText(v)
}

func truthy(v driver.Value) bool {
	switch x := v.(type) {
	case int64:
		return x != 0
	case float64:
		return x != 0
	case bool:
		return x
	case string:
		return x == "true" || x == "1"
	}
	return false
}

func plainText(v driver.Value) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// roundHalfAway rounds to a whole number, half away from zero, as the shell's
// roundHalfAwayFromZero does.
func roundHalfAway(x float64) float64 {
	return math.Copysign(math.Round(math.Abs(x)*(1+epsilon)), x)
}

// epsilon is JavaScript's Number.EPSILON, the shell's nudge for a value like
// 1000.4999999999999 that a reader takes for a half.
const epsilon = 2.220446049250313e-16

// maxSafe is JavaScript's Number.MAX_SAFE_INTEGER.
const maxSafe = 1<<53 - 1

// groupDigits writes a whole number's digits in groups of three.
func groupDigits(digits string) string {
	var b strings.Builder
	for i, d := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	return b.String()
}

// numberText is the shell's formatNumber: a whole number grouped, any other
// with at most six decimals.
func numberText(x float64) string {
	sign := ""
	if x < 0 {
		sign, x = "-", -x
	}
	if x == math.Trunc(x) && x <= maxSafe {
		return sign + groupDigits(strconv.FormatInt(int64(x), 10))
	}
	whole, frac, _ := strings.Cut(strconv.FormatFloat(x, 'f', 6, 64), ".")
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return sign + groupDigits(whole)
	}
	return sign + groupDigits(whole) + "." + frac
}

// currencySymbols are the symbols en-US Intl shows for a currency; any other
// well-formed code is written before the amount.
var currencySymbols = map[string]string{
	"USD": "$", "EUR": "€", "GBP": "£", "JPY": "¥", "CAD": "CA$", "AUD": "A$",
	"NZD": "NZ$", "HKD": "HK$", "MXN": "MX$", "BRL": "R$", "CNY": "CN¥",
	"INR": "₹", "KRW": "₩", "ILS": "₪", "TWD": "NT$", "VND": "₫", "PHP": "₱",
	"XCD": "EC$",
}

// moneyText is the shell's formatMoney (core/reports/lib/money.ts) for an
// amount in integer cents: the currency's symbol, or its code and a no-break
// space, then the amount with two decimals; a code Intl refuses follows the
// amount; no currency, the amount alone.
func moneyText(cents float64, code string) string {
	n := int64(roundHalfAway(cents))
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	amount := groupDigits(strconv.FormatInt(n/100, 10)) + "." + fmt.Sprintf("%02d", n%100)
	switch {
	case code == "":
		return sign + amount
	case !isCurrencyCode(code):
		return sign + amount + " " + code
	}
	upper := strings.ToUpper(code)
	if symbol, ok := currencySymbols[upper]; ok {
		return sign + symbol + amount
	}
	return sign + upper + " " + amount
}

// isCurrencyCode is true for a code Intl accepts: three ASCII letters.
func isCurrencyCode(code string) bool {
	if len(code) != 3 {
		return false
	}
	for _, r := range code {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
			return false
		}
	}
	return true
}

var shortMonths = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// dayText is the shell's dayText: a day as "Jan 5, 2026", any other text as
// stored.
func dayText(s string) string {
	if len(s) < len(time.DateOnly) {
		return s
	}
	day, err := time.Parse(time.DateOnly, s[:len(time.DateOnly)])
	if err != nil || day.Year() < 1900 {
		return s
	}
	return fmt.Sprintf("%s %d, %d", shortMonths[day.Month()-1], day.Day(), day.Year())
}
