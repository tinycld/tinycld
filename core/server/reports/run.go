package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// Options shape one run without changing the report.
type Options struct {
	// Expand lists the group keys (a group row's groupKey) whose children
	// the result includes. Groups load lazily: a run returns the first
	// level and the children of each expanded group.
	Expand    [][]any `json:"expand,omitempty"`
	ExpandAll bool    `json:"expandAll,omitempty"`
	// Cursor is the previous page's page.next; Limit is the page size of a
	// report without groups.
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	// Today is the reader's day (YYYY-MM-DD) for period presets; the
	// default is today in UTC.
	Today string `json:"today,omitempty"`
	// Display holds the shell's display options (number format, hidden
	// columns). The server does not read them: formatting stays in the
	// client.
	Display json.RawMessage `json:"display,omitempty"`
}

// Request is one run: a loaded spec, the reader, and the run's inputs.
type Request struct {
	Spec    *Spec
	Loaded  *LoadInfo
	Params  map[string]json.RawMessage
	Options Options
	// Auth is the reader. Every collection the report reads is filtered by
	// the reader's list rule.
	Auth *core.Record
	// Headers are the request's headers as a rule's @request.headers sees
	// them.
	Headers map[string]string
}

// Result is the run's answer, in the shell's ReportModel row shape.
type Result struct {
	Columns []ResultColumn `json:"columns"`
	Rows    []Row          `json:"rows"`
	Notices []Notice       `json:"notices"`
	Topics  []string       `json:"topics"`
	Page    *Page          `json:"page"`
	// Params are the report's params as this run resolved them, in spec
	// order, so the shell shows the period the server ran.
	Params []ResolvedParam `json:"params"`
}

type ResultColumn struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Type  FieldType `json:"type"`
	// TotalType is the type of the column's group, subtotal and total
	// cells when it differs from Type (a count of names is a number).
	TotalType FieldType `json:"totalType,omitempty"`
	Aggregate Aggregate `json:"aggregate,omitempty"`
	Format    string    `json:"format,omitempty"`
	Options   []Option  `json:"options,omitempty"`
	// Currency is a money column's fixed currency; CurrencyCell names the
	// cell that holds each row's currency instead.
	Currency      string       `json:"currency,omitempty"`
	CurrencyCell  string       `json:"currencyCell,omitempty"`
	MixedCurrency bool         `json:"mixedCurrency,omitempty"`
	Pivot         *PivotColumn `json:"pivot,omitempty"`
}

// PivotColumn marks a pivot cell column: the cell column it splits, and
// the pivot value it holds.
type PivotColumn struct {
	Cell string `json:"cell"`
	Key  any    `json:"key"`
}

// Row kinds.
const (
	KindDetail   = "detail"
	KindGroup    = "group"
	KindSubtotal = "subtotal"
	KindComputed = "computed"
	KindTotal    = "total"
)

type Row struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Depth int    `json:"depth"`
	Label string `json:"label"`
	// Section is the group row this row belongs to.
	Section string `json:"section,omitempty"`
	// Parent is the row a nested child row hangs under.
	Parent   string `json:"parent,omitempty"`
	GroupKey []any  `json:"groupKey,omitempty"`
	Expanded bool   `json:"expanded,omitempty"`
	// CanExpand says a group row has rows to open (false for a pivot's
	// innermost group), so the client never repeats that rule.
	CanExpand bool           `json:"canExpand,omitempty"`
	Cells     map[string]any `json:"cells"`
}

type Notice struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Columns []string `json:"columns,omitempty"`
	// Count is the number of rows a rowCap or nestCap notice kept.
	Count int `json:"count,omitempty"`
	// LeftOut is the number of pivot values a pivotCap notice left out.
	LeftOut int `json:"leftOut,omitempty"`
	// FromVersion is the spec version an upgraded notice came from.
	FromVersion int     `json:"fromVersion,omitempty"`
	Aliases     []Alias `json:"aliases,omitempty"`
}

type Page struct {
	Next string `json:"next,omitempty"`
	Size int    `json:"size"`
}

var (
	// ErrTimeout is a statement stopped at the time limit.
	ErrTimeout = errors.New("reports: the report took too long to run")
	// ErrAmountTooLarge is a total past the largest integer SQLite holds.
	ErrAmountTooLarge = errors.New("reports: an amount is too large to total")
	// ErrNoReader is a run without a signed-in user: a report never runs
	// as a superuser or as nobody.
	ErrNoReader = errors.New("reports: a report runs as a signed-in user")
)

