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
// over a blank one. A where on another param reads that param's value for
// the request.
func (c *compiler) optionsSelect(s *stmt, op *optionsPlan) (string, error) {
	sc := newScope(s, op.src)
	value := "CAST(" + sc.value(op.value).sql + " AS TEXT)"
	label := "COALESCE(min(NULLIF(CAST(" + optionLabelOf(s, sc.value(op.label)) + " AS TEXT), '')), '')"
	group := "NULL"
	if op.group != nil {
		group = "COALESCE(min(NULLIF(CAST(" + sc.value(op.group).sql + " AS TEXT), '')), '')"
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
	return fmt.Sprintf("SELECT %s AS v, %s AS l, %s AS g\n%s\nWHERE %s AND %s IS NOT NULL\nGROUP BY 1",
		value, label, group, sc.render(), where, value), nil
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

// ParamOption is one option: the value a run takes, its shown label and its
// group's value ("" for no group, which is also the blank group's value).
type ParamOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Group string `json:"group,omitempty"`
}

// OptionGroup is one group of a param's options: the value its options
// carry and its shown label (a record's label, a select's option label, Yes
// or No). A number, amount, percent, date or date-time label also has its
// type, so the client formats it as it formats a cell (R9).
type OptionGroup struct {
	Value string    `json:"value"`
	Label string    `json:"label"`
	Type  FieldType `json:"type,omitempty"`
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
		conds = append(conds, fmt.Sprintf("(o.l > %s OR (o.l = %s AND o.v > %s))", l, l, s.bind(value)))
	}
	body := fmt.Sprintf("SELECT o.v AS v, o.l AS l, o.g AS g FROM (%s) o\nWHERE %s\nORDER BY o.l, o.v LIMIT %s",
		inner, strings.Join(conds, " AND "), s.bind(int64(limit+1)))
	rows, _, err := readRows(ctx, c.app, s, nil, body, false)
	if err != nil {
		return err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	for _, row := range rows {
		res.Options = append(res.Options, ParamOption{Value: textOf(row["v"]), Label: shownText(row["l"]), Group: textOf(row["g"])})
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
	label := "COALESCE(" + optionLabelOf(s, pathExpr("o.g", op.group)) + ", '')"
	// A number orders by its value, not its text ("4" before "190"); the
	// blank group comes first, as a blank text label does.
	order := "2, 1"
	if typ == Number || typ == Money || typ == Percent {
		order = "o.g <> '', CAST(o.g AS REAL), 1"
	}
	body := fmt.Sprintf("SELECT o.g AS g, %s AS gl FROM (%s) o\nWHERE %s\nGROUP BY o.g\nORDER BY %s LIMIT %s",
		label, inner, strings.Join(optionConds(s, search, nil), " AND "), order, s.bind(int64(maxOptionGroups+1)))
	rows, _, err := readRows(ctx, c.app, s, nil, body, false)
	if err != nil {
		return err
	}
	if len(rows) > maxOptionGroups {
		rows, res.GroupsCut = rows[:maxOptionGroups], true
	}
	for _, row := range rows {
		res.Groups = append(res.Groups, groupOf(textOf(row["g"]), textOf(row["gl"]), typ))
	}
	return nil
}

// shownType is the type of what a path shows: a linked record's label
// field's, a select's text, else the field's own.
func shownType(p *pathPlan) FieldType {
	if p.target != nil && p.field.Type == Relation {
		return labelTypeOf(labelFieldOf(p.target, ""))
	}
	return labelTypeOf(p.field)
}

// groupOf is a group shown as a run shows a group label of its type: (none)
// for a blank, Yes or No for a yes/no, a typed text for a number, amount,
// percent or date, which the client formats.
func groupOf(value, label string, typ FieldType) OptionGroup {
	g := OptionGroup{Value: value, Label: shownText(label)}
	switch {
	case label == "" || label == noName:
	case typ == Bool:
		g.Label = "No"
		if label != "0" && label != "false" {
			g.Label = "Yes"
		}
	case typ == Number || typ == Money || typ == Percent || typ == Date || typ == DateTime:
		g.Type = typ
	}
	return g
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
