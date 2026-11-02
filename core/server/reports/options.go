package reports

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/core"
)

// optionsSelect selects a choice param's options: the rows of its source
// the reader may list that pass its where, one row per value (v) with its
// label (l) and group (g) as text. The label is already shown (R31); the
// group is its stored value, an empty text for a blank one, so options with
// no group value make one blank group. A link to a record the reader cannot
// see reads blank (the hop joins through the target's rule CTE), so it joins
// that blank group. Where a value's rows differ, a set label or group wins
// over a blank one. A yes/no label reads Yes or No, so a search and the
// order read what the picker shows; an amount label's currency is lc. An
// amount group with a currency is "<amount> <currency>" (amountGroupSQL). A
// where on another param reads that param's value for the request.
func (c *compiler) optionsSelect(s *stmt, op *optionsPlan) (string, error) {
	sc := newScope(s, op.src)
	value := "CAST(" + sc.value(op.value).sql + " AS TEXT)"
	lv := sc.value(op.label)
	label := optionLabelOf(s, lv)
	if shownType(op.label) == Bool {
		label = fmt.Sprintf("(CASE WHEN %[1]s IS NULL OR %[1]s = %[2]s THEN %[1]s WHEN %[1]s THEN 'Yes' ELSE 'No' END)", label, s.bind(noName))
	}
	label = "COALESCE(min(NULLIF(CAST(" + label + " AS TEXT), '')), '')"
	labelCur := "min(" + c.shownCurrencySQL(s, sc, op.label, lv) + ")"
	group := "NULL"
	if op.group != nil {
		group = "COALESCE(min(NULLIF(" + c.amountGroupSQL(s, sc, op.group) + ", '')), '')"
	}
	where := "1"
	if w := op.def.OptionsFrom.Where; w != nil {
		// A rows CTE with no root: a where reads its source's paths, never a
		// report column (joinCtx.col refuses outside the root scope).
		r := &rowsCTE{c: c, s: s}
		e, err := compileWhere(&joinCtx{r: r, sc: sc, src: op.src, isFilter: true, visiting: map[string]bool{}}, w, op.at)
		if err != nil {
			return "", err
		}
		if !e.unset {
			where = "COALESCE(" + e.sql + ", 0)"
		}
	}
	return fmt.Sprintf("SELECT %s AS v, %s AS l, %s AS lc, %s AS g\n%s\nWHERE %s AND %s IS NOT NULL\nGROUP BY 1",
		value, label, labelCur, group, sc.render(), where, value), nil
}

// groupListSQL is a choice list with group references expanded under the
// reader's rules: the values of the options select whose group is one of
// groups, plus the explicit values. A group compares by the text the options
// select gives it, the value the picker listed (an amount group's names its
// currency too), so a group the reader cannot list matches nothing. The
// explicit values are one VALUES term, which SQLite does not count against
// its 500 terms of a compound select (a UNION per value would). json_each
// would bind them once, but it is a virtual table, which the change-signal
// topic check refuses to map.
func (c *compiler) groupListSQL(s *stmt, op *optionsPlan, values, groups []string) (string, error) {
	inner, err := c.optionsSelect(s, op)
	if err != nil {
		return "", err
	}
	marks := make([]string, len(groups))
	for i, g := range groups {
		marks[i] = s.bind(g)
	}
	sql := fmt.Sprintf("SELECT o.v FROM (%s) o WHERE o.g IN (%s)", inner, strings.Join(marks, ", "))
	if len(values) > 0 {
		rows := make([]string, len(values))
		for i, v := range values {
			rows[i] = "(" + s.bind(v) + ")"
		}
		sql += " UNION SELECT column1 FROM (VALUES " + strings.Join(rows, ", ") + ")"
	}
	return "(" + sql + ")", nil
}