type limits struct {
	timeout  time.Duration
	rowCap   int
	pageSize int
	maxPage  int
}

// runLimits: each statement stops after timeout; group, pivot and nested
// results stop at rowCap rows; a detail page holds pageSize rows unless
// the run asks for up to maxPage.
var runLimits = limits{timeout: 10 * time.Second, rowCap: 5000, pageSize: 200, maxPage: 1000}

const blankLabel = "(none)"

// runner holds one run's state.
type runner struct {
	ctx    context.Context
	app    core.App
	c      *compiler
	opts   Options
	expand *expansion
	res    *Result
	topics map[string]bool
	// pivotValues are the pivot's columns, from its first pass.
	pivotValues []any
}

// Run runs a report for its reader. The statements read only rows the
// reader's list rules allow; they never write.
func Run(ctx context.Context, app core.App, req Request) (*Result, error) {
	if req.Auth == nil || req.Auth.IsSuperuser() {
		return nil, ErrNoReader
	}
	p, specErr := bind(req.Spec)
	if specErr != nil {
		return nil, specErr
	}
	today, err := todayOf(req.Options.Today)
	if err != nil {
		return nil, err
	}
	params, specErr := resolveParams(p.spec.Params, req.Params, periodContext{today: today, fiscalStart: fiscalStartOf(p.base, app)})
	if specErr != nil {
		return nil, specErr
	}
	c := newCompiler(p, app, listRequestInfo(req.Auth, req.Headers), params)
	expand, err := parseExpansion(req.Options)
	if err != nil {
		return nil, err
	}
	rn := &runner{
		ctx: ctx, app: app, c: c, opts: req.Options, expand: expand,
		res:    &Result{Rows: []Row{}, Notices: []Notice{}},
		topics: map[string]bool{},
	}
	if req.Loaded != nil {
		rn.loadNotices(req.Loaded)
	}
	if err := rn.run(); err != nil {
		return nil, err
	}
	rn.res.Params = resolvedParams(p.spec.Params, params)
	return rn.res, nil
}

func todayOf(s string) (time.Time, error) {
	if s == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	if !isDay(s) {
		return time.Time{}, invalid("options.today", "today is YYYY-MM-DD")
	}
	return time.Parse(time.DateOnly, s)
}

func (rn *runner) loadNotices(info *LoadInfo) {
	if info.UpgradedFrom != 0 {
		rn.notice(Notice{Code: "upgraded", Message: fmt.Sprintf("The report was made with spec version %d and upgraded.", info.UpgradedFrom), FromVersion: info.UpgradedFrom})
	}
	if len(info.Aliased) > 0 {
		rn.notice(Notice{Code: "aliased", Message: "Some fields of the report were renamed; it uses their new names.", Aliases: info.Aliased})
	}
}

func (rn *runner) notice(n Notice) { rn.res.Notices = append(rn.res.Notices, n) }

// noticeOnce adds a notice unless the run already has one of its code: a cap
// says it applied once, however many passes reached it.
func (rn *runner) noticeOnce(n Notice) {
	if !slices.ContainsFunc(rn.res.Notices, func(x Notice) bool { return x.Code == n.Code }) {
		rn.notice(n)
	}
}

// rowCapNotice says the row cap applied.
func rowCapNotice() Notice {
	return Notice{Code: "rowCap", Message: fmt.Sprintf("The report shows its first %d rows.", runLimits.rowCap), Count: runLimits.rowCap}
}

func (rn *runner) run() error {
	// A grouped report is capped, never paged, so options.limit does not
	// apply to it either.
	if len(rn.c.p.groups) > 0 && rn.opts.Cursor != "" {
		return invalid("options.cursor", "a grouped report is not paged")
	}
	groupRows, err := rn.groupPass()
	if err != nil {
		return err
	}
	var details []map[string]any
	if rn.c.p.pivot == nil && (len(rn.c.p.groups) == 0 || len(rn.expand.keys) > 0 || rn.expand.all) {
		details, err = rn.detailPass()
		if err != nil {
			return err
		}
	}
	// Column types come from the spec: a collapsed run compiles the detail
	// select too, so its columns read the same as an expanded run's.
	if err := rn.detailTypes(); err != nil {
		return err
	}
	nested, err := rn.nestPass(details)
	if err != nil {
		return err
	}
	rn.res.Columns = rn.columns()
	rn.markCurrencies(groupRows)
	rn.assemble(groupRows, details, nested)
	rn.res.Topics = make([]string, 0, len(rn.topics))
	for t := range rn.topics {
		rn.res.Topics = append(rn.res.Topics, t)
	}
	slices.Sort(rn.res.Topics)
	return nil
}

