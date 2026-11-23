package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
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
	// Collapse lists groups the reader closed under a level that is open by
	// default or under ExpandAll. Sent (even empty), it keeps the open levels
	// open beside Expand, so a fold in a report with many open groups never
	// lists them all (J1b, Ruling R2).
	Collapse [][]any `json:"collapse,omitempty"`
	// Cursor is the previous page's page.next; Limit is the page size (at
	// most maxPage). A grouped report without pivot, layout or nest pages its
	// group, opening, detail and subtotal rows in document order (J1a); a
	// pivot, a layout and a nest read their group rows whole.
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
	// Sort orders the detail rows before the report's own sort keys: the
	// viewer's heading sort.
	Sort []ViewSort `json:"sort,omitempty"`
	// Today is the reader's day (YYYY-MM-DD) for period presets; the
	// default is today in UTC.
	Today string `json:"today,omitempty"`
	// TimeZone is the reader's IANA zone (Intl's resolvedOptions().timeZone).
	// A date range, a date or asOf param, a range's bound and a day typed in
	// a condition compared with a date-time compare the instants of the
	// reader's local days; a missing or
	// unknown zone, a name of UTC itself (utcNames, zone.go) and "Local" are
	// UTC (readerZone). Buckets and date parts keep the UTC day (Q7).
	TimeZone string `json:"timeZone,omitempty"`
	// Display holds the shell's display options (number format, hidden
	// columns). The server does not read them: formatting stays in the
	// client.
	Display json.RawMessage `json:"display,omitempty"`
}

// ViewSort is one key of a run's viewer sort: a column, and "asc" (the
// default) or "desc".
type ViewSort struct {
	Col string `json:"col"`
	Dir string `json:"dir,omitempty"`
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
	// RowCap is the most rows a pivot, nest or layout run reads before it
	// stops with a rowCap or nestCap notice.
	// 0: the default cap. Only the export read sets it; the run endpoint never does.
	RowCap int
	// PageCap is the most rows a page holds, in place of maxPage: a caller
	// that needs every row reads them in one page, as each page of a grouped
	// run computes every group level again. 0: maxPage. Only the export read
	// sets it; the run endpoint never does.
	PageCap int
	// Live keeps the run's data topics though it reads a source that is
	// NotLive. Only the run endpoint sets it, for a shipped report that is
	// registered Live.
	Live bool
	// refuseOver, when set, stops a paged grouped run before its detail
	// statement with errPastRows once the rows it certainly shows are more
	// than refuseOver (pastRefusal). Only the export read sets it.
	refuseOver int
}

// Result is the run's answer, in the shell's ReportModel row shape.
type Result struct {
	Columns []ResultColumn `json:"columns"`
	Rows    []Row          `json:"rows"`
	Notices []Notice       `json:"notices"`
	Topics  []string       `json:"topics"`
	// Manual is set when the run reads a source that is NotLive and the
	// request is not Live: Topics is then empty, and the reader runs the
	// report again by hand.
	Manual bool `json:"manual,omitzero"`
	// Heading is what the report prints above its title: the base source's
	// Heading, or "" for the organization's name.
	Heading string `json:"heading,omitempty"`
	Page    *Page  `json:"page"`
	// Params are the report's params as this run resolved them, in spec
	// order, so the shell shows the period the server ran.
	Params []ResolvedParam `json:"params"`
	// HeaderLines are the lines the report prints under its period: one per
	// param with a header (ParamDef.Header), in spec order, never nil.
	HeaderLines []string `json:"headerLines"`
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
	// Scale is the decimal places a percent part's column (percentOfTotal,
	// percentOfParent, percentOfSection) prints, 0 included; no value means
	// a percent prints 1 place.
	Scale   *int     `json:"scale,omitempty"`
	Options []Option `json:"options,omitempty"`
	// Currency is a money column's fixed currency; CurrencyCell names the
	// cell that holds each row's currency instead.
	Currency      string       `json:"currency,omitempty"`
	CurrencyCell  string       `json:"currencyCell,omitempty"`
	MixedCurrency bool         `json:"mixedCurrency,omitempty"`
	Pivot         *PivotColumn `json:"pivot,omitempty"`
	// LabelCell names the cell that holds the label of the record a link
	// column's value points at (<id>@label), so no cell shows a raw id. The
	// label is its label field's stored value, of type LabelType (a select's
	// option label and a source's name are text), formatted as a cell of
	// that type; an amount's currency is in LabelCurrencyCell.
	LabelCell         string    `json:"labelCell,omitempty"`
	LabelType         FieldType `json:"labelType,omitempty"`
	LabelCurrencyCell string    `json:"labelCurrencyCell,omitempty"`
	// Sortable says a run accepts the column as a viewer sort key
	// (options.sort): not a running value or a percent, not a child row
	// column, not in a pivot report; the first column by its detail label
	// when the report has one.
	Sortable bool `json:"sortable,omitempty"`
	// BlankAbove says the column has no cell above a group level: its own
	// blankAbove, or that of a column it reads. The rows that show it can all
	// be closed when a run lands, so the client keeps room for its type's
	// worst case (column-measure.ts widestCell).
	BlankAbove bool `json:"blankAbove,omitempty"`
	// Heading is the value the column's heading shows (Column.Heading), absent
	// when the column has none or it is blank this run.
	Heading *ColumnHeading `json:"heading,omitempty"`
	// Drill is the report the column's figures open (Column.Drill), absent
	// when the column has none or the reader may not view it; the rows'
	// <id>@drill cells then are absent too.
	Drill *DrillTarget `json:"drill,omitempty"`
	// GroupOnly: a column that is blank on detail rows (Column.Detail) and whose
	// group cell is not a figure (not a number, money or percent). It names the
	// group, so its cell shows on the group row, also when the group's subtotal
	// row follows, and the subtotal row does not carry it.
	GroupOnly bool `json:"groupOnly,omitempty"`
}

// DrillTarget is the report a drilled column opens, as the reader sees it.
type DrillTarget struct {
	ReportID string `json:"reportId"`
	Title    string `json:"title"`
}

// ColumnHeading is a column's heading this run: a text, a day or a number.
type ColumnHeading struct {
	Value any       `json:"value"`
	Type  FieldType `json:"type"`
}

// PivotColumn marks a pivot cell column: the cell column it splits, and
// the pivot value it holds.
type PivotColumn struct {
	Cell string `json:"cell"`
	Key  any    `json:"key"`
	// LabelType, LabelValue and LabelCurrency type the column label as Row's
	// do: a pivot over a link's record label, an amount, or a bucket (Month,
	// Quarter, Year, FiscalYear, Day with the raw key), which the column's
	// Label holds as plain text.
	LabelType     FieldType `json:"labelType,omitempty"`
	LabelValue    any       `json:"labelValue,omitempty"`
	LabelCurrency string    `json:"labelCurrency,omitempty"`
}

// Row kinds.
const (
	KindDetail   = "detail"
	KindGroup    = "group"
	KindOpening  = "opening"
	KindSubtotal = "subtotal"
	KindComputed = "computed"
	KindTotal    = "total"
)

type Row struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Depth int    `json:"depth"`
	// Level is a group or subtotal row's group level (1 for the first
	// group). With a skipped level (Group.Blank) or an inlined group
	// (Group.Inline) depth + 1 is no longer the level, and a client that
	// reads a row's group from its depth would read the wrong one. A child
	// row of an inlined group has its depth and section, and its groupKey
	// still holds the inlined group's key. 0 (absent) on every other row.
	Level int    `json:"level,omitempty"`
	Label string `json:"label"`
	// LabelColumn names the column whose cell the label repeats, so the
	// client can leave that cell out; "" when the label is no cell's value.
	LabelColumn string `json:"labelColumn,omitempty"`
	// Section is the group row this row belongs to.
	Section string `json:"section,omitempty"`
	// Parent is the row a nested child row hangs under.
	Parent string `json:"parent,omitempty"`
	// Open is the app path a detail row opens (Detail.Open), under the
	// report's package; absent when the report has no open or the row's value
	// is not such a path. Only detail rows have one.
	Open     string `json:"open,omitempty"`
	GroupKey []any  `json:"groupKey,omitempty"`
	Expanded bool   `json:"expanded,omitempty"`
	// CanExpand says a group row has rows to open (false for a pivot's
	// innermost group), so the client never repeats that rule.
	CanExpand bool `json:"canExpand,omitempty"`
	// Muted marks a group row whose level's Group.Muted condition holds; a
	// view draws its label and figures in muted text. Never on another kind.
	Muted bool           `json:"muted,omitempty"`
	Cells map[string]any `json:"cells"`
	// LabelType is set when a group row's label (and its subtotal's), or a
	// detail row's record label or label formula, is a value that is not
	// text: an amount, a number, a percent, a date or a yes/no. LabelValue is that value as a cell of the type holds it (an
	// amount in integer cents, a yes/no as true or false) and LabelCurrency
	// is an amount's currency, so the client formats the label as it formats
	// a cell; Label is its plain text (a subtotal's is "Total " + it).
	LabelType     FieldType `json:"labelType,omitempty"`
	LabelValue    any       `json:"labelValue,omitempty"`
	LabelCurrency string    `json:"labelCurrency,omitempty"`
	// LabelTemplate is set on an opening row whose label names a value of the
	// run (Opening.LabelValue): the label with LabelValueToken where the value
	// goes. LabelType and LabelValue then hold the value, which a view
	// formats and puts in the token's place; Label has it as plain text.
	LabelTemplate string `json:"labelTemplate,omitempty"`
}

type Notice struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	// Parts are the sentences of a message that has more than one, in its
	// order (one per blank column, condition or sort key it names), so the
	// pages of a run that share a sentence show it once; nil for a message
	// of one sentence.
	Parts   []string `json:"parts,omitempty"`
	Columns []string `json:"columns,omitempty"`
	// Conditions names, by spec path, each condition (the filter, a having,
	// a fill's where) that read a looked-up total which mixes currencies.
	Conditions []string `json:"conditions,omitempty"`
	// Count is the number of rows a rowCap or nestCap notice kept.
	Count int `json:"count,omitempty"`
	// LeftOut is the number of pivot values a pivotCap notice left out.
	LeftOut int     `json:"leftOut,omitempty"`
	Aliases []Alias `json:"aliases,omitempty"`
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
	// errPastRows is a run stopped because it certainly shows more rows than
	// its Request.refuseOver.
	errPastRows = errors.New("reports: the report has more rows than the read takes")
)

type limits struct {
	timeout time.Duration
	rowCap  int
	// foldCap is the most group keys options.expand or options.collapse lists.
	foldCap  int
	pageSize int
	maxPage  int
}

// runLimits: each statement stops after timeout; pivot, nest and layout
// results stop at rowCap rows (a grouped report pages its group rows
// instead); a detail page holds pageSize rows unless the run asks for up to
// maxPage.
var runLimits = limits{timeout: 10 * time.Second, rowCap: 5000, foldCap: 5000, pageSize: 200, maxPage: 1000}

const blankLabel = "(none)"

// runner holds one run's state.
type runner struct {
	ctx context.Context
	app core.App
	// db is the run's read transaction (beginRead): every statement of the
	// run reads through it.
	db     dbx.Builder
	c      *compiler
	opts   Options
	expand *expansion
	res    *Result
	topics map[string]bool
	// askedRowCap is the request's RowCap (0: runLimits.rowCap); read it
	// through rowCap.
	askedRowCap int
	// askedPageCap is the request's PageCap (0: runLimits.maxPage); read it
	// through pageLimit.
	askedPageCap int
	// live is the request's Live.
	live bool
	// refuseOver is the request's refuseOver.
	refuseOver int
	// openDropped is set once a detail row's open value was dropped, so a run
	// logs a bad open path once, not once per row.
	openDropped bool
	// drillTargets is each drilled column's target the reader may view, by
	// column id; a column the reader may not open is absent.
	drillTargets map[string]*DrillTarget
	// sectionBases holds each percentOfSection column's section total of its
	// argument per currency block, by column id + "|" + currency key
	// (readSectionBases).
	sectionBases map[string]any
	// assembleErr is the first error assembling the rows met (a percent that
	// overflows); run returns it.
	assembleErr error
	// ruleTopics holds the topics of each rule CTE select the run read, so
	// its statements explain each rule once.
	ruleTopics map[string][]string
	// ruleAnswers holds the answer of each rule probe the run asked
	// (askRules), so its statements ask each rule once.
	ruleAnswers map[string]bool
	// pivotValues are the pivot's columns, from its first pass, and
	// pivotLabels their records' labels for a pivot over a link.
	pivotValues []pivotValue
	pivotLabels []rowLabel
	// page is the page of a grouped run whose group rows do not page (a
	// layout or a nest): where its rows start and end in the document
	// (pageWindow); nil when the run did not page grouped details.
	page *groupedPage
	// mixedRows holds, by row id, the columns whose cell on that row is
	// blank because a sum lookup met two currencies (push).
	mixedRows map[string][]string
	// mixedConditions are the conditions that read such a sum on some row
	// (probeConditions).
	mixedConditions []string
	// headings are the run's column headings by column id (readHeadings).
	headings map[string]*ColumnHeading
	// openingLabels are the run's opening label values by group level
	// (readHeadings); a level whose value is blank on this run has none.
	openingLabels map[int]*ColumnHeading
	// A run that pages its group rows (pagedGroups) reads its cursor into
	// groupCur, with each level's order terms (groupShape), the detail sort
	// keys (groupKeys) and the fingerprint its cursors carry (groupFP).
	// rawGroups and rawDetails hold the statement rows the page's group and
	// detail rows came from, by row id, so the next cursor reads their order
	// values.
	groupCur   *groupCursor
	groupShape [][]orderTerm
	groupKeys  []sortKey
	groupFP    string
	rawGroups  map[string]map[string]any
	rawDetails map[string]map[string]any
	// anchored is how many of the cursor's anchors the page read again: those
	// from the top that still have the cursor's order values (groupKeyset).
	anchored int
	// shown are the groups whose first details earlier pages showed, up to
	// which detail row (shownBefore, readShownNow).
	shown []held
	// reachCut is true when the page's detail statement may leave out the
	// rows of the open groups its page cannot reach (leafRows): its select
	// holds no percent of the whole report, which reads the rows of every
	// group, and probes no condition, which is probed over every open group of
	// the page. It is not compiler.leafCut, which cuts w to the open groups.
	reachCut bool
	// readDetails is set when the detail pass ran: such a run always had a
	// page, even one that fits.
	readDetails bool
	// sectionsCapped is set when the row cap dropped a first-level row (or a
	// total), so a layout section with no row may have rows the run did not
	// read.
	sectionsCapped bool
}