// paramLeaf is a param as a formula leaf in a context with a statement: a
// choice with group references reads the expanded list. Options whose where
// reads a param being expanded would expand themselves, so they are refused.
func paramLeaf(c *compiler, s *stmt, f fctx, id string, v paramValue) (expr, error) {
	if len(v.groups) == 0 {
		return paramExpr(f, v), nil
	}
	op := c.p.options[id]
	if op == nil {
		return expr{}, fmt.Errorf("reports: param %q holds groups but has no options source", id)
	}
	if c.expanding[id] {
		return expr{}, invalid(op.at+".where", fmt.Sprintf("the options of %q read a group picked from them", id))
	}
	c.expanding[id] = true
	defer delete(c.expanding, id)
	sql, err := c.groupListSQL(s, op, v.list, v.groups)
	if err != nil {
		return expr{}, err
	}
	return expr{sql: sql, typ: Text, list: true}, nil
}

// amountGroupSQL is a group's value as text. An amount with a currency is
// "<amount> <currency>" (one space; a currency code has none), so 300 USD
// and 300 EUR are two groups and a posted group value (a group filter, a
// group reference) names both; an amount whose currency is blank keeps the
// amount alone. Any other group is its stored value.
func (c *compiler) amountGroupSQL(s *stmt, sc *scope, p *pathPlan) string {
	text := "CAST(" + sc.value(p).sql + " AS TEXT)"
	if p.field.Type != Money {
		return text
	}
	cur := c.shownCurrencySQL(s, sc, p, sc.value(p))
	if cur == "NULL" {
		return text
	}
	return text + " || COALESCE(' ' || NULLIF(" + cur + ", ''), '')"
}

// shownCurrencySQL is the currency of what a path shows when that is an
// amount: a linked record's label's, the amount's currency field read
// beside it, or its source's fixed currency; NULL for anything else.
func (c *compiler) shownCurrencySQL(s *stmt, sc *scope, p *pathPlan, e expr) string {
	if e.target != nil && p.field.Type == Relation {
		return labelCurrencySQL(s, e.target, labelFieldOf(e.target, ""), e.sql)
	}
	sql, fixed := amountCurrencyOf(sc, p.hops, p)
	switch {
	case sql != "":
		return sql
	case fixed != nil && c.app != nil:
		return s.bind(fixed.Currency(c.app))
	}
	return "NULL"
}

// splitAmountGroup reads an amount group's value: its amount and currency.
func splitAmountGroup(value string) (string, string) {
	if amount, cur, ok := strings.Cut(value, " "); ok {
		return amount, cur
	}
	return value, ""
}

// optionLabelOf is an option's label as the picker shows it (R31): a value
// that points at a record by that record's label, through its rule CTE, so a
// record the reader cannot see has none; a select by its option's label.
func optionLabelOf(s *stmt, e expr) string {
	if e.target != nil {
		return labelValueSQL(s, e.target, labelFieldOf(e.target, ""), e.sql)
	}
	return asShown(s.bind, e).sql
}

// Param-options limits (R12): a page holds optionsPage options unless the
// request asks for up to maxChoiceValues; a search has at most
// maxOptionSearch characters; a list names at most maxOptionGroups groups.
const (
	optionsPage     = 50
	maxOptionSearch = 200
	maxOptionGroups = 200
)

// OptionsRequest asks for one page of a choice param's options.
type OptionsRequest struct {
	Spec  *Spec
	Param string
	// Search keeps the options whose label holds it (any case). Group
	// keeps one group's options, by the group's value. After is the
	// previous page's next.
	Search string
	Group  *string
	After  string
	// Values asks for the labels of these values instead of a page: the
	// options among them the reader may list, with no paging. It is not
	// combined with a search, a group or after.
	Values []string
	Limit  int
	// Params are the report's other params, for a where that reads them;
	// Today is the reader's day (YYYY-MM-DD) for their period presets.
	Params map[string]json.RawMessage
	Today  string
	// Auth is the reader; Headers the request's headers as a rule's
	// @request.headers sees them.
	Auth    *core.Record
	Headers map[string]string
}