// detailTypes fills the columns' detail-row types when the detail pass did
// not run. Nothing is sent to the database.
func (rn *runner) detailTypes() error {
	if rn.c.detailTypes != nil {
		return nil
	}
	r, err := rn.c.rows(newStmt(rn.app, rn.c.info))
	if err != nil {
		return err
	}
	_, _, err = rn.c.detailSelect(r)
	return err
}

// query runs one statement under the time limit and records the topics of
// every collection it reads.
func (rn *runner) query(s *stmt, ctes []string, body string) ([]map[string]any, error) {
	with, err := s.with(ctes...)
	if err != nil {
		return nil, err
	}
	sql := with + body
	ctx, cancel := context.WithTimeout(rn.ctx, runLimits.timeout)
	defer cancel()
	topics, err := changesignal.TopicsRead(ctx, rn.app, sql, s.params)
	if err != nil {
		return nil, mapRunError(err)
	}
	for _, t := range topics {
		rn.topics[t] = true
	}
	rows, err := rn.app.DB().NewQuery(sql).Bind(s.params).WithContext(ctx).Rows()
	if err != nil {
		return nil, mapRunError(err)
	}
	out, err := scanRows(rows)
	if err != nil {
		return nil, mapRunError(err)
	}
	return out, nil
}

func mapRunError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return ErrTimeout
	case strings.Contains(err.Error(), "integer overflow"):
		return ErrAmountTooLarge
	}
	return err
}

// groupPass runs the group statement: every group level, the total and the
// layout's computed rows, capped at rowCap rows.
func (rn *runner) groupPass() ([]map[string]any, error) {
	c := rn.c
	s := newStmt(rn.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	pivot, err := rn.pivotPass()
	if err != nil {
		return nil, err
	}
	// A statement without its computed rows is wrong, not just short: the
	// statement limits the level rows and adds the computed rows after.
	ctes, body, err := c.groupStatement(s, r, rn.expand, pivot, int64(runLimits.rowCap+1))
	if err != nil {
		return nil, err
	}
	rows, err := rn.query(s, append([]string{r.render()}, ctes...), body)
	if err != nil {
		return nil, err
	}
	return rn.capLevelRows(rows), nil
}

// capLevelRows drops the level row past the cap (the statement fetched one
// more to tell) and keeps every computed row, which follow the level rows.
func (rn *runner) capLevelRows(rows []map[string]any) []map[string]any {
	levels := 0
	for _, row := range rows {
		if row["__row"] == nil {
			levels++
		}
	}
	if levels <= runLimits.rowCap {
		return rows
	}
	rn.noticeOnce(rowCapNotice())
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row["__row"] != nil || len(out) < runLimits.rowCap {
			out = append(out, row)
		}
	}
	return out
}

func notASortKey(at string) error {
	return invalid(at, "a running value or a percent is not a sort key")
}

// groupStatement is the UNION ALL of every level's rows (the children of
// expanded groups only) and the layout's computed rows.
func (c *compiler) groupStatement(s *stmt, r *rowsCTE, expand *expansion, pivot []any, limit int64) ([]string, string, error) {
	var ctes, legs []string
	c.levelTypes = map[string]FieldType{}
	for n := 0; n <= len(c.p.groups); n++ {
		lv, err := c.level(r, n)
		if err != nil {
			return nil, "", err
		}
		cells := make([]string, 0, len(c.p.cols))
		for idx, cp := range c.p.cols {
			e, err := lv.cell(cp)
			if err != nil {
				return nil, "", err
			}
			cells = append(cells, fmt.Sprintf("%s AS v%d", e.sql, idx))
			c.levelTypes[cp.col.ID] = e.typ
		}
		pivotCells, err := c.pivotCells(lv, pivot)
		if err != nil {
			return nil, "", err
		}
		cells = append(cells, pivotCells...)
		ctes = append(ctes, lv.render(cells)...)
		leg := fmt.Sprintf("SELECT * FROM o%d", n)
		if n >= 2 {
			leg += " WHERE " + expand.where(c, s, n-1, "")
		}
		legs = append(legs, leg)
	}
	layoutCTEs, computed, err := c.layoutLegs(s, pivot)
	if err != nil {
		return nil, "", err
	}
	ctes = append(ctes, layoutCTEs...)
	// The limit applies to the level rows only: computed rows sort last, so a
	// limit on the whole statement would drop them first.
	levels := fmt.Sprintf("SELECT * FROM (%s\nORDER BY __level, __ord LIMIT %s)", strings.Join(legs, "\nUNION ALL\n"), s.bind(limit))
	all := append([]string{levels}, computed...)
	return ctes, strings.Join(all, "\nUNION ALL\n") + "\nORDER BY __level, __ord", nil
}