// groupedPage is a grouped run's page: after is the open group the previous
// page's last detail row was in (nil on page 1), last this page's last
// detail row's id ("" on the last page).
type groupedPage struct {
	after []any
	last  string
}

// Run runs a report for its reader. The statements read only rows the
// reader's list rules allow; they never write.
func Run(ctx context.Context, app core.App, req Request) (*Result, error) {
	c, err := readerCompiler(app, req.Spec, req.Auth, req.Headers, req.Params, req.Options.Today, "options.today", req.Options.TimeZone)
	if err != nil {
		return nil, err
	}
	p, params := c.p, c.params
	if c.viewSort, err = viewSortPlans(c, req.Options.Sort); err != nil {
		return nil, err
	}
	expand, err := parseExpansion(req.Options)
	if err != nil {
		return nil, err
	}
	// No expansion state: the levels marked open expand (openLevels).
	openLevels(p, req.Options, expand)
	// The group or page, Total, detail, nest, pivot, heading, probe and
	// held-group statements all read through rn.query in this one
	// transaction, so a write saved while the run reads is in none of them.
	// Reads of labels and settings, not figures, stay outside it: the fiscal
	// start (fiscalStartOf, above), the drill targets (drillTargets), the
	// topics of a statement (topicsRead), the header lines (headerLines →
	// pickLabels) and the source's Heading hook.
	db, endRead, err := beginRead(ctx, app)
	if err != nil {
		return nil, err
	}
	defer endRead()
	rn := &runner{
		ctx: ctx, app: app, db: db, c: c, opts: req.Options, expand: expand, askedRowCap: req.RowCap, askedPageCap: req.PageCap, live: req.Live, refuseOver: req.refuseOver,
		res:    &Result{Rows: []Row{}, Notices: []Notice{}},
		topics: map[string]bool{}, ruleTopics: map[string][]string{}, ruleAnswers: map[string]bool{},
	}
	if req.Loaded != nil {
		rn.loadNotices(req.Loaded)
	}
	if err := rn.run(); err != nil {
		return nil, err
	}
	rn.res.Params = resolvedParams(p.spec.Params, params)
	if rn.res.HeaderLines, err = c.headerLines(ctx); err != nil {
		return nil, err
	}
	if h := p.base.Heading; h != nil {
		rn.res.Heading = strings.TrimSpace(h(app, c.info))
	}
	return rn.res, nil
}

// readerCompiler binds a spec for its reader and resolves its params for the
// reader's day (today, given at todayAt) in the reader's zone (timeZone):
// what every read of a report, a run or a param's options, starts from. A
// report never reads as a superuser or as nobody.
func readerCompiler(app core.App, spec *Spec, auth *core.Record, headers map[string]string, given map[string]json.RawMessage, today, todayAt, timeZone string) (*compiler, error) {
	if auth == nil || auth.IsSuperuser() {
		return nil, ErrNoReader
	}
	p, specErr := bind(app, spec)
	if specErr != nil {
		return nil, specErr
	}
	loc := readerZone(timeZone)
	day, err := todayOf(today, todayAt, loc)
	if err != nil {
		return nil, err
	}
	info := listRequestInfo(auth, headers)
	// One read of the fiscal start month serves the presets and the
	// formulas, so a settings change mid-read cannot make them disagree.
	fiscal := fiscalStartOf(p.base, app, info)
	params, specErr := resolveParams(p.spec.Params, given, periodContext{today: day, fiscalStart: fiscal, loc: loc})
	if specErr != nil {
		return nil, specErr
	}
	c := newCompilerWithFiscal(p, app, info, params, fiscal)
	c.given = given
	c.loc = loc
	return c, nil
}