// ParamOption is one option: the value a run takes, its shown label (a
// record's label, a select's option label, Yes or No) and its group's value
// ("" for no group, which is also the blank group's value). A number,
// amount, percent, date or date-time label also has its type, and an amount
// its currency, so the client formats it as it formats a cell (R9).
type ParamOption struct {
	Value    string    `json:"value"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type,omitempty"`
	Currency string    `json:"currency,omitempty"`
	Group    string    `json:"group,omitempty"`
}

// OptionGroup is one group of a param's options: the value its options
// carry and its shown label, typed as an option's label is. The value of a
// group read from an amount with a currency is "<amount> <currency>" (one
// space, e.g. "300 EUR"), so 300 USD and 300 EUR are two groups; a group
// filter and a group reference (R30) post that value as it is.
type OptionGroup struct {
	Value    string    `json:"value"`
	Label    string    `json:"label"`
	Type     FieldType `json:"type,omitempty"`
	Currency string    `json:"currency,omitempty"`
}

// ParamOptions is one page of options, ordered by label then value. Groups
// are the groups of every option that matches the search, at most
// maxOptionGroups of them (GroupsCut says the list stops there); they come
// with the first page only (F9). Next is the cursor of the next page, "" on
// the last.
type ParamOptions struct {
	Options   []ParamOption `json:"options"`
	Groups    []OptionGroup `json:"groups"`
	GroupsCut bool          `json:"groupsCut,omitempty"`
	Next      string        `json:"next,omitempty"`
}