// detailPass runs the detail statement: one row per base row, a page at a
// time without groups, or the rows of the expanded innermost groups.
func (rn *runner) detailPass() ([]map[string]any, error) {
	c := rn.c
	s := newStmt(rn.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	sel, order, err := c.detailSelect(r)
	if err != nil {
		return nil, err
	}
	where := "1"
	limit := runLimits.rowCap
	paged := len(c.p.groups) == 0
	if paged {
		limit = runLimits.pageSize
		if rn.opts.Limit > 0 {
			limit = min(rn.opts.Limit, runLimits.maxPage)
		}
		where, err = keysetAfter(s, order, rn.opts.Cursor)
		if err != nil {
			return nil, err
		}
	} else {
		where = rn.expand.where(c, s, len(c.p.groups), "d.")
	}
	body := fmt.Sprintf("SELECT * FROM (%s) d WHERE %s ORDER BY %s LIMIT %s",
		sel, where, orderBy(order), s.bind(int64(limit+1)))
	rows, err := rn.query(s, []string{r.render()}, body)
	if err != nil {
		return nil, err
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	if paged {
		rn.res.Page = &Page{Size: len(rows)}
		if more {
			rn.res.Page.Next = cursorOf(order, rows[len(rows)-1])
		}
	} else if more {
		rn.noticeOnce(rowCapNotice())
	}
	return rows, nil
}

// detailSelect is the select of detail rows over w and their sort keys.
func (c *compiler) detailSelect(r *rowsCTE) (string, []sortKey, error) {
	keys := make([]string, 0)
	for i := range c.p.groups {
		keys = append(keys, fmt.Sprintf("w.k%d", i))
	}
	frame := &windowFrame{
		parent: append(prefixed("w.", c.currencies), keys...),
		total:  prefixed("w.", c.currencies),
	}
	visiting := map[string]bool{}
	var order []sortKey
	sel := []string{"w.__id AS __id"}
	for _, k := range append(keys, prefixed("w.", c.currencies)...) {
		sel = append(sel, k+" AS "+strings.TrimPrefix(k, "w."))
	}
	for i, s := range c.p.sort {
		ctx := &rowsCtx{r: r, visiting: visiting, detail: frame}
		e, err := ctx.col(s.col.col.ID, fmt.Sprintf("sort.%d.col", i))
		if err != nil {
			return "", nil, err
		}
		if e.win {
			return "", nil, notASortKey(fmt.Sprintf("sort.%d.col", i))
		}
		name := fmt.Sprintf("s%d", i)
		sel = append(sel, e.sql+" AS "+name)
		order = append(order, sortKey{name: name, desc: s.desc, col: s.col.col.ID, typ: e.typ})
		frame.order = append(frame.order, "("+e.sql+" IS NULL)", e.sql+dirOf(s.desc))
	}
	frame.order = append(frame.order, "w.__id")
	order = append(order, sortKey{name: "__id"})
	c.detailTypes = map[string]FieldType{}
	for idx, cp := range c.p.cols {
		if cp.nest > 0 {
			sel = append(sel, fmt.Sprintf("NULL AS v%d", idx))
			c.detailTypes[cp.col.ID] = cp.path.field.Type
			continue
		}
		ctx := &rowsCtx{r: r, visiting: visiting, detail: frame}
		e, err := ctx.col(cp.col.ID, cp.at)
		if err != nil {
			return "", nil, err
		}
		sel = append(sel, fmt.Sprintf("%s AS v%d", e.sql, idx))
		c.detailTypes[cp.col.ID] = e.typ
		if cp.path == nil {
			// A formula's amount has the currency of the amounts it reads.
			c.currencyOf[cp.col.ID] = e.cur
		}
	}
	if len(c.p.nest) > 0 {
		sel = append(sel, r.nestParent()+" AS __n0")
	}
	return "SELECT " + strings.Join(sel, ", ") + " FROM w", order, nil
}

// sortKey is one ORDER BY column of the detail select: s<i> for a sort
// column, then __id, which is unique and never NULL.
type sortKey struct {
	name string
	desc bool
	// col and typ name the report column and its type; they fingerprint a
	// page cursor. The id key has neither.
	col string
	typ FieldType
}

// orderBy sorts detail rows with NULLs last in either direction.
func orderBy(keys []sortKey) string {
	parts := make([]string, 0, 2*len(keys))
	for _, k := range keys {
		col := "d." + k.name
		if k.name != "__id" {
			parts = append(parts, "("+col+" IS NULL)")
		}
		parts = append(parts, col+dirOf(k.desc))
	}
	return strings.Join(parts, ", ")
}

func dirOf(desc bool) string {
	if desc {
		return " DESC"
	}
	return ""
}

// expansion is the set of group keys whose children a run includes.
type expansion struct {
	all  bool
	keys [][]any
	// set holds each key as keyString gives it, which keeps the type: the
	// text "1" is not the number 1.
	set map[string]bool
}

// maxExpandValues bounds the values options.expand binds into one statement
// (each key's group values and currency keys), under SQLite's limit of
// 32,766 variables with room for the statement's other params.
const maxExpandValues = 30000

// parseExpansion reads options.expand: at most rowCap keys, since a run
// shows no more group rows than that, and at most maxExpandValues values.
func parseExpansion(opts Options) (*expansion, error) {
	e := &expansion{all: opts.ExpandAll, set: map[string]bool{}}
	if len(opts.Expand) > runLimits.rowCap {
		return nil, invalid("options.expand", fmt.Sprintf("a run expands at most %d groups", runLimits.rowCap))
	}
	values := 0
	for _, key := range opts.Expand {
		values += len(key)
	}
	if values > maxExpandValues {
		return nil, invalid("options.expand", fmt.Sprintf("the expanded group keys hold at most %d values: expand fewer groups, or use expandAll", maxExpandValues))
	}
	for i, key := range opts.Expand {
		if len(key) == 0 {
			return nil, invalid(fmt.Sprintf("options.expand.%d", i), "a group key is a group row's groupKey")
		}
		norm := make([]any, len(key))
		for j, v := range key {
			switch x := v.(type) {
			case nil, string, bool:
				norm[j] = x
			case float64:
				if x == float64(int64(x)) {
					norm[j] = int64(x)
				} else {
					norm[j] = x
				}
			case json.Number:
				norm[j] = numberValue(x)
			default:
				return nil, invalid(fmt.Sprintf("options.expand.%d", i), "a group key holds text, numbers, true, false or null")
			}
		}
		e.keys = append(e.keys, norm)
		e.set[keyString(norm)] = true
	}
	return e, nil
}

// where matches rows whose first depth group keys and currency keys equal
// an expanded group key. The keys are a VALUES list the row is looked up
// in: an OR per key would make SQLite parse one level deeper per key (it
// refuses past 1,000) and plan in time that grows with the square of the
// keys.
func (e *expansion) where(c *compiler, s *stmt, depth int, prefix string) string {
	if e.all {
		return "1"
	}
	cols := make([]string, 0, depth+len(c.currencies))
	for i := range depth {
		cols = append(cols, fmt.Sprintf("%sk%d", prefix, i))
	}
	for _, ck := range c.currencies {
		cols = append(cols, prefix+ck)
	}
	var rows []string
	for _, key := range e.keys {
		if len(key) != len(cols) {
			continue
		}
		values := make([]string, len(key))
		for i, v := range key {
			values[i] = s.bind(v)
		}
		rows = append(rows, "("+strings.Join(values, ", ")+")")
	}
	if len(rows) == 0 {
		return "0"
	}
	match := make([]string, len(cols))
	for i, col := range cols {
		match[i] = fmt.Sprintf("x.column%d IS %s", i+1, col)
	}
	return fmt.Sprintf("EXISTS (SELECT 1 FROM (VALUES %s) x WHERE %s)", strings.Join(rows, ", "), strings.Join(match, " AND "))
}

func (e *expansion) has(key []any) bool {
	if e.all {
		return true
	}
	return e.set[keyString(key)]
}

// compileDry compiles every statement of a plan without a database, so a
// formula, a filter or a layout that cannot compile fails Validate, not a
// run. Params are unset and the pivot has one value.
func compileDry(p *plan) error {
	c := newCompiler(p, nil, nil, nil)
	s := newStmt(nil, nil)
	r, err := c.rows(s)
	if err != nil {
		return err
	}
	if _, _, err := c.detailSelect(r); err != nil {
		return err
	}
	var pivot []any
	if p.pivot != nil {
		// As many cells as Run compiles at the cap, so Validate and Run
		// refuse the same layouts.
		pivot = make([]any, p.pivot.max)
	}
	_, _, err = c.groupStatement(s, r, &expansion{}, pivot, 1)
	return err
}