// todayOf reads the reader's day, given at at in the request. With none it
// is today in the reader's zone (loc; nil is UTC).
func todayOf(s, at string, loc *time.Location) (time.Time, error) {
	if s == "" {
		if loc == nil {
			return time.Now().UTC().Truncate(24 * time.Hour), nil
		}
		now := time.Now().In(loc)
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	if !isDay(s) {
		return time.Time{}, invalid(at, "today is YYYY-MM-DD")
	}
	return time.Parse(time.DateOnly, s)
}

func (rn *runner) loadNotices(info *LoadInfo) {
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

// rowCap is the most rows a pivot, nest or layout pass reads: the request's
// RowCap when it is set, else runLimits.rowCap.
func (rn *runner) rowCap() int {
	if rn.askedRowCap > 0 {
		return rn.askedRowCap
	}
	return runLimits.rowCap
}

// rowCapNotice says the row cap applied.
func (rn *runner) rowCapNotice() Notice {
	limit := rn.rowCap()
	return Notice{Code: "rowCap", Message: fmt.Sprintf("The report shows its first %d rows.", limit), Count: limit}
}

func (rn *runner) run() error {
	if err := rn.resolveDrills(); err != nil {
		return err
	}
	// A pivot report has no detail rows to page.
	if rn.c.p.pivot != nil && rn.opts.Cursor != "" {
		return badCursor()
	}
	paged := rn.pagedGroups()
	groupPass := rn.groupPass
	if paged {
		groupPass = rn.pagedGroupPass
	}
	groupRows, err := groupPass()
	if err != nil {
		return err
	}
	var details []map[string]any
	switch {
	case paged:
		// The page's open innermost groups, anchors included, in document
		// order. In mode after the cursor's own group was not read, so it is
		// never one of them; the group cursor was read and checked already.
		leaves, shown, err := rn.openGroups(groupRows)
		if err != nil {
			return err
		}
		// An export past its row cap stops before the detail statement, the
		// largest of the run, when the group statement's counts already say so.
		if rn.pastRefusal(groupRows, leaves, shown) {
			return errPastRows
		}
		if len(leaves) > 0 {
			if details, err = rn.detailPass(leaves); err != nil {
				return err
			}
		}
	case rn.c.p.pivot == nil:
		var leaves [][]any
		grouped := len(rn.c.p.groups) > 0
		if grouped {
			if leaves, _, err = rn.openGroups(groupRows); err != nil {
				return err
			}
		}
		// A cursor is read even with no open group, so one whose group shut
		// is refused rather than answered with no rows.
		if !grouped || len(leaves) > 0 || rn.opts.Cursor != "" {
			details, err = rn.detailPass(leaves)
			if err != nil {
				return err
			}
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
	if err := rn.readHeadings(); err != nil {
		return err
	}
	rn.res.Columns = rn.columns()
	rn.markCurrencies(groupRows)
	rn.assemble(groupRows, details, nested)
	if rn.assembleErr != nil {
		return rn.assembleErr
	}
	if paged {
		if err := rn.cutPage(rn.pageLimit()); err != nil {
			return err
		}
	} else {
		rn.pageWindow()
	}
	rn.markMixedLookups()
	// A source's Go hooks read collections no statement reads, so their
	// topics are added by name.
	all := Sources(rn.app)
	reads := func(src Source) bool {
		return slices.ContainsFunc(src.collections(), func(c string) bool {
			return rn.topics[changesignal.Topic(c)]
		})
	}
	for _, src := range all {
		if reads(src) {
			for _, w := range src.Watch {
				rn.topics[changesignal.Topic(w)] = true
			}
		}
	}
	rn.res.Topics = make([]string, 0, len(rn.topics))
	// A manual run follows no data: its topics stay empty. This is decided
	// after every Watch is added, so a watched collection counts as read
	// whatever the order of the sources.
	if !rn.live && slices.ContainsFunc(all, func(src Source) bool { return src.NotLive && reads(src) }) {
		rn.res.Manual = true
		return nil
	}
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
	_, _, err = rn.c.detailSelect(r, "w")
	return err
}

// observeRead, when a test sets it, receives every read statement and its
// params before it runs, so a test can prepare each one as the run does.
// Nothing else sets it.
var observeRead func(sql string, params dbx.Params)

// topicsRead finds the change-signal topics of a read; a test wraps it to see
// what a run explains.
var topicsRead = changesignal.TopicsRead

// checkTopics, when a test sets it, receives every read statement and the
// topics the run found for it, so a test can compare them with the topics of
// the whole statement. Nothing else sets it.
var checkTopics func(ctx context.Context, app core.App, sql string, params dbx.Params, topics []string)

// readRows runs one read statement with the rule CTEs and ctes through db
// under the run time limit; with topics (a run's cache of each rule's
// topics), it also returns the change-signal topics of every collection the
// statement reads. A rule that one answer serves is asked through db before
// the statement is written (askRules); answers is a run's map of them, nil
// for a read that is no part of a run.
func readRows(ctx context.Context, app core.App, db dbx.Builder, s *stmt, ctes []string, body string, topics map[string][]string, answers map[string]bool) ([]map[string]any, []string, error) {
	ctes = materializeShared(ctes, body)
	s.ask = askRules(ctx, db, answers)
	with, err := s.with(ctes...)
	if err != nil {
		return nil, nil, err
	}
	sql := with + body
	if observeRead != nil {
		observeRead(sql, s.params)
	}
	ctx, cancel := context.WithTimeout(ctx, runLimits.timeout)
	defer cancel()
	var read []string
	if topics != nil {
		if read, err = s.topics(ctx, app, sql, topics); err != nil {
			return nil, nil, mapRunError(err)
		}
		if checkTopics != nil {
			checkTopics(ctx, app, sql, s.params, read)
		}
	}
	rows, err := db.NewQuery(sql).Bind(s.params).WithContext(ctx).Rows()
	if err != nil {
		return nil, nil, mapRunError(err)
	}
	out, err := scanRows(rows)
	if err != nil {
		return nil, nil, mapRunError(err)
	}
	return out, read, nil
}

// query runs one statement under the time limit and records the topics of
// every collection it reads. It reads through the run's read transaction; a
// runner built outside Run (a test of one pass) has none and reads through
// the app.
func (rn *runner) query(s *stmt, ctes []string, body string) ([]map[string]any, error) {
	db := rn.db
	if db == nil {
		db = rn.app.DB()
	}
	rows, topics, err := readRows(rn.ctx, rn.app, db, s, ctes, body, rn.ruleTopics, rn.ruleAnswers)
	for _, t := range topics {
		rn.topics[t] = true
	}
	return rows, err
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
	c.condProbes, c.probed = nil, nil
	if r.filterMixed != "" {
		c.probe("filter", "SELECT 1 "+r.root.render()+"\nWHERE "+r.filterMixed)
	}
	r.probeWheres()
	ctes, body, err := c.groupStatement(s, r, rn.expand, pivot, int64(rn.rowCap()+1))
	if err != nil {
		return nil, err
	}
	ctes = append([]string{r.render()}, ctes...)
	rows, err := rn.query(s, ctes, body)
	if err != nil {
		return nil, err
	}
	if err := rn.readProbes(s, ctes, rows, c.condProbes); err != nil {
		return nil, err
	}
	return rn.capLevelRows(rows), nil
}

// pagedGroups is true for a run whose group rows page (J1a): a grouped
// report with no pivot (a pivot's columns come from every row), no layout (a
// statement's computed rows read every section) and no nest (child rows page
// with their detail rows). The others keep one group statement and its cap.
func (rn *runner) pagedGroups() bool {
	p := rn.c.p
	return len(p.groups) > 0 && p.pivot == nil && len(p.layout) == 0 && len(p.nest) == 0
}

// pageLimit is the most rows a page holds: options.limit up to maxPage (up
// to the request's PageCap when it is set), else pageSize.
func (rn *runner) pageLimit() int {
	if rn.opts.Limit <= 0 {
		return runLimits.pageSize
	}
	if rn.askedPageCap > 0 {
		return min(rn.opts.Limit, rn.askedPageCap)
	}
	return min(rn.opts.Limit, runLimits.maxPage)
}

// pagedGroupPass reads one page of a grouped run's level rows (J1a) and the
// Total rows in one statement, so w and the level CTEs are built once for
// both. The levels compile once; the cursor is read from their order terms
// before the page statement is written, as its keyset compares their values.
func (rn *runner) pagedGroupPass() ([]map[string]any, error) {
	c := rn.c
	s := newStmt(rn.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	c.condProbes, c.probed = nil, nil
	if r.filterMixed != "" {
		c.probe("filter", "SELECT 1 "+r.root.render()+"\nWHERE "+r.filterMixed)
	}
	r.probeWheres()
	levels, err := c.compileLevels(s, r, rn.expand, nil, nil)
	if err != nil {
		return nil, err
	}
	rn.groupShape = make([][]orderTerm, 0, len(levels)-1)
	for _, l := range levels[1:] {
		rn.groupShape = append(rn.groupShape, l.lv.terms)
	}
	// The detail sort keys are in the cursor's fingerprint: a cursor from
	// another sort is refused even on a page with no detail row. This compile
	// also gives the columns their detail types (detailTypes). It runs over a
	// w of its own, as detailTypes does: over r, the sort keys' columns and
	// binds would go into the page statement (with its Total), which never
	// reads them. A second rows() gives the same currency keys, and the levels are
	// compiled already.
	keysOf, err := c.rows(newStmt(rn.app, c.info))
	if err != nil {
		return nil, err
	}
	if _, rn.groupKeys, err = c.detailSelect(keysOf, "w"); err != nil {
		return nil, err
	}
	rn.reachCut = !c.detailWholeReport && len(c.detailSortMixed) == 0
	rn.groupFP = groupFingerprint(c.groupLevels(), c.inlineLevels(), levelShape(rn.groupShape), foldPrint(rn.expand), rn.groupKeys)
	if rn.opts.Cursor != "" {
		cur, err := readGroupCursor(rn.groupFP, rn.groupShape, rn.groupKeys, len(c.p.groups), len(c.currencies), rn.opts.Cursor)
		if err != nil {
			return nil, err
		}
		rn.groupCur = &cur
	}
	levelCTEs, pageCTEs, body, err := c.pagedGroupStatement(s, levels, rn.expand, rn.groupCur, int64(rn.pageLimit()))
	if err != nil {
		return nil, err
	}
	ctes := slices.Concat([]string{r.render()}, levelCTEs, pageCTEs)
	read, err := rn.query(s, ctes, body)
	if err != nil {
		return nil, err
	}
	if err := rn.readProbes(s, ctes, read, c.condProbes); err != nil {
		return nil, err
	}
	// The statement returns the page's level rows, then the Total rows; the
	// first level row carries the anchors the page read again.
	if len(read) > 0 && toInt(read[0]["__level"]) != 0 && rn.groupCur != nil {
		rn.anchored = int(toInt(read[0]["__anchored"]))
	}
	rn.shown, _ = rn.shownBefore()
	rn.rawGroups, rn.rawDetails = map[string]map[string]any{}, map[string]map[string]any{}
	return read, nil
}

// pagedGroupStatement selects one page of level rows (J1a) in document order:
// the currency blocks in Total order (R7), then each level's sibling ordinal
// down the path (a parent before its children). Its rows are the cursor's
// anchors (the enclosing groups the page starts inside, for their subtotals)
// and the rows after the cursor, at most limit+1 of them past the anchors,
// so the cut (cutPage) knows whether more follow. Every o<L> keeps its
// lookup layers and record joins (#452); the keyset and limit go on the outer
// select. having and fill already shaped o<L>, so they apply before the
// keyset. The levels are compiled (compileLevels) and the cursor read
// before this is called: the cursor's shape is the levels' order terms.
func (c *compiler) pagedGroupStatement(s *stmt, levels []compiledLevel, expand *expansion, cur *groupCursor, limit int64) (levelCTEs, pageCTEs []string, body string, err error) {
	groups := len(c.p.groups)
	unread := c.unreadCells(levels, nil)
	for _, l := range levels[1:] {
		l.lv.paged = true
	}
	for _, l := range levels {
		levelCTEs = append(levelCTEs, l.lv.render(withoutCells(l.cells, unread), l.own, l.totals)...)
	}
	// The union names its columns, so each leg's own order values line up
	// with the other legs' NULLs.
	base := []string{"__level", "__row"}
	for i := range groups {
		base = append(base, fmt.Sprintf("k%d", i))
	}
	base = append(base, c.currencies...)
	for _, cell := range withoutCells(levels[1].cells, unread) {
		_, name := splitCell(cell)
		base = append(base, name)
	}
	base = append(base, "__ord", "__own", "__label", "__labelcur", "__n", "__kids", "__inline", "__muted")
	skipping := false
	for n := 1; n <= groups; n++ {
		skipping = skipping || c.skips(n) || c.inlines(n)
	}
	// pageOnly are the columns a page row has past base, which the Total leg
	// reads as NULL.
	var pageOnly []string
	if skipping {
		pageOnly = append(pageOnly, uncountedCell)
	}
	for i := 1; i <= groups; i++ {
		pageOnly = append(pageOnly, fmt.Sprintf("__p%d", i))
	}
	for m := 1; m <= groups; m++ {
		for j := range levels[m].lv.terms {
			pageOnly = append(pageOnly, orderValue(m, j))
		}
	}
	legs := make([]string, 0, groups)
	for n := 1; n <= groups; n++ {
		cols := prefixed("o.", base)
		// A skip level's blank key and an inlined group are read (their
		// children's path, the cursor's order values) but draw no row
		// (children), so the limit does not count them.
		if skipping {
			var uncounted []string
			if c.skips(n) {
				uncounted = append(uncounted, fmt.Sprintf("(o.k%d IS NULL)", n-1))
			}
			if c.inlines(n) {
				uncounted = append(uncounted, "(o.__inline = 1)")
			}
			blank := "0"
			if len(uncounted) > 0 {
				blank = "(" + strings.Join(uncounted, " OR ") + ")"
			}
			cols = append(cols, blank+" AS "+uncountedCell)
		}
		for i := 1; i <= groups; i++ {
			if i <= n {
				cols = append(cols, fmt.Sprintf("pp.__p%d", i))
			} else {
				cols = append(cols, fmt.Sprintf("NULL AS __p%d", i))
			}
		}
		for m := 1; m <= groups; m++ {
			for j := range levels[m].lv.terms {
				if m == n {
					cols = append(cols, "o."+orderValue(m, j))
				} else {
					cols = append(cols, "NULL AS "+orderValue(m, j))
				}
			}
		}
		legs = append(legs, fmt.Sprintf("SELECT %s FROM o%d o JOIN pp%d pp ON %s WHERE %s",
			strings.Join(cols, ", "), n, n, strings.Join(c.sameGroup("o", "pp", n), " AND "), expand.shownUnder(c, s, n-1, "o.")))
	}
	keyset, pc, anchors := c.groupKeyset(s, levels, cur)
	pageCTEs = c.pathCTEs()
	// Each row carries how many anchors the page read again (pageStart starts
	// after the deepest of them).
	selected := "*"
	if pc != "" {
		pageCTEs = append(pageCTEs, pc)
		selected = "*, (SELECT an FROM pc) AS __anchored"
		pageOnly = append(pageOnly, "__anchored")
	}
	var order []string
	for _, ck := range c.currencies {
		order = append(order, "("+ck+" IS NULL)", ck)
	}
	for i := 1; i <= groups; i++ {
		order = append(order, fmt.Sprintf("__p%d", i))
	}
	rows := limitSQL(limit + 1 + int64(anchors))
	orderBy := strings.Join(order, ", ")
	union := strings.Join(legs, "\nUNION ALL\n")
	var page string
	if skipping {
		// A LIMIT would count the blank keys of skip levels and the inlined
		// groups, which draw no row: a page of them would end early and look
		// like the last one. The running count of the rows that draw takes its
		// place.
		page = fmt.Sprintf("SELECT * FROM (SELECT %s, SUM(1 - %s) OVER (ORDER BY %s ROWS UNBOUNDED PRECEDING) AS __counted FROM (%s)\nWHERE %s)\nWHERE __counted <= %s\nORDER BY %s",
			selected, uncountedCell, orderBy, union, keyset, rows, orderBy)
		pageOnly = append(pageOnly, "__counted")
	} else {
		page = fmt.Sprintf("SELECT %s FROM (%s)\nWHERE %s\nORDER BY %s LIMIT %s", selected, union, keyset, orderBy, rows)
	}
	// The Total rows (o0, one per currency) are read in the same statement,
	// over the same w and level CTEs, after the page's rows: every row the
	// filter keeps, whatever page the group rows are on. The condition probes
	// ride on it (readProbes), as the Total always returns a row unless there
	// are no rows at all.
	total := slices.Clone(base)
	for _, name := range pageOnly {
		total = append(total, "NULL AS "+name)
	}
	probes := c.probeCells()
	if cte := c.probeCTE(); cte != "" {
		pageCTEs = append(pageCTEs, cte)
	}
	body = fmt.Sprintf("SELECT %s FROM (SELECT * FROM (%s)\nUNION ALL\nSELECT %s FROM o0)\nORDER BY (__level = 0), %s",
		strings.Join(append([]string{"*"}, probes...), ", "), page, strings.Join(total, ", "), orderBy)
	return levelCTEs, pageCTEs, body, nil
}

// uncountedCell is 1 on a paged level row that draws no row of its own (a
// skip level's blank key, an inlined group), which the page's limit does not
// count.
const uncountedCell = "__uncounted"

// sameGroup matches a's row to b's by the first n group keys and the
// currency keys, with IS, so a blank key matches a blank one.
func (c *compiler) sameGroup(a, b string, n int) []string {
	out := make([]string, 0, n+len(c.currencies))
	for i := range n {
		out = append(out, fmt.Sprintf("%s.k%d IS %s.k%d", a, i, b, i))
	}
	for _, ck := range c.currencies {
		out = append(out, a+"."+ck+" IS "+b+"."+ck)
	}
	return out
}

// pathCTEs give each level row its path: pp<L> holds a level-L row's keys and
// the sibling ordinals (__ord) of it and each enclosing group, __p1 to __p<L>,
// which order the page's rows in document order.
func (c *compiler) pathCTEs() []string {
	groups := len(c.p.groups)
	ctes := make([]string, 0, groups)
	ctes = append(ctes, fmt.Sprintf("pp1 AS (SELECT %s FROM o1)", strings.Join(append(append([]string{"k0"}, c.currencies...), "__ord AS __p1"), ", ")))
	for n := 2; n <= groups; n++ {
		var cols []string
		for i := range n {
			cols = append(cols, fmt.Sprintf("o.k%d", i))
		}
		cols = append(cols, prefixed("o.", c.currencies)...)
		for i := 1; i < n; i++ {
			cols = append(cols, fmt.Sprintf("pp.__p%d", i))
		}
		cols = append(cols, fmt.Sprintf("o.__ord AS __p%d", n))
		ctes = append(ctes, fmt.Sprintf("pp%d AS (SELECT %s FROM o%d o JOIN pp%d pp ON %s)",
			n, strings.Join(cols, ", "), n, n-1, strings.Join(c.sameGroup("o", "pp", n-1), " AND ")))
	}
	return ctes
}

// groupKeyset selects the page's rows after a group cursor ("1" without
// one), the CTE pc it reads ("" without one), and the number of anchors: the
// enclosing groups of where the page starts, which the page reads again so
// their open rows' subtotals follow their children (the path group too when
// the page starts inside it). With d the cursor's depth, a row is kept when it
// is an anchor, inside the path group (mode in), in a later currency block,
// or under the path's level-(i-1) group and after its level-i group, i = 1..d.
// "After" compares sibling ordinals with the count of siblings at or before
// the cursor's order values (Ruling R1), so a group added or removed between
// pages shifts no other.
func (c *compiler) groupKeyset(s *stmt, levels []compiledLevel, cur *groupCursor) (string, string, int) {
	if cur == nil {
		return "1", "", 0
	}
	d := len(cur.Path)
	curs := cur.Group[d:]
	anchors := d
	if cur.Mode == cursorAfter {
		anchors = d - 1
	}
	// under is the first n of the cursor's group keys and its currency keys,
	// on the columns of table (bare for "").
	under := func(table string, n int) []string {
		prefix := ""
		if table != "" {
			prefix = table + "."
		}
		out := make([]string, 0, n+len(curs))
		for i := range n {
			out = append(out, fmt.Sprintf("%sk%d IS %s", prefix, i, s.bind(cur.Group[i])))
		}
		for j, ck := range c.currencies {
			out = append(out, prefix+ck+" IS "+s.bind(curs[j]))
		}
		return out
	}
	// An anchor is read again only while it and each anchor around it still
	// have the cursor's order values: pc's "an" counts the anchors from the
	// top that do. A group that sorts elsewhere now (renamed, re-weighed)
	// counts as removed, so the page does not start at its new place and
	// skip the rows between; the keyset still reads it there when it sorts
	// after the cursor.
	still := make([]string, 0, anchors+1)
	for n := 1; n <= anchors; n++ {
		same := under("x", n)
		for j, v := range cur.Path[n-1] {
			same = append(same, "x."+orderValue(n, j)+" IS "+s.bind(v))
		}
		still = append(still, fmt.Sprintf("WHEN NOT EXISTS (SELECT 1 FROM o%d x WHERE %s) THEN %d", n, strings.Join(same, " AND "), n-1))
	}
	anchored := "0"
	if anchors > 0 {
		anchored = fmt.Sprintf("CASE %s ELSE %d END", strings.Join(still, " "), anchors)
	}
	var terms []string
	for n := 1; n <= anchors; n++ {
		terms = append(terms, "("+strings.Join(append([]string{fmt.Sprintf("__level = %d", n)}, append(under("", n), fmt.Sprintf("(SELECT an FROM pc) >= %d", n))...), " AND ")+")")
	}
	if cur.Mode == cursorIn {
		terms = append(terms, "("+strings.Join(append([]string{fmt.Sprintf("__level > %d", d)}, append(under("", d), fmt.Sprintf("(SELECT an FROM pc) >= %d", d))...), " AND ")+")")
	}
	if len(c.currencies) > 0 {
		var cols []orderTerm
		var values []any
		for j, ck := range c.currencies {
			blank := int64(0)
			if curs[j] == nil {
				blank = 1
			}
			cols = append(cols, orderTerm{sql: "(" + ck + " IS NULL)"}, orderTerm{sql: ck})
			values = append(values, blank, curs[j])
		}
		terms = append(terms, afterSQL(s, cols, values))
	}
	counts := make([]string, 0, d)
	for i := 1; i <= d; i++ {
		xTerms := make([]orderTerm, len(levels[i].lv.terms))
		for j, t := range levels[i].lv.terms {
			xTerms[j] = orderTerm{sql: "x." + orderValue(i, j), desc: t.desc}
		}
		conds := append(under("x", i-1), "NOT "+afterSQL(s, xTerms, cur.Path[i-1]))
		counts = append(counts, fmt.Sprintf("(SELECT count(*) FROM o%d x WHERE %s) AS c%d", i, strings.Join(conds, " AND "), i))
		terms = append(terms, "("+strings.Join(append(under("", i-1), fmt.Sprintf("__p%d > (SELECT c%d FROM pc)", i, i)), " AND ")+")")
	}
	counts = append(counts, anchored+" AS an")
	return anyOf(terms), "pc AS (SELECT " + strings.Join(counts, ", ") + ")", anchors
}

// probesCTE is the CTE that computes, once, whether each condition probe
// of the group statement found a row (__probe<i>, condProbe); every leg of
// the statement reads it, so the probes cost no statement of their own.
const probesCTE = "__probes"

// probeCTE is the probes' CTE, "" with no probe.
func (c *compiler) probeCTE() string {
	if len(c.condProbes) == 0 {
		return ""
	}
	cols := make([]string, len(c.condProbes))
	for i, p := range c.condProbes {
		cols[i] = fmt.Sprintf("EXISTS (%s) AS __probe%d", p.sql, i)
	}
	return fmt.Sprintf("%s AS MATERIALIZED (SELECT %s)", probesCTE, strings.Join(cols, ", "))
}

// probeCells are the cells a leg of the group statement reads the probes'
// results into, one per probe.
func (c *compiler) probeCells() []string {
	out := make([]string, len(c.condProbes))
	for i := range c.condProbes {
		out[i] = fmt.Sprintf("(SELECT __probe%d FROM %s) AS __probe%d", i, probesCTE, i)
	}
	return out
}

// readProbes names the conditions whose probe found a row, read from the
// group statement's first row, and takes the probe cells off every row. A
// statement that returned no row carries no result, so its probes run in a
// statement of their own.
func (rn *runner) readProbes(s *stmt, ctes []string, rows []map[string]any, probes []condProbe) error {
	if len(probes) == 0 {
		return nil
	}
	if len(rows) == 0 {
		return rn.probeConditions(s, ctes, probes)
	}
	for i, p := range probes {
		name := fmt.Sprintf("__probe%d", i)
		if toInt(rows[0][name]) != 0 && !slices.Contains(rn.mixedConditions, p.at) {
			rn.mixedConditions = append(rn.mixedConditions, p.at)
		}
		for _, row := range rows {
			delete(row, name)
		}
	}
	return nil
}

// condProbe is a condition that reads a sum lookup which may meet two
// currencies (expr.condMixed): sql selects a row where one did, among the
// rows the condition decides on, inside the statement's CTEs. at is the
// condition's spec path.
type condProbe struct {
	at, sql string
}

// probe adds a condition's probe; one that decides on the rows of several
// levels (a sort key) is probed over each. A probe added again (a lookup's
// where compiled once for each cell of a computed row) is probed once.
func (c *compiler) probe(at, sql string) {
	key := at + "\n" + sql
	if c.probed[key] {
		return
	}
	if c.probed == nil {
		c.probed = map[string]bool{}
	}
	c.probed[key] = true
	i := slices.IndexFunc(c.condProbes, func(p condProbe) bool { return p.at == at })
	if i < 0 {
		c.condProbes = append(c.condProbes, condProbe{at: at, sql: sql})
		return
	}
	c.condProbes[i].sql += "\nUNION ALL\n" + sql
}

// probeConditions finds the conditions of a statement that read a sum
// lookup which met two currencies on some row. Such a sum has no value, so
// the condition was unknown there: it hid that group, dropped that row or
// left out that record unless another term held; a sort key sorted it as
// blank. A row a condition dropped is on no page, and a row a key sorted may
// be on another, so the conditions are probed over the rows they decide on,
// in a statement of their own.
func (rn *runner) probeConditions(s *stmt, ctes []string, probes []condProbe) error {
	if len(probes) == 0 {
		return nil
	}
	cols := make([]string, len(probes))
	for i, p := range probes {
		cols[i] = fmt.Sprintf("EXISTS (%s) AS m%d", p.sql, i)
	}
	rows, err := rn.query(s, ctes, "SELECT "+strings.Join(cols, ", "))
	if err != nil || len(rows) == 0 {
		return err
	}
	for i, p := range probes {
		if toInt(rows[0][fmt.Sprintf("m%d", i)]) != 0 && !slices.Contains(rn.mixedConditions, p.at) {
			rn.mixedConditions = append(rn.mixedConditions, p.at)
		}
	}
	return nil
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
	limit := rn.rowCap()
	if levels <= limit {
		return rows
	}
	rn.noticeOnce(rn.rowCapNotice())
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if row["__row"] != nil || len(out) < limit {
			out = append(out, row)
		} else if toInt(row["__level"]) <= 1 {
			rn.sectionsCapped = true
		}
	}
	return out
}

func notASortKey(at string) error {
	return invalid(at, "a running value or a percent is not a sort key")
}

// viewSortPlans checks a run's viewer sort (options.sort) and plans its keys.
// It is a run input, not a spec part: a link outlives the report's columns,
// so each key is checked against the report as it is now (R7). A key on the
// first column of a report with a detail label sorts by that label. A running
// value is refused where it compiles (detailSortValue).
func viewSortPlans(c *compiler, keys []ViewSort) ([]sortPlan, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if len(keys) > maxSort {
		return nil, invalid("options.sort", fmt.Sprintf("a run sorts by at most %d columns", maxSort))
	}
	if c.p.pivot != nil {
		return nil, invalid("options.sort", "a pivot report has no opened rows to sort")
	}
	seen := map[string]bool{}
	out := make([]sortPlan, 0, len(keys))
	for i, k := range keys {
		at := fmt.Sprintf("options.sort.%d", i)
		cp := c.p.byID[k.Col]
		d := c.p.spec.Detail
		// The first column of a report with a detail label sorts by that label,
		// even when the column is a child row column (Sortable says the same).
		byLabel := cp != nil && cp == c.p.cols[0] && d != nil && d.Label != nil
		switch {
		case cp == nil || cp.col.Hidden:
			return nil, unknownColumn(at+".col", k.Col)
		case seen[k.Col]:
			return nil, invalid(at+".col", "a column sorts once")
		case k.Dir != "" && k.Dir != "asc" && k.Dir != "desc":
			return nil, invalid(at+".dir", "dir is asc or desc")
		case cp.nest > 0 && !byLabel:
			return nil, invalid(at+".col", "a child row column is not a sort key")
		}
		seen[k.Col] = true
		s := sortPlan{col: cp, at: at + ".col", desc: k.Dir == "desc"}
		if byLabel {
			s = sortPlan{formula: d.Label, at: at + ".col", desc: s.desc}
		}
		out = append(out, s)
	}
	return out, nil
}

// groupStatement is the UNION ALL of every level's rows (the children of
// expanded groups only) and the layout's computed rows: the one group
// statement of a run whose group rows do not page (a pivot, a layout, a nest).
func (c *compiler) groupStatement(s *stmt, r *rowsCTE, expand *expansion, pivot []pivotValue, limit int64) ([]string, string, error) {
	var ctes, legs []string
	levels, err := c.compileLevels(s, r, expand, pivot, func(n int) {
		leg := fmt.Sprintf("SELECT * FROM o%d", n)
		if n >= 2 {
			// Every enclosing group must be open, not only the parent: a
			// row under a folded grandparent would else count toward the
			// limit and could cut rows that show.
			leg += " WHERE " + expand.shownUnder(c, s, n-1, "")
		}
		legs = append(legs, leg)
	})
	if err != nil {
		return nil, "", err
	}
	layoutCTEs, computed, err := c.layoutLegs(s, pivot)
	if err != nil {
		return nil, "", err
	}
	unread := c.unreadCells(levels, computed)
	for _, l := range levels {
		ctes = append(ctes, l.lv.render(withoutCells(l.cells, unread), l.own, l.totals)...)
	}
	ctes = append(ctes, layoutCTEs...)
	// The conditions that may read a mixed sum are probed in this statement
	// (readProbes), over the CTEs it computes anyway.
	probes := c.probeCells()
	if cte := c.probeCTE(); cte != "" {
		ctes = append(ctes, cte)
	}
	// The limit applies to the level rows only: computed rows sort last, so a
	// limit on the whole statement would drop them first.
	first := strings.Join(append([]string{"*"}, probes...), ", ")
	body := []string{fmt.Sprintf("SELECT %s FROM (%s\nORDER BY __level, __ord LIMIT %s)", first, strings.Join(legs, "\nUNION ALL\n"), limitSQL(limit))}
	for _, leg := range computed {
		cells := append(withoutCells(leg.cells, unread), probes...)
		body = append(body, "SELECT "+strings.Join(cells, ", ")+" FROM "+leg.from)
	}
	return ctes, strings.Join(body, "\nUNION ALL\n") + "\nORDER BY __level, __ord", nil
}

// compiledLevel is one level of a group statement, compiled and not yet
// rendered: each level's o<L> renders once every leg's cells are known, as a
// helper column no leg has a value for is left out of all of them.
type compiledLevel struct {
	lv            *level
	cells, totals []string
	own           string
}

// compileLevels compiles levels 0 (the Total) to the innermost group. A
// level binds its parameters on s and the levels share compiler state (the
// level types, the section currencies, the fill keysets), so a statement
// compiles its levels once, in this order. compiled, when given, runs after
// each level compiles: what it binds keeps its place among the levels'
// parameters, so the statement's text does not move.
func (c *compiler) compileLevels(s *stmt, r *rowsCTE, expand *expansion, pivot []pivotValue, compiled func(n int)) ([]compiledLevel, error) {
	c.levelTypes, c.sectionCur, c.fillSets = map[string]FieldType{}, map[string]currency{}, map[int]string{}
	c.sectionMixed = map[string]bool{}
	// readsSums is true when the level above adds this level's "rows" cells
	// (childSum), which rt<L> then holds.
	readsSums := false
	var levels []compiledLevel
	for n := 0; n <= len(c.p.groups); n++ {
		lv, err := c.level(r, n, expand)
		if err != nil {
			return nil, err
		}
		cells := make([]string, 0, len(c.p.cols))
		labels := make([]string, 0, len(c.p.cols))
		carried := make([]string, 0, len(c.p.cols))
		for idx, cp := range c.p.cols {
			e, err := lv.cell(cp, lv.frame)
			if err != nil {
				return nil, err
			}
			cells = append(cells, fmt.Sprintf("%s AS v%d", cellSQL(e), idx))
			ref, _ := c.columnLabel(lv.labels, cp, e)
			labels = append(labels, ref.cells(idx)...)
			carried = append(carried, c.carriedCell(s, cp, e, idx), mixedCell(e, idx))
			c.levelTypes[cp.col.ID] = e.typ
			if n == 1 {
				c.sectionCur[cp.col.ID] = e.cur
				c.sectionMixed[cp.col.ID] = e.blankFor() != ""
			}
		}
		pivotCells, pivotMixed, err := c.pivotCells(lv, pivot)
		if err != nil {
			return nil, err
		}
		opening, err := lv.openingCells()
		if err != nil {
			return nil, err
		}
		drill, err := lv.drillCells(pivot)
		if err != nil {
			return nil, err
		}
		// The labels, the carried currencies, the pivot cells' mixed helpers,
		// then the opening cells, then the drill cells follow the pivot cells,
		// as in every leg of the union.
		cells = append(append(append(append(append(append(cells, pivotCells...), labels...), carried...), pivotMixed...), opening...), drill...)
		// A skip level's blank key passes on its shown children, so they are
		// counted before its condition compiles.
		lv.countShownKids()
		// The having's aggregates join a<L>, so it compiles before render.
		own, err := lv.own()
		if err != nil {
			return nil, err
		}
		if err := lv.groupConditions(); err != nil {
			return nil, err
		}
		var totals []string
		if readsSums {
			if totals, err = lv.rowsCells(); err != nil {
				return nil, err
			}
			if pv := c.p.pivot; pv != nil && pv.cell.col.Total == TotalRows {
				totals = append(append(totals, pivotCells...), pivotMixed...)
			}
		}
		levels = append(levels, compiledLevel{lv: lv, cells: cells, totals: totals, own: own})
		readsSums = len(lv.childCells) > 0
		if compiled != nil {
			compiled(n)
		}
	}
	return levels, nil
}

// unreadCells are the helper cells no leg of a statement over levels and the
// layout's computed legs has a value for (unreadHelpers), but those a
// layout's section chunks read by name.
func (c *compiler) unreadCells(levels []compiledLevel, computed []unionLeg) map[string]bool {
	all := make([][]string, 0, len(levels)+len(computed))
	for _, l := range levels {
		all = append(all, l.cells)
	}
	for _, leg := range computed {
		all = append(all, leg.cells)
	}
	unread := unreadHelpers(all)
	// A layout's section chunks read o1.vcur<idx> by name for every column
	// whose first-level currency has SQL (layoutLegs, lc.cellNames), so that
	// cell stays even when its SQL is the text NULL.
	for idx, cp := range c.p.cols {
		if c.sectionCur[cp.col.ID].sql != "" {
			delete(unread, fmt.Sprintf("vcur%d", idx))
		}
	}
	return unread
}

// unionLeg is one select of the group statement's UNION ALL: its cells
// ("expr AS name", in the union's column order) and its FROM.
type unionLeg struct {
	cells []string
	from  string
}

// helperCell matches a row's record label (vl<idx>), its label's currency
// (vc<idx>), a value's carried currency (vcur<idx>), a looked-up total's
// mixed check (vmx<idx>) and an opening row's cells (op<idx>, opcur<idx>,
// opmx<idx>).
var helperCell = regexp.MustCompile(`^(?:vl|vc|vcur|vmx|op|opcur|opmx)\d+$`)

// unreadHelpers are the helper cells that are NULL in every leg of a union:
// no row has a label or a carried currency there, and assembly reads a missing
// cell as NULL, so the statement leaves them out. Every leg selects the same
// cells in the same order. The one reader inside the statement is a layout's
// section chunks, which read o1.vcur<idx> by name (layout.go, lc.cellNames and
// sectionSlot.column); groupStatement keeps those cells.
func unreadHelpers(legs [][]string) map[string]bool {
	unread := map[string]bool{}
	for _, cells := range legs {
		for _, cell := range cells {
			expr, name := splitCell(cell)
			if !helperCell.MatchString(name) {
				continue
			}
			if _, seen := unread[name]; !seen {
				unread[name] = true
			}
			if expr != "NULL" {
				unread[name] = false
			}
		}
	}
	return unread
}

// withoutCells drops the cells named in drop.
func withoutCells(cells []string, drop map[string]bool) []string {
	out := make([]string, 0, len(cells))
	for _, cell := range cells {
		if _, name := splitCell(cell); !drop[name] {
			out = append(out, cell)
		}
	}
	return out
}

// splitCell splits "expr AS name" at its last " AS ".
func splitCell(cell string) (string, string) {
	i := strings.LastIndex(cell, " AS ")
	if i < 0 {
		return cell, ""
	}
	return cell[:i], cell[i+len(" AS "):]
}

// shownLevels returns the CTEs that select the shown rows of the deepest level
// whose group has a having (s<m>), and m; m is 0 when no group has one. The
// detail statement reads only the base rows under them. It compiles levels 1
// to m, and past m each next level whose rt<L> the level before it reads (any
// "rows" sum a compiled level reads: in its having, its sort keys or its own
// rt cells); no others. win is true when a value of those CTEs is a running
// value or a percent (expr.win), which reads rows of other groups.
func (c *compiler) shownLevels(r *rowsCTE, expand *expansion) ([]string, int, bool, error) {
	m := c.havingAbove(len(c.p.groups) + 1)
	if m == 0 {
		return nil, 0, false, nil
	}
	c.fillSets = map[int]string{}
	var ctes []string
	readsSums, win := false, false
	for n := 1; n <= m || readsSums; n++ {
		lv, err := c.level(r, n, expand)
		if err != nil {
			return nil, 0, false, err
		}
		own, err := lv.own()
		if err != nil {
			return nil, 0, false, err
		}
		var totals []string
		if readsSums {
			if totals, err = lv.rowsCells(); err != nil {
				return nil, 0, false, err
			}
		}
		base, _ := lv.base(own, totals)
		ctes = append(ctes, base...)
		readsSums = len(lv.childCells) > 0
		win = win || lv.win
	}
	return ctes, m, win, nil
}

// detailPass runs the detail statement: one row per base row, a page at a
// time. Without groups a page follows the detail order; with groups it holds
// the rows of the open innermost groups (leaves) in document order: the
// groups' order, then the detail order (R9b). Window values, sort keys and
// what they read are computed before the page is cut, so a running value goes
// on across pages; a row's other lookups and its cells are computed for the
// page's rows (pagedCells). Rows under
// a hidden group are left out before any running value reads them.
func (rn *runner) detailPass(leaves [][]any) ([]map[string]any, error) {
	c := rn.c
	s := newStmt(rn.app, c.info)
	r, err := c.rows(s)
	if err != nil {
		return nil, err
	}
	shown, m, win, err := c.shownLevels(r, rn.expand)
	if err != nil {
		return nil, err
	}
	table := "w"
	if m > 0 {
		table = "ws"
		shown = append(shown, "ws AS (SELECT w.* FROM w WHERE "+c.inShown(m, "w")+")")
	}
	limit := rn.pageLimit()
	paged := rn.pagedGroups()
	// A page of two or more open groups computes only the rows of the groups
	// it can reach (leafRows), when its select lets it (reachCut).
	cut, all := paged && rn.reachCut && len(leaves) > 1 && !legacySQL.everyLeafRows, table
	if cut {
		table = "wl"
	}
	sel, order, err := c.detailSelect(r, table)
	if err != nil {
		return nil, err
	}
	rn.readDetails = true
	grouped := len(c.p.groups) > 0
	// With open leaves, w reads only the base rows that can be in one of them
	// (leafCut), so it reads gord, which then comes first. heldCTEs is the CTE
	// list readShownNow reads a held group over: one with every row of w, as
	// a held group may be no leaf of this page. Both lists end with the
	// reached rows (leafRows) the select reads as wl: over the cut w they are
	// the same rows of each open group, and over the whole w they also keep
	// the shown row of a held group that is no leaf (away).
	whole := append([]string{r.render()}, shown...)
	ctes, heldCTEs := whole, whole
	if paged || grouped {
		gord := c.leafOrder(s, leaves)
		var reached []string
		if cut {
			held, away := rn.heldLeaves(leaves)
			reached = c.leafRows(s, all, held, away, limit+1)
		}
		heldCTEs = slices.Concat(whole, []string{gord}, reached)
		ctes = heldCTEs
		if r.cut = c.leafCut(r, leaves, m > 0, c.detailWin || win); r.cut != "" {
			ctes = slices.Concat([]string{gord, r.render()}, shown, reached)
		}
	}
	// The group cursor's fingerprint and values hold the sort keys the group
	// pass compiled: this pass orders by the same ones, or a cursor's values
	// would be read against other columns.
	if paged && !slices.Equal(order, rn.groupKeys) {
		return nil, fmt.Errorf("reports: the detail sort keys %v differ from the group cursor's %v", order, rn.groupKeys)
	}
	from, by, levels := "("+sel+") d", orderBy(order), ""
	var where string
	switch {
	case paged:
		from = c.leafFrom(sel)
		by = "d.__g, " + by
		if err = rn.readShownNow(s, heldCTEs, sel, order); err == nil {
			where = rn.pagedDetailAfter(s, order, leaves)
		}
	case grouped:
		levels = c.groupLevels()
		from = c.leafFrom(sel)
		by = "d.__g, " + by
		rn.page = &groupedPage{}
		where, rn.page.after, err = groupedAfter(s, levels, order, rn.opts.Cursor, leaves)
	default:
		where, err = keysetAfter(s, order, rn.opts.Cursor)
	}
	if err != nil {
		return nil, err
	}
	body := fmt.Sprintf("SELECT * FROM %s WHERE %s ORDER BY %s LIMIT %s", from, where, by, limitSQL(int64(limit+1)))
	// The cells that nothing below the cut reads are computed for the page.
	if c.detailCells != nil {
		body = c.detailCells(body, by)
	}
	rows, err := rn.query(s, ctes, body)
	if err != nil {
		return nil, err
	}
	// The probe reads every row of the groups this pass reads: every open
	// group on an unpaged run, this page's on a paged one.
	probes := make([]condProbe, len(c.detailSortMixed))
	for i, m := range c.detailSortMixed {
		probes[i] = condProbe{at: m.at, sql: fmt.Sprintf("SELECT 1 FROM %s WHERE d.%s = 1", from, m.sql)}
	}
	if err := rn.probeConditions(s, ctes, probes); err != nil {
		return nil, err
	}
	// A paged run cuts its page over every row kind (cutPage), which needs
	// the row past the limit to know a row follows.
	if paged {
		return rows, nil
	}
	more := len(rows) > limit
	if more {
		rows = rows[:limit]
	}
	rn.res.Page = &Page{Size: len(rows)}
	if more {
		last := rows[len(rows)-1]
		var group []any
		if grouped {
			group = rn.rowKey(last, len(c.p.groups))
			rn.page.last = fmt.Sprintf("d:%v", last["__id"])
		}
		rn.res.Page.Next = cursorOf(levels, order, last, group)
	}
	return rows, nil
}

// pagedDetailAfter selects the detail rows of a paged page: every row of the
// page's open innermost groups, but in a group whose first details an earlier
// page showed only its rows after them (F7's keyset). Such a group is the one
// the cursor stopped in after a detail row, the first leaf unless it moved
// later between pages, and each group the cursor holds (heldAfter): its rows
// up to the cursor were shown already, wherever it is now.
func (rn *runner) pagedDetailAfter(s *stmt, keys []sortKey, leaves [][]any) string {
	var conds []string
	for _, h := range rn.shown {
		// A group that is gone (removed between pages, or sorted before the
		// cursor now) or not a leaf of this page reads no row here. A fold
		// change never gets here: the fold print refused the cursor.
		at := slices.IndexFunc(leaves, func(k []any) bool { return keyString(k) == keyString(h.Group) })
		if at < 0 {
			continue
		}
		conds = append(conds, fmt.Sprintf("(d.__g <> %s OR %s)", s.bind(int64(at)), keysetCond(s, keys, h.Values)))
	}
	if len(conds) == 0 {
		return "1"
	}
	return strings.Join(conds, " AND ")
}

// shownBefore lists the groups whose first details earlier pages showed: the
// cursor's held groups, then the group the cursor stopped in after a detail
// row. moved marks those that sort elsewhere now: every held group, and the
// cursor's own group when the page did not read it again as an anchor.
func (rn *runner) shownBefore() (out []held, moved []bool) {
	cur := rn.groupCur
	if cur == nil {
		return nil, nil
	}
	for _, h := range cur.Held {
		out, moved = append(out, held{Group: h.Group, Values: slices.Clone(h.Values)}), append(moved, true)
	}
	if cur.Values != nil {
		out = append(out, held{Group: cur.Group, Values: slices.Clone(cur.Values)})
		moved = append(moved, rn.anchored < len(cur.Path))
	}
	return out, moved
}

// readShownNow gives each moved group of shownBefore the current sort values
// of the detail row its values end at (rn.shown). A group sorts elsewhere
// because a value its details share changed (a sort over the group's record),
// and the cursor holds the old one: compared with it, every detail of the
// group would sort before or after the cursor. A row that is gone or in
// another group now keeps the cursor's values. sel is the detail select, read
// over ctes, whose w holds every row: a held group may be no open leaf of this
// page, so a w cut to the page's leaves (leafCut) would not hold its rows.
func (rn *runner) readShownNow(s *stmt, ctes []string, sel string, keys []sortKey) error {
	shown, moved := rn.shownBefore()
	rn.shown = shown
	var ids []string
	at := map[string]int{}
	for _, m := range movedShown(shown, moved) {
		ids = append(ids, s.bind(m.id))
		at[m.id] = m.at
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := rn.query(s, ctes, fmt.Sprintf("SELECT * FROM (%s) d WHERE d.__id IN (%s)", sel, strings.Join(ids, ", ")))
	if err != nil {
		return err
	}
	groups := len(rn.c.p.groups)
	for _, row := range rows {
		id, _ := row["__id"].(string)
		i, ok := at[id]
		if !ok || keyString(rn.rowKey(row, groups)) != keyString(shown[i].Group) {
			continue
		}
		for j, k := range keys {
			shown[i].Values[j] = row[k.name]
		}
	}
	return nil
}

// openGroups lists the open innermost groups in the order the report shows
// them (R9a): the group rows that an assembly with no detail rows marks
// expanded at the innermost depth. That assembly is thrown away, but an error
// it met (a percent that overflows) is the run's.
func (rn *runner) openGroups(groupRows []map[string]any) ([][]any, int, error) {
	rows, notices := rn.res.Rows, rn.res.Notices
	rn.res.Rows, rn.res.Notices = []Row{}, slices.Clone(notices)
	rn.assemble(groupRows, nil, nil)
	if rn.assembleErr != nil {
		return nil, 0, rn.assembleErr
	}
	var out [][]any
	for _, r := range rn.res.Rows {
		if r.Kind == KindGroup && r.Expanded && r.Level == len(rn.c.p.groups) {
			out = append(out, r.GroupKey)
		}
	}
	shown := len(rn.res.Rows)
	rn.res.Rows, rn.res.Notices = rows, notices
	return out, shown, nil
}

// pastRefusal is true when a run asked to stop past refuseOver rows
// (Request.refuseOver) certainly shows more: the rows of its group statement
// as assembled before the details (shown: group, opening, subtotal and Total
// rows), and the base rows its open innermost groups hold (their __n), each
// of which is a detail row under its group. A run with a cursor reads anchors
// again, so it never stops here.
func (rn *runner) pastRefusal(groupRows []map[string]any, leaves [][]any, shown int) bool {
	if rn.refuseOver <= 0 || rn.opts.Cursor != "" {
		return false
	}
	groups := len(rn.c.p.groups)
	open := make(map[string]bool, len(leaves))
	for _, key := range leaves {
		open[keyString(key)] = true
	}
	certain := shown
	for _, row := range groupRows {
		if row["__row"] != nil || toInt(row["__level"]) != int64(groups) {
			continue
		}
		if key := keyString(rn.rowKey(row, groups)); open[key] {
			delete(open, key)
			certain += int(toInt(row["__n"]))
		}
	}
	return certain > rn.refuseOver
}

// skips is true when group level n (1 for the first) passes its blank keys up.
func (c *compiler) skips(n int) bool {
	return n >= 1 && n <= len(c.p.groups) && c.p.groups[n-1].group.Blank == GroupBlankSkip
}

// inlines is true when group level n (1 for the first) has an inline
// condition: its groups that meet it pass their rows up.
func (c *compiler) inlines(n int) bool {
	return n >= 1 && n <= len(c.p.groups) && c.p.groups[n-1].group.Inline != nil
}

// inlineLevels is one flag per group level, "i" for a level with an inline
// condition and "-" for one without, or "" when no level has one: a group
// cursor's fingerprint holds it, as an inlined group draws no row and moves
// its children up.
func (c *compiler) inlineLevels() string {
	flags := make([]byte, len(c.p.groups))
	found := false
	for i := range c.p.groups {
		flags[i] = '-'
		if c.inlines(i + 1) {
			flags[i], found = 'i', true
		}
	}
	if !found {
		return ""
	}
	return string(flags)
}

// inlinedRow is true for a row (its keys under prefix) whose level-n group is
// inlined: one of il<n> with the row's first n group keys and its currency
// keys whose __inline is 1. The keys compare as JSON arrays, so a blank key
// matches a blank one as IS matches it, and an unqualified key (prefix "")
// still reads the outer row.
func (c *compiler) inlinedRow(n int, prefix string) string {
	outer := make([]string, 0, n+len(c.currencies))
	inner := make([]string, 0, n+len(c.currencies))
	for i := range n {
		outer = append(outer, fmt.Sprintf("%sk%d", prefix, i))
		inner = append(inner, fmt.Sprintf("k%d", i))
	}
	for _, ck := range c.currencies {
		outer = append(outer, prefix+ck)
		inner = append(inner, ck)
	}
	return fmt.Sprintf("(json_array(%s) IN (SELECT json_array(%s) FROM il%d WHERE __inline = 1))",
		strings.Join(outer, ", "), strings.Join(inner, ", "), n)
}

// groupLevels names the report's group levels (column, bucket and blank) for
// a grouped cursor's fingerprint: a cursor from another grouping is refused,
// and a skip changes the row tree as much as a bucket does.
func (c *compiler) groupLevels() string {
	parts := make([]string, len(c.p.groups))
	for i, g := range c.p.groups {
		parts[i] = g.col.col.ID + "/" + g.group.Bucket + "/" + g.group.Blank
	}
	return strings.Join(parts, ";")
}

// leafOrder is the CTE gord: one row per open innermost group, __g its place
// in document order (json_each's index), g<j> its key values (group keys,
// then currency keys), bound as one JSON list as expansion.where binds keys.
// It is MATERIALIZED: read inline, the detail statement's time grows with
// rows x open groups (4,000 open groups: 6.7 s inline, 40 ms materialized).
func (c *compiler) leafOrder(s *stmt, leaves [][]any) string {
	encoded := make([]string, len(leaves))
	for i, key := range leaves {
		encoded[i] = keyString(key)
	}
	cols := []string{"key AS __g"}
	for j := range len(c.p.groups) + len(c.currencies) {
		cols = append(cols, fmt.Sprintf("json_extract(value, '$[%d]') AS g%d", j, j))
	}
	return fmt.Sprintf("gord AS MATERIALIZED (SELECT %s FROM json_each(%s))", strings.Join(cols, ", "), s.bind("["+strings.Join(encoded, ",")+"]"))
}

// leafJoin matches a detail row to its open group by each key value with IS,
// so a blank key matches a blank one, as a group row's key reads. row is what
// stands before a key's name on the row's side: "d." or "+d.".
func (c *compiler) leafJoin(row string) string {
	on := make([]string, 0, len(c.p.groups)+len(c.currencies))
	for i := range c.p.groups {
		on = append(on, fmt.Sprintf("%sk%d IS gord.g%d", row, i, i))
	}
	for j, ck := range c.currencies {
		on = append(on, fmt.Sprintf("%s%s IS gord.g%d", row, ck, len(c.p.groups)+j))
	}
	return strings.Join(on, " AND ")
}

// leafFrom is the detail rows (sel) with each row's open group: __g, the
// group's place in document order. The rows are read once, and each finds its
// group through the index SQLite builds on gord (an automatic index):
//   - CROSS JOIN fixes the join order in SQLite. Left to the planner, the
//     open groups were read first, and a key that is not a stored column of
//     the rows (a link's id, read from the joined row) is in no index of the
//     rows: every open group read every row.
//   - The unary + takes the column affinity off a key. A term that compares
//     with a column's affinity cannot use an index on gord, whose values (out
//     of json_extract) have none. A key and its group's value have the same
//     storage class, so the comparison gives the same answer without it.
func (c *compiler) leafFrom(sel string) string {
	return fmt.Sprintf("(SELECT d.*, gord.__g AS __g FROM (%s) d %s) d", sel, c.leafOn("d"))
}

// leafOn joins the rows read as alias to their open group, as leafFrom
// explains.
func (c *compiler) leafOn(alias string) string {
	join, on := c.leafTerms(alias)
	return join + " gord ON " + on
}

// leafTerms are the join and the condition of leafOn.
func (c *compiler) leafTerms(alias string) (join, on string) {
	join, row := "CROSS JOIN", "+"+alias+"."
	if legacySQL.plannedLeafJoin {
		join, row = "JOIN", alias+"."
	}
	return join, c.leafJoin(row)
}

// leafRows is the CTEs of the rows a page's detail statement computes: wl,
// the rows of table under the open groups the page can reach. A page shows
// its rows in the groups' order and holds at most want of them, so it ends in
// the first group at which the groups up to it hold want rows (gcut, NULL
// when all of them hold fewer: every group is read). The rows of the groups
// after it would be sorted, and their running values computed, to be thrown
// away. A group in held (its place in gord) shows only its rows after a
// cursor: its rows count as none, and it is read wherever it is, so the rows
// readShownNow asks for are there.
//
// A group the cursor holds that is not a leaf of the page (it moved after
// the page's groups) has no rows in gord. readShownNow still reads its shown
// row (an id in away) for the sort values the next cursor holds, so wl keeps
// that row too: the rows are joined to gord with a LEFT JOIN, and a row with
// no open group is kept by its id. The page's own join to gord leaves it
// out of the page.
//
// What the select reads of other rows is the same over wl: a running value
// and a percent of the parent read the rows of the row's own group, all of
// which wl holds, and a keyed layer reads its group keys. A sort key is no
// running value (notASortKey), so a row kept by its id alone has its own
// sort values. A percent of the whole report reads every group's rows, and a
// probed condition is probed over every open group: neither select is cut
// (runner.reachCut).
func (c *compiler) leafRows(s *stmt, table string, held []int, away []string, want int) []string {
	on := c.leafOn("w")
	counted, kept := "", ""
	if len(held) > 0 {
		places := make([]string, len(held))
		for i, at := range held {
			places[i] = s.bind(int64(at))
		}
		in := strings.Join(places, ", ")
		counted, kept = " WHERE gord.__g NOT IN ("+in+")", " OR gord.__g IN ("+in+")"
	}
	rowsOn := on
	if len(away) > 0 {
		ids := make([]string, len(away))
		for i, id := range away {
			ids[i] = s.bind(id)
		}
		_, cond := c.leafTerms("w")
		rowsOn = "LEFT JOIN gord ON " + cond
		kept += " OR (gord.__g IS NULL AND w.__id IN (" + strings.Join(ids, ", ") + "))"
	}
	return []string{
		// The integer LIMIT literal of limitSQL is approved by the human
		// (2026-11-21).
		fmt.Sprintf("gcut AS MATERIALIZED (SELECT MIN(c.__g) AS __g FROM (SELECT n.__g AS __g, SUM(n.n) OVER (ORDER BY n.__g ROWS UNBOUNDED PRECEDING) AS seen FROM (SELECT gord.__g AS __g, COUNT(*) AS n FROM %s w %s%s GROUP BY gord.__g) n) c WHERE c.seen >= %s)",
			table, on, counted, limitSQL(int64(want))),
		fmt.Sprintf("wl AS MATERIALIZED (SELECT w.* FROM %s w %s WHERE gord.__g <= COALESCE((SELECT __g FROM gcut), gord.__g)%s)", table, rowsOn, kept),
	}
}

// heldLeaves are the places in leaves of the groups whose first details
// earlier pages showed (shownBefore), and away the ids of the shown rows
// readShownNow reads of such groups that are not leaves.
func (rn *runner) heldLeaves(leaves [][]any) (held []int, away []string) {
	shown, moved := rn.shownBefore()
	movedAt := map[int]string{}
	for _, m := range movedShown(shown, moved) {
		movedAt[m.at] = m.id
	}
	for i, h := range shown {
		if at := slices.IndexFunc(leaves, func(k []any) bool { return keyString(k) == keyString(h.Group) }); at >= 0 {
			held = append(held, at)
		} else if id, ok := movedAt[i]; ok {
			away = append(away, id)
		}
	}
	return held, away
}

// movedShownRow is the shown row of a moved group: its place in shownBefore's
// list and the id its values end at.
type movedShownRow struct {
	at int
	id string
}

// movedShown is the one rule that names the shown rows read by id: those of
// the moved groups of shownBefore. readShownNow reads them and heldLeaves
// keeps them in wl, so a change here moves both.
func movedShown(shown []held, moved []bool) []movedShownRow {
	var out []movedShownRow
	for i, h := range shown {
		if moved[i] {
			id, _ := h.Values[len(h.Values)-1].(string)
			out = append(out, movedShownRow{at: i, id: id})
		}
	}
	return out
}

// groupedAfter selects the detail rows after a grouped report's cursor: the
// rows of a later open group, or of the cursor's group after its sort values
// (R9c), and returns the cursor's group, which the page starts after. A
// cursor whose group no longer opens is refused; the client then reads from
// page 1.
func groupedAfter(s *stmt, levels string, keys []sortKey, text string, leaves [][]any) (string, []any, error) {
	if text == "" {
		return "1", nil, nil
	}
	cur, err := readCursor(levels, keys, text)
	if err != nil {
		return "", nil, err
	}
	want := keyString(cur.Group)
	at := slices.IndexFunc(leaves, func(k []any) bool { return keyString(k) == want })
	if at < 0 {
		return "", nil, badCursor()
	}
	g := s.bind(int64(at))
	return fmt.Sprintf("(d.__g > %s OR (d.__g = %s AND %s))", g, g, keysetCond(s, keys, cur.Values)), cur.Group, nil
}

// shownOrder is what a select's value (or a mix of a select's values and
// other text) sorts by first: the label a row shows, as group rows of it
// read (levels.go shownKey), so one column has one order on every row; the
// stored value follows it. ok is false for any other value.
func shownOrder(bind func(any) string, e expr) (string, bool) {
	if e.shown == "" && len(e.options) == 0 {
		return "", false
	}
	return showSQL(bind, e), true
}

// detailSortValue is a sort key's value on a detail row, and the name its
// cursor fingerprint records: the column's id, or the formula itself, so a
// cursor from another sort is refused.
func detailSortValue(ctx *rowsCtx, s sortPlan) (expr, string, error) {
	var e expr
	var err error
	key := ""
	if s.formula != nil {
		e, err = ctx.formula(s.formula, s.at)
		raw, _ := json.Marshal(s.formula)
		key = "=" + string(raw)
	} else {
		e, err = ctx.col(s.col.col.ID, s.at)
		key = s.col.col.ID
	}
	if err != nil {
		return expr{}, "", err
	}
	if e.win {
		return expr{}, "", notASortKey(s.at)
	}
	return e, key, nil
}

// detailSelect is the select of detail rows over table (w, or ws: w's rows
// under shown groups) and their sort keys. table is read as w.
func (c *compiler) detailSelect(r *rowsCTE, table string) (string, []sortKey, error) {
	keys := make([]string, 0)
	for i := range c.p.groups {
		keys = append(keys, fmt.Sprintf("w.k%d", i))
	}
	frame := &windowFrame{
		parent: append(prefixed("w.", c.currencies), keys...),
		total:  prefixed("w.", c.currencies),
		recs:   newRecordJoins(r.s, "w"),
		paged:  !legacySQL.cellsBeforeThePage,
	}
	visiting := map[string]bool{}
	labels := newLabelRefs(c, r.s, frame, "w")
	c.detailWheres = nil
	var order []sortKey
	sel := []string{"w.__id AS __id"}
	// early names the values of sel that are read before the page is cut:
	// the sort keys and the probe marks.
	var early []earlyCol
	before := func(sql, name string) {
		early = append(early, earlyCol{at: len(sel), name: name})
		sel = append(sel, sql+" AS "+name)
	}
	for _, k := range append(keys, prefixed("w.", c.currencies)...) {
		sel = append(sel, k+" AS "+strings.TrimPrefix(k, "w."))
	}
	// addKey selects one sort key's values as name and orders by them. A spec
	// key also orders the window frame; a viewer key does not, so running
	// values keep the report's order (R7), and its cursor column is view:<col>.
	addKey := func(s sortPlan, name string, spec bool) error {
		e, col, err := detailSortValue(&rowsCtx{r: r, visiting: visiting, detail: frame}, s)
		if err != nil {
			return err
		}
		inFrame := func(parts ...string) {
			if spec {
				frame.order = append(frame.order, parts...)
			}
		}
		if !spec {
			col = "view:" + col
		}
		// A record sorts by its label's typed value, then by its id so equal
		// labels keep one order. A label key holds a value of the label
		// field's type or a no-name record's text, so its cursor value may be
		// either (typ "").
		if ref, ok := c.sortLabel(labels, s, e); ok {
			key := labelOrder(r.s, ref.value)
			before(key, name)
			order = append(order, sortKey{name: name, desc: s.desc, col: col + "@label"})
			inFrame("("+key+" IS NULL)", key+dirOf(s.desc))
			name = name + "_id"
		} else if key, ok := shownOrder(r.s.bind, e); ok {
			before(key, name)
			order = append(order, sortKey{name: name, desc: s.desc, col: col + "@shown", typ: Text})
			inFrame("("+key+" IS NULL)", key+dirOf(s.desc))
			name = name + "_value"
		}
		before(e.sql, name)
		order = append(order, sortKey{name: name, desc: s.desc, col: col, typ: e.typ})
		inFrame("("+e.sql+" IS NULL)", e.sql+dirOf(s.desc))
		// A spec key over a sum that met two currencies sorts as blank there;
		// a viewer key sorts by a column, whose blank cell is named already.
		if m := e.blankFor(); spec && m != "" {
			mx := fmt.Sprintf("__smx%d", len(c.detailSortMixed))
			before(fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE 0 END)", m), mx)
			c.detailSortMixed = append(c.detailSortMixed, condProbe{at: s.at, sql: mx})
		}
		return nil
	}
	c.detailSortMixed = nil
	for i, s := range c.viewSort {
		if err := addKey(s, fmt.Sprintf("vs%d", i), false); err != nil {
			return "", nil, err
		}
	}
	for i, s := range c.p.sort {
		if err := addKey(s, fmt.Sprintf("s%d", i), true); err != nil {
			return "", nil, err
		}
	}
	frame.order = append(frame.order, "w.__id")
	order = append(order, sortKey{name: "__id"})
	c.detailTypes, c.formulaOptions, c.sortable = map[string]FieldType{}, map[string][]Option{}, map[string]bool{}
	for idx, cp := range c.p.cols {
		if cp.nest > 0 {
			// A child row column's label is read with its child rows.
			if err := c.checkLabelField(cp, cp.path.target); err != nil {
				return "", nil, err
			}
			sel = append(sel, fmt.Sprintf("NULL AS v%d", idx), noCarriedCell(idx), noMixedCell(idx))
			sel = append(sel, labelRef{}.cells(idx)...)
			c.detailTypes[cp.col.ID] = cp.path.field.Type
			continue
		}
		// A column blank on detail rows shows no value there (detailCells): it
		// compiles with blank set, so its running values and percents are NULL
		// and add no window to the frame, and they turn off no cut (leafCut,
		// leafRows). Its mixed cell and its lookups' where probes stay, as the
		// mixedCurrency notice reads them on the rows shown. When one of those
		// reads a running value or a percent, the column compiles again on the
		// frame, so the notice reads the real value (and the cut is off). It
		// still gives the column's type, sortability, currency and options.
		blank := cp.col.Detail == DetailBlank
		wheres := len(c.detailWheres)
		ctx := &rowsCtx{r: r, visiting: visiting, detail: frame, blank: blank}
		e, err := ctx.col(cp.col.ID, cp.at)
		if err == nil && ctx.nulled && (e.blankFor() != "" || len(c.detailWheres) > wheres) {
			c.detailWheres = c.detailWheres[:wheres]
			e, err = (&rowsCtx{r: r, visiting: visiting, detail: frame}).col(cp.col.ID, cp.at)
		}
		if err != nil {
			return "", nil, err
		}
		if err := c.checkLabelField(cp, e.target); err != nil {
			return "", nil, err
		}
		if blank {
			c.markCarried(cp, e)
			sel = append(sel, fmt.Sprintf("NULL AS v%d", idx), noCarriedCell(idx), mixedCell(e, idx))
			sel = append(sel, labelRef{}.cells(idx)...)
		} else {
			ref, _ := c.columnLabel(labels, cp, e)
			sel = append(sel, fmt.Sprintf("%s AS v%d", cellSQL(e), idx), c.carriedCell(r.s, cp, e, idx), mixedCell(e, idx))
			sel = append(sel, ref.cells(idx)...)
		}
		c.detailTypes[cp.col.ID] = e.typ
		c.sortable[cp.col.ID] = !e.win
		if cp.path == nil {
			// A formula's amount has the currency of the amounts it reads.
			c.currencyOf[cp.col.ID] = e.cur
			c.formulaOptions[cp.col.ID] = e.options
		}
	}
	if d := c.p.spec.Detail; d != nil && d.Label != nil {
		ref, win, err := c.compileDetailLabel(&rowsCtx{r: r, visiting: visiting, detail: frame}, labels, d.Label)
		if err != nil {
			return "", nil, err
		}
		sel = append(sel, ref.value+" AS __label", ref.currency+" AS __labelcur")
		// The first column sorts by the label: sortable when the label is.
		c.sortable[c.p.cols[0].col.ID] = !win
	}
	if d := c.p.spec.Detail; d != nil && d.Open != nil {
		e, err := compileFormula(&rowsCtx{r: r, visiting: visiting, detail: frame, idText: true}, d.Open, "detail.open")
		if err != nil {
			return "", nil, err
		}
		if e.typ != Text {
			return "", nil, invalid("detail.open", "open is a text: the path the row opens")
		}
		sel = append(sel, cellSQL(e)+" AS __open")
	}
	if len(c.p.nest) > 0 {
		sel = append(sel, r.nestParent()+" AS __n0")
	}
	// A lookup's where unknown on a row is probed over the rows the pages
	// show, as a sort key over a mixed sum is.
	for i, w := range c.detailWheres {
		mx := fmt.Sprintf("__wmx%d", i)
		before(fmt.Sprintf("(CASE WHEN %s THEN 1 ELSE 0 END)", w.sql), mx)
		c.detailSortMixed = append(c.detailSortMixed, condProbe{at: w.at, sql: mx})
	}
	rows, cells := pagedCells(frame, table, sel, early)
	c.detailCells = cells
	c.detailWholeReport = frame.wholeReport
	c.detailWin = frame.win
	return rows, order, nil
}

// The CTEs of a page's cells: the page cut, and the layers above the cut.
const (
	pageCTE  = "wpage"
	cellsCTE = "wcells"
)

// earlyCol is a value of the detail select that is read before the page is
// cut: its place in the select list and its name.
type earlyCol struct {
	at   int
	name string
}

// pagedCells writes the detail select (sel over table, read as w) so that a
// row's own lookups run for the rows of the page only. rows is the select the
// page is cut from: every row with its sort keys and probe marks (early), its
// running values and percents, and the hoisted layers one of those reads.
// cells writes the statement of a page cut from rows (page, ordered by by):
// the other layers and the cells, over the page's rows.
//
// cells is nil, and rows is the whole select, when no layer would move above
// the cut, or when a value read before the cut reads a running value (a
// lookup's where over one).
func pagedCells(frame *windowFrame, table string, sel []string, early []earlyCol) (rows string, cells func(page, by string) string) {
	whole := frame.inline("SELECT " + strings.Join(sel, ", ") + " FROM " + frame.from(table, "w"))
	if !frame.paged {
		return whole, nil
	}
	texts := slices.Clone(frame.windows)
	for _, e := range early {
		texts = append(texts, sel[e.at])
	}
	isEarly := frame.earlyLayers(texts)
	if !slices.Contains(isEarly, false) {
		return whole, nil
	}
	for i, h := range frame.hoisted {
		if isEarly[i] {
			texts = append(texts, h.cols...)
		}
	}
	if slices.ContainsFunc(texts, windowRef.MatchString) {
		return whole, nil
	}
	first, last := []string{"w.*"}, slices.Clone(sel)
	for _, e := range early {
		first = append(first, sel[e.at])
		last[e.at] = "w." + e.name + " AS " + e.name
	}
	for i, sql := range frame.windows {
		first = append(first, fmt.Sprintf("%s AS ww%d", sql, i))
	}
	rows = "SELECT " + strings.Join(first, ", ") + " FROM " +
		frame.layers(frame.rows(table, "w"), table+" w", "w", func(i int) bool { return isEarly[i] })
	return rows, func(page, by string) string {
		cols := last
		if strings.HasPrefix(by, "d.__g") {
			cols = append(slices.Clone(last), "w.__g AS __g")
		}
		from := frame.layers("(SELECT * FROM "+pageCTE+") w", table+" w", "w", func(i int) bool { return !isEarly[i] })
		// Both selects are materialized. SQLite flattens a layer select into
		// the select that reads it and writes a hoisted value's expression in
		// each place that reads it (the Split count twice, a beginning balance
		// in every cell), so the layers are a CTE of their own: each hoisted
		// value is one column, computed once for a page row. The page is a CTE
		// too, or SQLite flattens it (it has a LIMIT) into the layers and runs
		// the lookups for every row again. The CTEs sit in the body's select,
		// not in the statement's list, so materializeShared does not count
		// their reads of w and a detail page with no groups keeps w inline.
		return fmt.Sprintf("SELECT * FROM (WITH %s AS MATERIALIZED (%s), %s AS MATERIALIZED (SELECT * FROM %s) SELECT %s FROM %s w) d ORDER BY %s",
			pageCTE, page, cellsCTE, from, strings.Join(cols, ", "), cellsCTE, by)
	}
}

// carriedCell is the currency a column's value e carries on a row (a record's
// or a looked-up row's own, or an amount param's code: vcur<idx>), NULL when
// it carries none. A column that carries one has the cell <id>@currency.
func (c *compiler) carriedCell(s *stmt, cp *colPlan, e expr, idx int) string {
	if !c.markCarried(cp, e) {
		return noCarriedCell(idx)
	}
	return fmt.Sprintf("%s AS vcur%d", carriedSQL(s, e), idx)
}

// markCarried records that cp's value e carries a currency on a row, so the
// column has the cell <id>@currency; it is false when e carries none.
func (c *compiler) markCarried(cp *colPlan, e expr) bool {
	if e.cur.sql == "" && e.cur.param == "" {
		return false
	}
	c.carried[cp.col.ID] = true
	return true
}

// carriedSQL is the currency a value carries on a row (a record's or a
// looked-up row's own, or an amount param's code), "NULL" when it carries
// none.
func carriedSQL(s *stmt, e expr) string {
	switch {
	case e.cur.param != "":
		// An amount param's currency is its own posted code.
		return s.bind(e.cur.param)
	case e.cur.sql != "":
		return e.cur.sql
	}
	return "NULL"
}

func noCarriedCell(idx int) string { return fmt.Sprintf("NULL AS vcur%d", idx) }

// mixedCell is a column cell's helper vmx<idx>: 1 on a row where the cell is
// blank because a sum lookup it reads met amounts in two currencies
// (expr.blankFor), NULL elsewhere.
func mixedCell(e expr, idx int) string {
	blank := e.blankFor()
	if blank == "" {
		return noMixedCell(idx)
	}
	return fmt.Sprintf("(CASE WHEN %s THEN 1 END) AS vmx%d", blank, idx)
}

func noMixedCell(idx int) string { return fmt.Sprintf("NULL AS vmx%d", idx) }

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
	// open holds the levels (1 for the first group) that open because the
	// run said nothing.
	open map[int]bool
	// encoded holds each key's values as sqlJSON writes them, in the order
	// of keys.
	encoded [][]string
	// closed holds the folded keys (options.collapse) as keyString gives
	// them; closedKeys and closedEncoded are them as keys and encoded hold
	// the expanded ones.
	closed        map[string]bool
	closedKeys    [][]any
	closedEncoded [][]string
}

// parseKeys reads a list of group keys (options.expand or options.collapse):
// at most foldCap keys. The keys' values need no cap of their own: a read
// binds them as one JSON parameter (keysIn), a key longer than a group row's
// matches nothing, and the request body's size limit bounds what a caller
// can send.
func parseKeys(at string, keys [][]any) ([][]any, [][]string, map[string]bool, error) {
	if len(keys) > runLimits.foldCap {
		verb := "expands"
		if at == "options.collapse" {
			verb = "folds"
		}
		return nil, nil, nil, invalid(at, fmt.Sprintf("a run %s at most %d groups", verb, runLimits.foldCap))
	}
	norms, encs, set := make([][]any, 0, len(keys)), make([][]string, 0, len(keys)), map[string]bool{}
	for i, key := range keys {
		where := fmt.Sprintf("%s.%d", at, i)
		if len(key) == 0 {
			return nil, nil, nil, invalid(where, "a group key is a group row's groupKey")
		}
		norm := make([]any, len(key))
		encoded := make([]string, len(key))
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
				return nil, nil, nil, invalid(where, "a group key holds text, numbers, true, false or null")
			}
			text, err := sqlJSON(norm[j])
			if err != nil {
				return nil, nil, nil, invalid(where, "a group key holds text, numbers, true, false or null")
			}
			encoded[j] = text
		}
		norms, encs = append(norms, norm), append(encs, encoded)
		set[keyString(norm)] = true
	}
	return norms, encs, set, nil
}

// parseExpansion reads options.expand and options.collapse.
func parseExpansion(opts Options) (*expansion, error) {
	e := &expansion{all: opts.ExpandAll, open: map[int]bool{}}
	var err error
	if e.keys, e.encoded, e.set, err = parseKeys("options.expand", opts.Expand); err != nil {
		return nil, err
	}
	if e.closedKeys, e.closedEncoded, e.closed, err = parseKeys("options.collapse", opts.Collapse); err != nil {
		return nil, err
	}
	return e, nil
}

// openLevels opens the levels marked open, down to the first level that is
// not (a level under a closed ancestor shows nothing, so it is not fetched),
// unless the run says what opens: expandAll opens every group, and expand
// without collapse lists the open groups (expand: [] opens nothing). A run
// that sends collapse keeps the open levels (Ruling R2).
func openLevels(p *plan, opts Options, e *expansion) {
	if opts.ExpandAll || (opts.Expand != nil && opts.Collapse == nil) {
		return
	}
	for i, g := range p.groups {
		if !g.group.Open {
			break
		}
		e.open[i+1] = true
	}
}

// keysIn matches rows whose cols equal one of keys, value by value as IS
// compares them. A key's blank values match by IS NULL, and the rest by one
// IN over the keys with the same blanks, bound as one JSON parameter:
// "(k0, ck0) IN (SELECT …)". IN builds its lookup once, where a correlated
// lookup per row (EXISTS over the keys) takes time that grows with rows times
// keys. IN compares with =, which never matches NULL, hence the split by
// blanks. The json_extract columns carry no affinity, so the row's key keeps
// its own as it does in IS, and COLLATE BINARY keeps IS's collation: the key
// list is the left operand there. "0" when no key has len(cols) values. The
// term is never NULL (each IN leg compares only set values; blanks go through
// IS NULL), so NOT of it is safe.
func keysIn(s *stmt, cols []string, keys [][]any, encoded [][]string) string {
	// blanks marks a key's blank values ("10" is a set first value, a
	// blank second); rowsOf holds each pattern's keys, set values only.
	var blanks []string
	rowsOf := map[string][]string{}
	for i, key := range keys {
		if len(key) != len(cols) {
			continue
		}
		mask := make([]byte, len(key))
		var set []string
		for j, text := range encoded[i] {
			mask[j] = '0'
			// null is nil, or a NaN, which SQLite binds as NULL.
			if text == "null" {
				mask[j] = '1'
				continue
			}
			set = append(set, text)
		}
		if _, seen := rowsOf[string(mask)]; !seen {
			blanks = append(blanks, string(mask))
		}
		rowsOf[string(mask)] = append(rowsOf[string(mask)], "["+strings.Join(set, ",")+"]")
	}
	if len(blanks) == 0 {
		return "0"
	}
	terms := make([]string, len(blanks))
	for i, mask := range blanks {
		var conds, left, picks []string
		for j, col := range cols {
			if mask[j] == '1' {
				conds = append(conds, col+" IS NULL")
				continue
			}
			picks = append(picks, fmt.Sprintf("json_extract(value, '$[%d]')", len(left)))
			left = append(left, col+" COLLATE BINARY")
		}
		if len(left) > 0 {
			param := s.bind("[" + strings.Join(rowsOf[mask], ",") + "]")
			conds = append(conds, fmt.Sprintf("(%s) IN (SELECT %s FROM json_each(%s))",
				strings.Join(left, ", "), strings.Join(picks, ", "), param))
		}
		terms[i] = "(" + strings.Join(conds, " AND ") + ")"
	}
	return anyOf(terms)
}

// where matches rows whose first depth group keys and currency keys are an
// open group's: every group under expandAll or an open level, else an
// expanded key; never a folded key.
func (e *expansion) where(c *compiler, s *stmt, depth int, prefix string) string {
	return e.whereAt(c, s, depth, prefix, true)
}

// whereAt is where; with exact false it may also match a closed group of an
// inline level whose parent is open (it does not read which groups are
// inlined), for a fill's parent keys (mayShowUnder).
func (e *expansion) whereAt(c *compiler, s *stmt, depth int, prefix string, exact bool) string {
	own := e.whereOwn(c, s, depth, prefix)
	if c.skips(depth) {
		// A blank key at a skip level is never a row the reader opens or
		// folds: it is open exactly when the group above it is, so its rows
		// show under that group.
		parent := e.whereAt(c, s, depth-1, prefix, exact)
		own = fmt.Sprintf("(%s OR (%sk%d IS NULL AND %s))", own, prefix, depth-1, parent)
	}
	if c.inlines(depth) {
		// An inlined group is not a row the reader opens or folds either: its
		// rows show where it would, so a key of expand or collapse that names
		// it is not read. A first-level group sits under the Total, which is
		// always open.
		parent := "1"
		if depth > 1 {
			parent = e.whereAt(c, s, depth-1, prefix, exact)
		}
		if !exact {
			if own == "1" || parent == "1" {
				return "1"
			}
			return fmt.Sprintf("(%s OR %s)", own, parent)
		}
		own = fmt.Sprintf("(CASE WHEN %s THEN %s ELSE %s END)", c.inlinedRow(depth, prefix), parent, own)
	}
	return own
}

// whereOwn is where without the skip levels: the keys the reader opened and
// did not fold.
func (e *expansion) whereOwn(c *compiler, s *stmt, depth int, prefix string) string {
	cols := make([]string, 0, depth+len(c.currencies))
	for i := range depth {
		cols = append(cols, fmt.Sprintf("%sk%d", prefix, i))
	}
	for _, ck := range c.currencies {
		cols = append(cols, prefix+ck)
	}
	open := "1"
	if !e.all && !e.open[depth] {
		open = keysIn(s, cols, e.keys, e.encoded)
	}
	folded := keysIn(s, cols, e.closedKeys, e.closedEncoded)
	if folded == "0" {
		return open
	}
	return "(" + open + " AND NOT " + folded + ")"
}

// shownUnder is true for a row whose every enclosing group is open: the
// groups at depths 1..depth above it. A row under a folded grandparent
// shows nothing even when its parent's level is open.
func (e *expansion) shownUnder(c *compiler, s *stmt, depth int, prefix string) string {
	return e.shownUnderAt(c, s, depth, prefix, true)
}

// mayShowUnder is shownUnder that does not read which groups are inlined
// (whereAt, exact false): it also matches a row under a closed group of an
// inline level whose parent is open. A fill's parent keys read it: il<L> may
// read the sums of the level below, whose rows the fill's keyset makes, so
// reading il<L> there would be circular.
func (e *expansion) mayShowUnder(c *compiler, s *stmt, depth int, prefix string) string {
	return e.shownUnderAt(c, s, depth, prefix, false)
}

func (e *expansion) shownUnderAt(c *compiler, s *stmt, depth int, prefix string, exact bool) string {
	conds := []string{}
	for d := 1; d <= depth; d++ {
		if w := e.whereAt(c, s, d, prefix, exact); w != "1" {
			conds = append(conds, w)
		}
	}
	if len(conds) == 0 {
		return "1"
	}
	return strings.Join(conds, " AND ")
}

// anyOf joins conditions with OR as a balanced tree: SQLite's parser goes a
// level deeper per OR of a chain and refuses past 1,000, and the keys may
// hold more patterns of blanks than that.
func anyOf(conds []string) string {
	if len(conds) == 1 {
		return conds[0]
	}
	mid := len(conds) / 2
	return "(" + anyOf(conds[:mid]) + " OR " + anyOf(conds[mid:]) + ")"
}

// has is true when the level-n group with key opens.
func (e *expansion) has(c *compiler, key []any, n int) bool {
	if e.closed[keyString(key)] {
		return false
	}
	if e.all || e.open[n] || e.set[keyString(key)] {
		return true
	}
	// key is n group keys then the currency keys; parent drops key n-1.
	if c.skips(n) && key[n-1] == nil {
		parent := append(slices.Clone(key[:n-1]), key[n:]...)
		return e.has(c, parent, n-1)
	}
	return false
}

// compileDry compiles every statement of a plan without a database, so a
// formula, a filter or a layout that cannot compile fails Validate, not a
// run. Params are unset and the pivot has one value.
func compileDry(p *plan) error {
	return newCompiler(p, nil, nil, nil).compileAll()
}

// compileAll compiles every statement of c's plan, as compileDry.
func (c *compiler) compileAll() error {
	p := c.p
	s := newStmt(nil, nil)
	r, err := c.rows(s)
	if err != nil {
		return err
	}
	if _, _, err := c.detailSelect(r, "w"); err != nil {
		return err
	}
	var pivot []pivotValue
	if p.pivot != nil {
		// As many cells as Run compiles at the cap, so Validate and Run
		// refuse the same layouts.
		pivot = make([]pivotValue, p.pivot.max)
	}
	if _, _, err = c.groupStatement(s, r, &expansion{}, pivot, 1); err != nil {
		return err
	}
	if _, err := c.headings(s); err != nil {
		return err
	}
	if _, err := c.openingLabels(s); err != nil {
		return err
	}
	for _, d := range p.spec.Params {
		if op := p.options[d.ID]; op != nil {
			// As the picker reads them: with the group order.
			if _, _, err := c.rankedOptionsSQL(newStmt(nil, nil), op); err != nil {
				return err
			}
		}
	}
	return nil
}