// ReadParamOptions reads one page of a choice param's options for its
// reader, under the reader's list rules, as a run of the same spec and
// params would read them. It writes nothing.
func ReadParamOptions(ctx context.Context, app core.App, req OptionsRequest) (*ParamOptions, error) {
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Today, "today")
	if err != nil {
		return nil, err
	}
	op := c.p.options[req.Param]
	if op == nil {
		return nil, invalid("param", fmt.Sprintf("the report has no choice param %q whose options come from a source", req.Param))
	}
	if req.Group != nil && op.group == nil {
		return nil, invalid("group", fmt.Sprintf("the options of %q have no groups", req.Param))
	}
	limit, err := optionsLimit(req)
	if err != nil {
		return nil, err
	}
	res := &ParamOptions{Options: []ParamOption{}, Groups: []OptionGroup{}}
	if err := c.optionsPage(ctx, op, req, limit, res); err != nil {
		return nil, err
	}
	if op.group != nil && len(req.Values) == 0 && req.After == "" {
		if err := c.optionGroups(ctx, op, req.Search, res); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// optionsLimit checks a request's bounds and returns its page size: every
// asked value when it labels values.
func optionsLimit(req OptionsRequest) (int, error) {
	if utf8.RuneCountInString(req.Search) > maxOptionSearch {
		return 0, invalid("search", fmt.Sprintf("a search has at most %d characters", maxOptionSearch))
	}
	if len(req.Values) > 0 {
		switch {
		case len(req.Values) > maxChoiceValues:
			return 0, invalid("values", fmt.Sprintf("a request labels at most %d values", maxChoiceValues))
		case req.Search != "" || req.Group != nil || req.After != "" || req.Limit != 0:
			return 0, invalid("values", "values are labelled alone, with no search, group, after or limit")
		}
		return len(req.Values), nil
	}
	switch {
	case req.Limit == 0:
		return optionsPage, nil
	case req.Limit < 1 || req.Limit > maxChoiceValues:
		return 0, invalid("limit", fmt.Sprintf("limit is 1 to %d", maxChoiceValues))
	}
	return req.Limit, nil
}

// optionsPage reads the options of one page (or of the asked values) into
// res, with the next page's cursor when there is one.
func (c *compiler) optionsPage(ctx context.Context, op *optionsPlan, req OptionsRequest, limit int, res *ParamOptions) error {
	s := newStmt(c.app, c.info)
	inner, err := c.optionsSelect(s, op)
	if err != nil {
		return err
	}
	typ := shownType(op.label)
	conds := optionConds(s, req.Search, req.Group)
	switch {
	case len(req.Values) > 0:
		marks := make([]string, len(req.Values))
		for i, v := range req.Values {
			marks[i] = s.bind(v)
		}
		conds = append(conds, "o.v IN ("+strings.Join(marks, ", ")+")")
	case req.After != "":
		label, value, err := readOptionsCursor(req.After)
		if err != nil {
			return err
		}
		l := s.bind(label)
		conds = append(conds, fmt.Sprintf("(o.l <> '', %s, o.v) > (%s <> '', %s, %s)", orderKey(typ, "o.l"), l, orderKey(typ, l), s.bind(value)))
	}
	body := fmt.Sprintf("SELECT o.v AS v, o.l AS l, o.lc AS lc, o.g AS g FROM (%s) o\nWHERE %s\nORDER BY o.l <> '', %s, o.v LIMIT %s",
		inner, strings.Join(conds, " AND "), orderKey(typ, "o.l"), s.bind(int64(limit+1)))
	rows, _, err := readRows(ctx, c.app, s, nil, body, false)
	if err != nil {
		return err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, row := range rows {
		label, t, cur := pickerLabel(textOf(row["l"]), textOf(row["lc"]), typ)
		res.Options = append(res.Options, ParamOption{Value: textOf(row["v"]), Label: label, Type: t, Currency: cur, Group: textOf(row["g"])})
	}
	if more {
		last := rows[len(rows)-1]
		res.Next = optionsCursor(textOf(last["l"]), textOf(last["v"]))
	}
	return nil
}

// optionGroups reads into res the groups of the options that match search,
// each shown by its label: a group read from a link shows the record's
// label, never its id, and a select's its option's label.
func (c *compiler) optionGroups(ctx context.Context, op *optionsPlan, search string, res *ParamOptions) error {
	s := newStmt(c.app, c.info)
	inner, err := c.optionsSelect(s, op)
	if err != nil {
		return err
	}
	typ := shownType(op.group)
	key := pathExpr("o.g", op.group)
	label, cur := "COALESCE("+optionLabelOf(s, key)+", '')", "NULL"
	if key.target != nil {
		// A link's group shows the record's label, and its currency when the
		// label is an amount; an amount's own group carries its currency in
		// its value (amountGroupSQL).
		cur = labelCurrencySQL(s, key.target, labelFieldOf(key.target, ""), "o.g")
	}
	// The blank group comes first; a number orders by its value, not its
	// text ("4" before "190").
	body := fmt.Sprintf("SELECT o.g AS g, %s AS gl, %s AS glc FROM (%s) o\nWHERE %s\nGROUP BY o.g\nORDER BY o.g <> '', %s, 1 LIMIT %s",
		label, cur, inner, strings.Join(optionConds(s, search, nil), " AND "), orderKey(typ, "gl"), s.bind(int64(maxOptionGroups+1)))
	rows, _, err := readRows(ctx, c.app, s, nil, body, false)
	if err != nil {
		return err
	}
	if len(rows) > maxOptionGroups {
		rows, res.GroupsCut = rows[:maxOptionGroups], true
	}
	for _, row := range rows {
		text, groupCur := textOf(row["gl"]), textOf(row["glc"])
		if key.target == nil && typ == Money {
			text, groupCur = splitAmountGroup(text)
		}
		if typ == Bool {
			text = yesNo(text)
		}
		label, t, cur := pickerLabel(text, groupCur, typ)
		res.Groups = append(res.Groups, OptionGroup{Value: textOf(row["g"]), Label: label, Type: t, Currency: cur})
	}
	return nil
}

// orderKey is a label's sort key: a number's value, so "4" sorts before
// "190"; any other label's text.
func orderKey(typ FieldType, label string) string {
	if typ == Number || typ == Money || typ == Percent {
		return "CAST(" + label + " AS REAL)"
	}
	return label
}

// shownType is the type of what a path shows: a linked record's label
// field's, a select's text, else the field's own.
func shownType(p *pathPlan) FieldType {
	if p.target != nil && p.field.Type == Relation {
		return labelTypeOf(labelFieldOf(p.target, ""))
	}
	return labelTypeOf(p.field)
}

// yesNo is a stored yes/no as the picker shows it.
func yesNo(text string) string {
	switch text {
	case "", noName:
		return text
	case "0", "false":
		return "No"
	}
	return "Yes"
}

// pickerLabel is a label of type typ as the picker shows it, as a run shows
// a group label of that type: (none) for a blank, and a number, amount,
// percent, date or date-time typed (an amount with its currency), which the
// client formats. A yes/no label already reads Yes or No.
func pickerLabel(text, cur string, typ FieldType) (string, FieldType, string) {
	switch {
	case text == "" || text == noName:
		return shownText(text), "", ""
	case typ == Money:
		return text, typ, cur
	case typ == Number || typ == Percent || typ == Date || typ == DateTime:
		return text, typ, ""
	}
	return text, "", ""
}

// optionConds are the search and group conditions on the options select (o).
func optionConds(s *stmt, search string, group *string) []string {
	conds := []string{"1"}
	if search != "" {
		conds = append(conds, "instr(tc_fold(o.l), tc_fold("+s.bind(search)+")) > 0")
	}
	if group != nil {
		conds = append(conds, "o.g IS "+s.bind(*group))
	}
	return conds
}

// shownText is a label as the picker shows it: a blank one reads (none).
// The label is already shown (a record's label, a select's option label).
func shownText(v any) string {
	if text := textOf(v); text != "" {
		return text
	}
	return blankLabel
}

func textOf(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

// optionsCursor is the cursor after the option with this label and value.
func optionsCursor(label, value string) string {
	b, _ := json.Marshal([]string{label, value})
	return base64.RawURLEncoding.EncodeToString(b)
}

func readOptionsCursor(text string) (string, string, error) {
	bad := invalid("after", "the cursor is not one this list gave")
	raw, err := base64.RawURLEncoding.DecodeString(text)
	if err != nil {
		return "", "", bad
	}
	var parts []string
	if json.Unmarshal(raw, &parts) != nil || len(parts) != 2 {
		return "", "", bad
	}
	return parts[0], parts[1], nil
}

// paramOptionsBody is one param-options request: a draft spec, or the id of
// a saved report the caller can view, and what to read. Never SQL.
type paramOptionsBody struct {
	Spec     json.RawMessage            `json:"spec"`
	ReportID string                     `json:"reportId"`
	Param    string                     `json:"param"`
	Search   string                     `json:"search"`
	Group    *string                    `json:"group"`
	After    string                     `json:"after"`
	Values   []string                   `json:"values"`
	Limit    int                        `json:"limit"`
	Params   map[string]json.RawMessage `json:"params"`
	Today    string                     `json:"today"`
}

// handleParamOptions answers POST /api/reports/param-options: one page of a
// choice param's options, for a draft spec or a saved report the caller can
// view (read through its view rule, as a run reads it). It writes nothing.
func handleParamOptions(re *core.RequestEvent) error {
	var body paramOptionsBody
	if err := decodeStrict(re, &body, "a param-options request"); err != nil {
		return err
	}
	raw, err := requestSpec(re, body.Spec, body.ReportID)
	if err != nil {
		return err
	}
	spec, _, err := Load(raw)
	if err != nil {
		return runError(re, err)
	}
	res, err := ReadParamOptions(re.Request.Context(), re.App, OptionsRequest{
		Spec: spec, Param: body.Param, Search: body.Search, Group: body.Group, After: body.After,
		Values: body.Values, Limit: body.Limit, Params: body.Params, Today: body.Today,
		Auth: re.Auth, Headers: ruleHeaders(re.Request.Header),
	})
	if err != nil {
		return runError(re, err)
	}
	return re.JSON(http.StatusOK, res)
}
