package reports

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The text of an exported report: a cell, a row label, a column heading, the
// period, the zone line, the params line and the empty text, as the report
// view writes them (core/reports/lib/format.ts, params.ts, dates.ts,
// date-time.ts, money.ts). Each function is a port of the TS function its
// comment names. core/reports/lib/fixtures/format-cases.json holds one case
// list that the vitest suite runs against the TS and export_format_test.go
// runs against this file, so the two cannot drift.

// exportTablesJSON holds what Intl gives the client and Go has not: the text
// before the digits of an amount in each currency, and each time zone in
// words. core/scripts/gen-export-tables.ts writes it from Intl in en-US.
//
//go:embed export_tables.json
var exportTablesJSON []byte

var exportTables = func() struct {
	Currencies map[string]string `json:"currencies"`
	Zones      map[string]string `json:"zones"`
} {
	var t struct {
		Currencies map[string]string `json:"currencies"`
		Zones      map[string]string `json:"zones"`
	}
	if err := json.Unmarshal(exportTablesJSON, &t); err != nil {
		panic("reports: export_tables.json: " + err.Error())
	}
	return t
}()

// noBreakSpace is the space Intl puts between a currency code and its amount.
const noBreakSpace = "\xc2\xa0"

// maxSafeAmount is JavaScript's Number.MAX_SAFE_INTEGER (MAX_STORED_AMOUNT in
// money.ts): a larger amount cannot be shown truthfully.
const maxSafeAmount = 1<<53 - 1

// reportAmountError is the text of an amount too large to show
// (REPORT_AMOUNT_ERROR).
const reportAmountError = "Too large"

// exportDisplay is the client's DisplayOptions (format.ts), as the export
// request's options.display carries it.
type exportDisplay struct {
	DivideBy1000 bool   `json:"divideBy1000"`
	WithoutCents bool   `json:"withoutCents"`
	Negative     string `json:"negative"`
	NegativeRed  bool   `json:"negativeRed"`
	// Hidden are the columns the reader hid; the first column is never
	// hidden.
	Hidden []string `json:"hidden"`
}

// defaultDisplay is DEFAULT_DISPLAY: a request with no display options.
var defaultDisplay = exportDisplay{Negative: "minus", Hidden: []string{}}

// exportFormatter writes the text of a report for one reader: the display
// options, the reader's zone and hour cycle.
type exportFormatter struct {
	display  exportDisplay
	timeZone string
	// loc is the reader's zone (readerZone); nil is UTC.
	loc    *time.Location
	hour12 bool
}

func newExportFormatter(display exportDisplay, timeZone string, hour12 bool) *exportFormatter {
	return &exportFormatter{display: display, timeZone: timeZone, loc: readerZone(timeZone), hour12: hour12}
}

// cellText is a cell's shown text (CellDisplay without its alignment).
type cellText struct {
	Text    string
	IsRed   bool
	IsError bool
}

// rawCell is a cell's raw value as the CSV writes it, and the kind of value
// it is, which the Excel writer types its cell from: money, number, percent,
// date, datetime, bool or text.
type rawCell struct {
	Text string
	Kind string
}

// Raw cell kinds.
const (
	rawMoney    = "money"
	rawNumber   = "number"
	rawPercent  = "percent"
	rawDate     = "date"
	rawDateTime = "datetime"
	rawBool     = "bool"
	rawText     = "text"
)

// numberOf reads a cell value as a number: a run gives whole figures as
// int64, JSON as float64, a param as json.Number. ok is false for any other
// value.
func numberOf(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case int32:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// jsNumberText is String(n) in JavaScript for a finite number: a whole number
// without a fraction, else the shortest text that reads back as n.
func jsNumberText(n float64) string {
	switch {
	case math.IsNaN(n):
		return "NaN"
	case math.IsInf(n, 1):
		return "Infinity"
	case math.IsInf(n, -1):
		return "-Infinity"
	case n == 0:
		return "0"
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// valueString is String(value) of a key or a text cell.
func valueString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if n, ok := numberOf(v); ok {
		return jsNumberText(n)
	}
	if b, ok := v.(bool); ok {
		return strconv.FormatBool(b)
	}
	return fmt.Sprint(v)
}

// isSafeInteger is Number.isSafeInteger.
func isSafeInteger(n float64) bool {
	return !math.IsInf(n, 0) && n == math.Trunc(n) && math.Abs(n) <= maxSafeAmount
}

// roundHalfAwayFromZero (money.ts): the epsilon nudge makes
// 1000.4999999999999 round as a person reading the decimal expects.
func roundHalfAwayFromZero(v float64) float64 {
	r := math.Floor(math.Abs(v)*(1+epsilon) + 0.5)
	if v < 0 {
		return -r
	}
	return r
}

// epsilon is Number.EPSILON.
const epsilon = 2.220446049250313e-16

// groupDigits (money.ts) writes a whole magnitude with a comma every three
// digits.
func groupDigits(digits string) string {
	if len(digits) <= 3 {
		return digits
	}
	var b strings.Builder
	lead := len(digits) % 3
	if lead > 0 {
		b.WriteString(digits[:lead])
	}
	for i := lead; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// roundedQuotient (format.ts): integer cents divided by divisor, half away
// from zero, exact for integers.
func roundedQuotient(cents, divisor int64) int64 {
	magnitude := cents
	if magnitude < 0 {
		magnitude = -magnitude
	}
	remainder := magnitude % divisor
	quotient := (magnitude - remainder) / divisor
	if 2*remainder >= divisor {
		quotient++
	}
	if cents < 0 {
		return -quotient
	}
	return quotient
}

// displayUnits (format.ts): the one rounding of an amount, to whole cents of
// the display unit; ok is false when the amount cannot be stored, so it
// cannot be shown either.
func (f *exportFormatter) displayUnits(cents float64) (int64, bool) {
	if math.IsNaN(cents) || math.IsInf(cents, 0) || math.Abs(cents) > maxSafeAmount {
		return 0, false
	}
	centsPerUnit := int64(1)
	if f.display.WithoutCents {
		centsPerUnit = 100
	}
	divisor := centsPerUnit
	if f.display.DivideBy1000 {
		divisor *= 1000
	}
	var rounded int64
	if cents == math.Trunc(cents) {
		rounded = roundedQuotient(int64(cents), divisor)
	} else {
		rounded = int64(roundHalfAwayFromZero(cents / float64(divisor)))
	}
	return rounded * centsPerUnit, true
}

// centsDigits writes whole cents as "1,234.50" (grouped) or "1234.50".
func centsDigits(magnitude int64, grouped bool) string {
	whole := strconv.FormatInt(magnitude/100, 10)
	if grouped {
		whole = groupDigits(whole)
	}
	return fmt.Sprintf("%s.%02d", whole, magnitude%100)
}

// plainAmount (format.ts) is a magnitude of cents with no symbol.
func plainAmount(magnitude int64) string { return centsDigits(magnitude, true) }

// wholeCentsTail is the ".00" withoutCents drops: the last one with no digit
// after it.
var wholeCentsTail = regexp.MustCompile(`\.00(\D*)$`)

// unitsText (format.ts).
func (f *exportFormatter) unitsText(units int64, currency string, withSymbol bool) string {
	magnitude := units
	if magnitude < 0 {
		magnitude = -magnitude
	}
	amount := plainAmount(magnitude)
	if withSymbol {
		amount = formatMoney(float64(magnitude), currency)
	}
	body := amount
	if f.display.WithoutCents {
		body = wholeCentsTail.ReplaceAllString(amount, "$1")
	}
	switch {
	case units >= 0:
		return body
	case f.display.Negative == "parens":
		return "(" + body + ")"
	}
	return "-" + body
}

// formatReportAmount (format.ts).
func (f *exportFormatter) formatReportAmount(cents float64, currency string, withSymbol bool) string {
	units, ok := f.displayUnits(cents)
	if !ok {
		return reportAmountError
	}
	return f.unitsText(units, currency, withSymbol)
}

// isCurrencyCode is Intl's well-formed currency code: three ASCII letters, in
// any case.
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

// formatMoney (money.ts): integer cents in a currency, as Intl writes them
// in en-US: a currency of the table with its symbol ("$1,234.50"), a
// well-formed code Intl does not know with the code and a no-break space
// ("QQQ 1,234.50"), any other code after the plain amount ("1,234.50 ZOOX").
// An amount under half a cent is zero, so it never reads "-$0.00".
func formatMoney(cents float64, currency string) string {
	if math.IsNaN(cents) || math.IsInf(cents, 0) {
		return "—"
	}
	if math.Abs(cents) < 0.5 {
		cents = 0
	}
	sign := ""
	if cents < 0 {
		sign = "-"
	}
	digits := moneyDigits(math.Abs(cents))
	upper := strings.ToUpper(currency)
	switch {
	case currency == "":
		return sign + digits
	case exportTables.Currencies[upper] != "":
		return sign + exportTables.Currencies[upper] + digits
	case isCurrencyCode(currency):
		return sign + upper + noBreakSpace + digits
	}
	return sign + digits + " " + currency
}

// moneyDigits (formatCents in money.ts) writes a magnitude of cents grouped
// with two decimals: whole cents from their digits, a fraction or an amount
// past 2^53 through a float, as Intl formats cents / 100.
func moneyDigits(magnitude float64) string {
	if isSafeInteger(magnitude) {
		return plainAmount(int64(magnitude))
	}
	text := strconv.FormatFloat(magnitude/100, 'f', 2, 64)
	whole, fraction, _ := strings.Cut(text, ".")
	return groupDigits(whole) + "." + fraction
}

// formatReportPercent (format.ts): value is already a percent, 12.345 shows
// as 12.3%; a share of nothing shows nothing.
func formatReportPercent(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return ""
	}
	r := roundHalfAwayFromZero(value*10) / 10
	if r == 0 {
		r = 0 // toFixed writes -0 as 0
	}
	return strconv.FormatFloat(r, 'f', 1, 64) + "%"
}

// formatNumber (format.ts): a safe integer grouped; else at most 6 fraction
// digits, grouped, as toLocaleString('en-US') writes it: the exact value of
// the double rounded half away from zero, and a negative that rounds to zero
// keeps its minus ("-0").
func formatNumber(value float64) string {
	if isSafeInteger(value) {
		sign := ""
		if value < 0 {
			sign = "-"
		}
		return sign + groupDigits(strconv.FormatInt(int64(math.Abs(value)), 10))
	}
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "∞"
	case math.IsInf(value, -1):
		return "-∞"
	}
	whole, fraction := roundSixPlaces(math.Abs(value))
	fraction = strings.TrimRight(fraction, "0")
	sign := ""
	if value < 0 {
		sign = "-"
	}
	out := sign + groupDigits(whole)
	if fraction != "" {
		out += "." + fraction
	}
	return out
}

// exactFractionDigits is enough fraction digits to write any float64 exactly
// (the smallest subnormal has 1074).
const exactFractionDigits = 1074

// roundSixPlaces rounds a finite, non-negative double to 6 fraction digits
// from its exact decimal digits, half away from zero, as Intl's halfExpand
// does; strconv's own rounding is half to even on a tie (1.0078125).
func roundSixPlaces(magnitude float64) (string, string) {
	exact := strconv.FormatFloat(magnitude, 'f', exactFractionDigits, 64)
	whole, fraction, _ := strings.Cut(exact, ".")
	digits := []byte(whole + fraction[:6])
	if fraction[6] >= '5' {
		i := len(digits) - 1
		for ; i >= 0 && digits[i] == '9'; i-- {
			digits[i] = '0'
		}
		if i < 0 {
			digits = append([]byte{'1'}, digits...)
		} else {
			digits[i]++
		}
	}
	split := len(digits) - 6
	return string(digits[:split]), string(digits[split:])
}

// cellType (format.ts): group, subtotal, computed and total cells take the
// column's total type when it has one.
func cellType(col ResultColumn, row Row) FieldType {
	isTotalRow := row.Kind != KindDetail && row.Kind != KindOpening
	if isTotalRow && col.TotalType != "" {
		return col.TotalType
	}
	return col.Type
}

// cellCurrency (format.ts): the column's fixed currency, else the row's
// currency cell.
func cellCurrency(col ResultColumn, row Row) string {
	if col.Currency != "" {
		return col.Currency
	}
	if col.CurrencyCell != "" {
		if s, ok := row.Cells[col.CurrencyCell].(string); ok {
			return s
		}
	}
	return ""
}

// symbolRows are SYMBOL_ROWS: the rows whose amounts carry a symbol.
var symbolRows = []string{KindSubtotal, KindComputed, KindTotal}

// blankLinkLabel is BLANK_LINK_LABEL.
const blankLinkLabel = "(none)"

// isBlankLabel says a link's label cell holds no label.
func isBlankLabel(v any) bool {
	return v == nil || v == ""
}

// linkLabelColumn is the column a link cell's label is formatted as.
func linkLabelColumn(col ResultColumn) ResultColumn {
	t := col.LabelType
	if t == "" {
		t = Text
	}
	return ResultColumn{ID: col.LabelCell, Label: col.Label, Type: t, CurrencyCell: col.LabelCurrencyCell}
}

// blankLinkText (linkDisplay): a detail row's blank link reads (none); a group
// or total row with no value in the column shows nothing.
func blankLinkText(row Row, value any) string {
	if value == nil && row.Kind != KindDetail {
		return ""
	}
	return blankLinkLabel
}

// CellText is cellDisplay (format.ts), a date-time through formatDateTime
// in the reader's zone and hour cycle. A nil value is the cell's null; a
// writer does not ask for a cell the row does not have.
func (f *exportFormatter) CellText(col ResultColumn, row Row, value any) cellText {
	t := cellType(col, row)
	if col.LabelCell != "" && t == Relation {
		label := row.Cells[col.LabelCell]
		if isBlankLabel(label) {
			return cellText{Text: blankLinkText(row, value)}
		}
		return f.CellText(linkLabelColumn(col), row, label)
	}
	if value == nil {
		return cellText{}
	}
	if n, ok := numberOf(value); ok {
		switch {
		case t == Money:
			return f.moneyText(col, row, n)
		case t == Percent || col.Format == "percent":
			return cellText{Text: formatReportPercent(n)}
		}
		return cellText{Text: formatNumber(n)}
	}
	if b, ok := value.(bool); ok {
		if b {
			return cellText{Text: "Yes"}
		}
		return cellText{Text: "No"}
	}
	s, isString := value.(string)
	switch {
	case t == DateTime && isString:
		return cellText{Text: f.formatDateTime(s)}
	case t == Date && isString:
		return cellText{Text: dayText(s)}
	case t == Select:
		return cellText{Text: selectLabel(col, value)}
	}
	return cellText{Text: valueString(value)}
}

// moneyText is moneyDisplay (format.ts).
func (f *exportFormatter) moneyText(col ResultColumn, row Row, cents float64) cellText {
	units, ok := f.displayUnits(cents)
	if !ok {
		return cellText{Text: reportAmountError, IsError: true}
	}
	return cellText{
		Text:  f.unitsText(units, cellCurrency(col, row), slices.Contains(symbolRows, row.Kind)),
		IsRed: f.display.NegativeRed && units < 0,
	}
}

// selectLabel is a select cell's option label, else its value.
func selectLabel(col ResultColumn, value any) string {
	for _, o := range col.Options {
		if s, ok := value.(string); ok && o.Value == s {
			return o.Label
		}
	}
	return valueString(value)
}

// dayText (format.ts): a date cell's day in the short form; a value that is
// no day shows as it is.
func dayText(value string) string {
	day := value
	if len(day) > 10 {
		day = day[:10]
	}
	if isDay(day) {
		return shortDay(day)
	}
	return value
}

var (
	monthNames      = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	shortMonthNames = []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
)

// monthDay (dates.ts): "November 15" of a YYYY-MM-DD day.
func monthDay(day string, names []string) string {
	month, _ := strconv.Atoi(day[5:7])
	d, _ := strconv.Atoi(day[8:10])
	return fmt.Sprintf("%s %d", names[month-1], d)
}

// dayWords (dates.ts): "November 15, 2026".
func dayWords(day string) string { return monthDay(day, monthNames) + ", " + day[:4] }

// shortDay (dates.ts): "Nov 15, 2026".
func shortDay(day string) string { return monthDay(day, shortMonthNames) + ", " + day[:4] }

var (
	monthKey   = regexp.MustCompile(`^(\d{4})-(\d{2})$`)
	quarterKey = regexp.MustCompile(`^(\d{4})-Q([1-4])$`)
	yearKey    = regexp.MustCompile(`^\d{4}$`)
)

// isBucketType is isBucketLabelType (types.ts).
func isBucketType(t FieldType) bool {
	switch t {
	case Month, Quarter, Year, FiscalYear, Day:
		return true
	}
	return false
}

// bucketWords (dates.ts): month "Jan 2026", quarter "Q1 2026", year "2026",
// fiscal year "FY 2026", day "Jan 5, 2026"; a key not in its bucket's form
// shows as it is.
func bucketWords(t FieldType, key any) string {
	text := valueString(key)
	switch t {
	case Month:
		if m := monthKey.FindStringSubmatch(text); m != nil {
			if month, _ := strconv.Atoi(m[2]); month >= 1 && month <= 12 {
				return shortMonthNames[month-1] + " " + m[1]
			}
		}
	case Quarter:
		if m := quarterKey.FindStringSubmatch(text); m != nil {
			return "Q" + m[2] + " " + m[1]
		}
	case FiscalYear:
		if yearKey.MatchString(text) {
			return "FY " + text
		}
	case Day:
		if isDay(text) {
			return shortDay(text)
		}
	}
	return text
}

// rangeText (dates.ts): one day for a one-day range; the first year is
// dropped when both ends share it.
func rangeText(from, to string, withYear, withoutYear func(string) string) string {
	if from == to {
		return withYear(to)
	}
	start := withYear(from)
	if from[:4] == to[:4] {
		start = withoutYear(from)
	}
	return start + " – " + withYear(to)
}

// rangeWords (dates.ts): "January 1 – March 31, 2026".
func rangeWords(from, to string) string {
	return rangeText(from, to, dayWords, func(day string) string { return monthDay(day, monthNames) })
}

// rangeChipText (dates.ts): "Jan 1 – Mar 31, 2026".
func rangeChipText(from, to string) string {
	return rangeText(from, to, shortDay, func(day string) string { return monthDay(day, shortMonthNames) })
}

// instantPattern is INSTANT (date-time.ts): an instant as PocketBase stores
// it.
var instantPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})[ T](\d{2}):(\d{2})(:(\d{2})(\.(\d{1,3}))?)?Z$`)

// instantOf (date-time.ts) reads an instant; ok is false for any other text.
func instantOf(value string) (time.Time, bool) {
	m := instantPattern.FindStringSubmatch(value)
	if m == nil {
		return time.Time{}, false
	}
	seconds := m[5]
	if seconds == "" {
		seconds = "00"
	}
	millis := m[7]
	for len(millis) < 3 {
		millis += "0"
	}
	t, err := time.Parse("2006-01-02 15:04:05.000", m[1]+" "+m[2]+":"+m[3]+":"+seconds+"."+millis)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// wallClock is an instant on the reader's wall clock (nil zone: UTC).
func (f *exportFormatter) wallClock(t time.Time) time.Time {
	if f.loc == nil {
		return t.UTC()
	}
	return t.In(f.loc)
}

// clockText (date-time.ts): "4:07 PM" or "16:07".
func clockText(hour, minute int, hour12 bool) string {
	if !hour12 {
		return fmt.Sprintf("%02d:%02d", hour, minute)
	}
	h := hour % 12
	if h == 0 {
		h = 12
	}
	period := "AM"
	if hour >= 12 {
		period = "PM"
	}
	return fmt.Sprintf("%d:%02d %s", h, minute, period)
}

// formatDateTime (date-time.ts): an instant as "Nov 14, 2026, 4:07 PM" (or
// "Nov 14, 2026, 16:07") on the reader's wall clock; a bare day as the short
// day; any other text as it is.
func (f *exportFormatter) formatDateTime(value string) string {
	if isDay(value) {
		return shortDay(value)
	}
	t, ok := instantOf(value)
	if !ok {
		return value
	}
	local := f.wallClock(t)
	return shortDay(local.Format(time.DateOnly)) + ", " + clockText(local.Hour(), local.Minute(), f.hour12)
}

// typedLabelText (format.ts): a typed label (a row's, a pivot heading's)
// formatted as a cell of its type; an amount always shows its currency.
func (f *exportFormatter) typedLabelText(t FieldType, value any, currency string) string {
	if isBucketType(t) {
		if _, isString := value.(string); isString {
			return bucketWords(t, value)
		}
		if _, isNumber := numberOf(value); isNumber {
			return bucketWords(t, value)
		}
	}
	if n, ok := numberOf(value); ok && t == Money {
		return f.formatReportAmount(n, currency, true)
	}
	return f.CellText(ResultColumn{Type: t, Currency: currency}, Row{Kind: KindDetail, Cells: map[string]any{}}, value).Text
}

// RowLabel is rowLabelText (format.ts): a typed label formatted, a
// subtotal's with "Total " before it; else the row's label.
func (f *exportFormatter) RowLabel(row Row) string {
	if row.LabelType == "" || row.LabelValue == nil {
		return row.Label
	}
	text := f.typedLabelText(row.LabelType, row.LabelValue, row.LabelCurrency)
	if row.Kind == KindSubtotal {
		return "Total " + text
	}
	return text
}

// Heading is headingText (format.ts): the column's run heading, a pivot
// column over typed values by its typed label, else its label.
func (f *exportFormatter) Heading(col ResultColumn) string {
	if col.Heading != nil && col.Heading.Value != nil {
		return f.typedLabelText(col.Heading.Type, col.Heading.Value, "")
	}
	p := col.Pivot
	if p == nil || p.LabelType == "" || p.LabelValue == nil {
		return col.Label
	}
	return f.typedLabelText(p.LabelType, p.LabelValue, p.LabelCurrency)
}

// allDatesLabel is PRESET_LABELS.allDates.
const allDatesLabel = "All dates"

// PeriodWords is periodWords (params.ts): the days the server ran, "" when
// the client's answer is null.
func (f *exportFormatter) PeriodWords(def *ParamDef, resolved *ResolvedParam) string {
	if def == nil || resolved == nil || resolved.Unset {
		return ""
	}
	if def.Type == ParamDateRange {
		if resolved.From != "" && resolved.To != "" {
			return rangeWords(resolved.From, resolved.To)
		}
		return allDatesLabel
	}
	day, ok := resolved.Value.(string)
	if !ok || !isDay(day) {
		return ""
	}
	if def.Type == ParamAsOf {
		return "As of " + dayWords(day)
	}
	return dayWords(day)
}

// EmptyText is emptyMessage (params.ts): the view's text for a report with no
// rows.
func (f *exportFormatter) EmptyText(def *ParamDef, resolved *ResolvedParam) string {
	words := f.PeriodWords(def, resolved)
	switch {
	case def == nil || words == "":
		return "No rows"
	case words == allDatesLabel:
		return "No rows for all dates"
	case def.Type == ParamDateRange:
		return "No rows for " + words
	case def.Type == ParamAsOf:
		return "No rows as of " + strings.TrimPrefix(words, "As of ")
	}
	return "No rows on " + words
}

// timeZoneWords (date-time.ts): "Central Time" from the table; UTC for a name
// of UTC and for a zone the server does not know; else the zone's city
// ("Chicago time").
func timeZoneWords(timeZone string) string {
	if utcNames[timeZone] {
		return "UTC"
	}
	if words, ok := exportTables.Zones[timeZone]; ok {
		return words
	}
	if _, err := time.LoadLocation(timeZone); err != nil || timeZone == "Local" {
		return "UTC"
	}
	city := timeZone[strings.LastIndex(timeZone, "/")+1:]
	return strings.ReplaceAll(city, "_", " ") + " time"
}

// ZoneLine is zoneLine (format.ts): "Times in Central Time" when a column is
// a date-time, or a link whose record label is one; else "".
func (f *exportFormatter) ZoneLine(columns []ResultColumn) string {
	for _, c := range columns {
		if c.Type == DateTime || c.LabelType == DateTime {
			return "Times in " + timeZoneWords(f.timeZone)
		}
	}
	return ""
}

// isDateParamType is isDateParam (params.ts).
func isDateParamType(t string) bool {
	return t == ParamDate || t == ParamAsOf || t == ParamDateRange
}

// firstDateParam (params.ts) is the report's period: its first date param,
// or nil.
func firstDateParam(defs []ParamDef) *ParamDef {
	for i := range defs {
		if isDateParamType(defs[i].Type) {
			return &defs[i]
		}
	}
	return nil
}

// resolvedOf is the run's resolved value of a param, or nil.
func resolvedOf(params []ResolvedParam, id string) *ResolvedParam {
	for i := range params {
		if params[i].ID == id {
			return &params[i]
		}
	}
	return nil
}

// namedChoices is NAMED_CHOICES: a chip names up to this many choices.
const namedChoices = 2

// groupLabelKey (params.ts) files a whole group's label among a choice's
// labels.
func groupLabelKey(group string) string { return "group:" + group }

// groupPickText (params.ts): the def's groupLabel with the group's label in
// place of each {group}, or "All <group>".
func groupPickText(def ParamDef, groupLabel string) string {
	text := "All " + GroupLabelToken
	if def.OptionsFrom != nil && def.OptionsFrom.GroupLabel != "" {
		text = def.OptionsFrom.GroupLabel
	}
	return strings.ReplaceAll(text, GroupLabelToken, groupLabel)
}

// choiceValues are a resolved choice's values: its list, or its one value.
func choiceValues(r ResolvedParam) []string {
	if r.List != nil {
		return r.List
	}
	if s, ok := r.Value.(string); ok {
		return []string{s}
	}
	return nil
}

// choiceText is choiceLabels (params.ts): the labels of up to two choices,
// else (or while a label is unknown) the count.
func choiceText(def ParamDef, r ResolvedParam, labels map[string]string) string {
	values := choiceValues(r)
	count := len(values) + len(r.Groups)
	all := make([]string, 0, count)
	known := true
	for _, v := range values {
		label, ok := labels[v]
		if !ok {
			i := slices.IndexFunc(def.Options, func(o Option) bool { return o.Value == v })
			if ok = i >= 0; ok {
				label = def.Options[i].Label
			}
		}
		known = known && ok
		all = append(all, label)
	}
	for _, g := range r.Groups {
		label, ok := labels[groupLabelKey(g)]
		known = known && ok
		all = append(all, groupPickText(def, label))
	}
	if count > namedChoices || !known {
		return fmt.Sprintf("%d selected", count)
	}
	return strings.Join(all, ", ")
}

// amountParamText (amountText in params.ts): the amount with its currency
// code before it ("USD 1,500.00"), the amount alone in any currency.
func amountParamText(v any) (string, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", false
	}
	cents, ok := numberOf(m["amount"])
	if !ok {
		return "", false
	}
	currency, _ := m["currency"].(string)
	amount := formatMoney(cents, "")
	if currency == "" {
		return amount, true
	}
	return currency + " " + amount, true
}

// valueText (params.ts) of a param as the run resolved it: "Any" when unset.
// A date param other than the period names its days as its chip does.
func valueText(def ParamDef, r *ResolvedParam, labels map[string]string) string {
	if r == nil || r.Unset {
		return "Any"
	}
	switch r.Type {
	case ParamChoice:
		return choiceText(def, *r, labels)
	case ParamText:
		return "“" + valueString(r.Value) + "”"
	case ParamNumber:
		if n, ok := numberOf(r.Value); ok {
			return jsNumberText(n)
		}
	case ParamAmount:
		if text, ok := amountParamText(r.Value); ok {
			return text
		}
	case ParamDateRange:
		if r.From != "" && r.To != "" {
			return rangeChipText(r.From, r.To)
		}
		return allDatesLabel
	case ParamDate, ParamAsOf:
		if day, ok := r.Value.(string); ok && isDay(day) {
			return shortDay(day)
		}
	}
	return "Any"
}

// ParamsLine is the header's line of params: "<label>: <value>" (chipText in
// params.ts) for every param but the period, in spec order, joined with
// " · ". labels holds a choice param's shown labels by param id, keyed as
// the client's ChoiceLabels (a value, or group:<value>).
func (f *exportFormatter) ParamsLine(defs []ParamDef, resolved []ResolvedParam, labels map[string]map[string]string) string {
	period := firstDateParam(defs)
	parts := make([]string, 0, len(defs))
	for i, def := range defs {
		if period != nil && &defs[i] == period {
			continue
		}
		text := valueText(def, resolvedOf(resolved, def.ID), labels[def.ID])
		if def.Label != "" {
			text = def.Label + ": " + text
		}
		parts = append(parts, text)
	}
	return strings.Join(parts, " · ")
}

// MadeText is the day and time the file was made, as a date-time cell reads
// in the reader's zone and hour cycle.
func (f *exportFormatter) MadeText(made time.Time) string {
	return f.formatDateTime(made.UTC().Format(instantLayout))
}

// rawCents writes cents as a decimal number with two places, a period and a
// leading minus, no symbol and no grouping; ok is false for an amount too
// large to show.
func rawCents(cents float64) (string, bool) {
	if math.IsNaN(cents) || math.IsInf(cents, 0) || math.Abs(cents) > maxSafeAmount {
		return "", false
	}
	whole := int64(roundHalfAwayFromZero(cents))
	if whole == 0 {
		return "0.00", true
	}
	sign := ""
	if whole < 0 {
		sign, whole = "-", -whole
	}
	return sign + centsDigits(whole, false), true
}

// plainNumber writes a number as a spreadsheet reads it: no grouping, every
// digit it has.
func plainNumber(n float64) string {
	if n == 0 {
		return "0"
	}
	return strconv.FormatFloat(n, 'f', -1, 64)
}

// RawCell is a cell as the CSV writes it ("Values are raw"): money as
// -1234.50 with no symbol, grouping or number format option; a percent and a
// number as plain numbers; a day as YYYY-MM-DD; a date-time as
// YYYY-MM-DD HH:MM on the reader's wall clock; a bool as TRUE or FALSE; a
// bucket, a select and a link as their words. Kind types the value for the
// Excel writer. A text comes back unchanged: the CSV's formula guard is the
// CSV writer's (csvGuard). A nil value is an empty text.
func (f *exportFormatter) RawCell(col ResultColumn, row Row, value any) rawCell {
	t := cellType(col, row)
	if col.LabelCell != "" && t == Relation {
		label := row.Cells[col.LabelCell]
		if isBlankLabel(label) {
			return rawCell{Text: blankLinkText(row, value), Kind: rawText}
		}
		return f.RawCell(linkLabelColumn(col), row, label)
	}
	if value == nil {
		return rawCell{Kind: rawText}
	}
	if n, ok := numberOf(value); ok {
		return f.rawNumberCell(col, t, n)
	}
	if b, ok := value.(bool); ok {
		if b {
			return rawCell{Text: "TRUE", Kind: rawBool}
		}
		return rawCell{Text: "FALSE", Kind: rawBool}
	}
	s, isString := value.(string)
	switch {
	case t == DateTime && isString:
		return f.rawDateTime(s)
	case t == Date && isString:
		day := s
		if len(day) > 10 {
			day = day[:10]
		}
		if isDay(day) {
			return rawCell{Text: day, Kind: rawDate}
		}
	case isBucketType(t):
		return rawCell{Text: bucketWords(t, value), Kind: rawText}
	case t == Select:
		return rawCell{Text: selectLabel(col, value), Kind: rawText}
	}
	return rawCell{Text: valueString(value), Kind: rawText}
}

// rawNumberCell is RawCell of a number.
func (f *exportFormatter) rawNumberCell(col ResultColumn, t FieldType, n float64) rawCell {
	switch {
	case isBucketType(t):
		return rawCell{Text: bucketWords(t, n), Kind: rawText}
	case t == Money:
		if text, ok := rawCents(n); ok {
			return rawCell{Text: text, Kind: rawMoney}
		}
		return rawCell{Text: reportAmountError, Kind: rawText}
	case math.IsNaN(n) || math.IsInf(n, 0):
		return rawCell{Kind: rawText}
	case t == Percent || col.Format == "percent":
		return rawCell{Text: plainNumber(n), Kind: rawPercent}
	}
	return rawCell{Text: plainNumber(n), Kind: rawNumber}
}

// rawDateTime is RawCell of a date-time: the instant on the reader's wall
// clock, a bare day as a day, any other text as it is.
func (f *exportFormatter) rawDateTime(value string) rawCell {
	if isDay(value) {
		return rawCell{Text: value, Kind: rawDate}
	}
	t, ok := instantOf(value)
	if !ok {
		return rawCell{Text: value, Kind: rawText}
	}
	return rawCell{Text: f.wallClock(t).Format("2006-01-02 15:04"), Kind: rawDateTime}
}
