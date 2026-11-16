package reports

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/router"

	"tinycld.org/core/audit"
	"tinycld.org/core/pdfrender"
)

// POST /api/reports/export answers a saved report as a file for a reader
// who can open it: the whole report, read page by page as that reader with
// the view's state, written by the format's writer and named by
// exportFileName. Every download writes one audit row. The route takes a
// signed-in session only (Register binds it as the run route); oauth
// classifies no scope for it, so an OAuth token is refused by default-deny.

// The refusals of an export. The limits are the spec's fixed sentences, never
// built from exportLimits, which tests shrink.
const (
	exportNoAccess    = "You no longer have access to this report."
	exportDisabled    = "Your account is disabled."
	exportBadFormat   = "The format is not pdf, xlsx or csv."
	exportSheetRows   = "This report has more than 50,000 rows. Narrow the period."
	exportTooLong     = "The report took too long to export."
	exportCannotWrite = "Could not create the file."
)

// exportLimits: a PDF holds at most pdfRows rows, a CSV or Excel file
// sheetRows; an export stops after timeout.
var exportLimits = struct {
	pdfRows, sheetRows int
	timeout            time.Duration
}{5000, 50000, 60 * time.Second}

// exportBody is one export: a saved report the caller can view, the run's
// params and options as the run endpoint takes them, the format, and the
// reader's clock. There is no spec field: a draft cannot be exported.
type exportBody struct {
	ReportID string                     `json:"reportId"`
	Params   map[string]json.RawMessage `json:"params"`
	Options  Options                    `json:"options"`
	Format   string                     `json:"format"`
	Hour12   bool                       `json:"hour12"`
}

// exportWriter writes one format: its file extension and type, the most
// rows it holds (read at call time), whether it opens every group, and the
// writer.
type exportWriter struct {
	Ext       string
	Mime      string
	MaxRows   func() int
	ExpandAll bool
	// TooMany is the refusal of a report past MaxRows.
	TooMany string
	Write   func(ctx context.Context, d *exportData) ([]byte, error)
}

// exportWriters are the formats an export writes, by the body's format.
var exportWriters = map[string]exportWriter{
	"csv": {
		Ext: "csv", Mime: "text/csv; charset=utf-8",
		MaxRows:   func() int { return exportLimits.sheetRows },
		ExpandAll: true, TooMany: exportSheetRows, Write: writeCSV,
	},
}

// exportReader is the reader's clock: the zone (an IANA name), the hour
// cycle and the reader's day (YYYY-MM-DD).
type exportReader struct {
	Zone   string
	Hour12 bool
	Today  string
}

// exportRow is one row of the file: the row and its cells after the view's
// rule that a figure shows once (D7, visibleRows in rows.ts).
type exportRow struct {
	Row   Row
	Cells map[string]any
}

// exportData is the whole report, ready to write.
type exportData struct {
	Title    string
	ReportID string
	// Heading is the source's heading; "" for the organization's name.
	Heading string
	// Columns are the shown columns, the label column first.
	Columns []ResultColumn
	// Rows are empty for a report with no detail and no group row (D8).
	Rows    []exportRow
	Params  []ResolvedParam
	Defs    []ParamDef
	Notices []Notice
	// ParamLabels are the shown labels of each choice param from a source,
	// by param id, keyed as the client's ChoiceLabels, for ParamsLine.
	ParamLabels map[string]map[string]string
	Reader      exportReader
	Display     exportDisplay
	Format      *exportFormatter
	Made        time.Time
}

// exportFile is a written file and what its audit row names.
type exportFile struct {
	Name     string
	Mime     string
	Bytes    []byte
	Rows     int
	Title    string
	ReportID string
	Format   string
}

func handleExport(re *core.RequestEvent) error {
	if re.Auth.GetBool("disabled") {
		return re.ForbiddenError(exportDisabled, nil)
	}
	var body exportBody
	if err := decodeStrict(re, &body, "a report export"); err != nil {
		return err
	}
	file, err := buildExport(re, body)
	if err != nil {
		return err
	}
	if err := writeExportAudit(re, "report.export", file, nil); err != nil {
		log.ErrorContext(re.Request.Context(), "report export: audit row not written", "reportId", file.ReportID, "err", err)
		return re.InternalServerError(exportCannotWrite, nil)
	}
	ext := exportWriters[file.Format].Ext
	re.Response.Header().Set("Content-Disposition", pdfrender.ContentDisposition(strings.TrimSuffix(file.Name, "."+ext), ext))
	re.Response.Header().Set("Access-Control-Expose-Headers", "Content-Disposition")
	re.Response.Header().Set("Cache-Control", "no-store")
	return re.Blob(http.StatusOK, file.Mime, file.Bytes)
}

// writeExportAudit writes the one audit row of a download (report.export) or
// a sent email (report.email): the report, the format, the rows the file
// holds, and extra (an email's recipients).
func writeExportAudit(re *core.RequestEvent, action string, file *exportFile, extra map[string]any) error {
	metadata := map[string]any{"format": file.Format, "rows": file.Rows}
	for k, v := range extra {
		metadata[k] = v
	}
	return audit.Log(re.App, action, reportsCollection, file.ReportID, file.Title, re, metadata)
}

// buildExport reads the report the body names as the caller and writes it
// in the body's format. A refusal is an error to answer as it is.
func buildExport(re *core.RequestEvent, body exportBody) (*exportFile, error) {
	writer, ok := exportWriters[body.Format]
	if !ok {
		return nil, re.BadRequestError(exportBadFormat, fieldError("format", "invalid_format", exportBadFormat))
	}
	if body.ReportID == "" {
		return nil, re.BadRequestError("The request has no reportId.", fieldError("reportId", "required", "The report to export is required."))
	}
	rec, err := viewableReport(re, body.ReportID)
	if err != nil {
		var apiErr *router.ApiError
		if errors.As(err, &apiErr) && apiErr.Status == http.StatusForbidden {
			return nil, re.ForbiddenError(exportNoAccess, fieldError("reportId", "no_access", exportNoAccess))
		}
		return nil, err
	}
	spec, loaded, err := Load(specBytes(rec))
	if err != nil {
		return nil, runError(re, err)
	}
	display, err := exportDisplayOf(body.Options.Display)
	if err != nil {
		return nil, re.BadRequestError("The display options are not valid.", fieldError("options.display", "invalid", "The display options are not valid."))
	}
	ctx, cancel := context.WithTimeout(re.Request.Context(), exportLimits.timeout)
	defer cancel()

	res, err := readWholeReport(ctx, re, spec, loaded, body, writer)
	if err != nil {
		return nil, exportRunError(re, ctx, err)
	}
	if res == nil {
		return nil, re.Error(http.StatusUnprocessableEntity, writer.TooMany, nil)
	}
	labels, err := readParamLabels(ctx, re, spec, body, res.Params)
	if err != nil {
		return nil, exportRunError(re, ctx, err)
	}
	if ctx.Err() != nil {
		return nil, exportRunError(re, ctx, ctx.Err())
	}
	loc := readerZone(body.Options.TimeZone)
	day, err := todayOf(body.Options.Today, "options.today", loc)
	if err != nil {
		return nil, runError(re, err)
	}
	today := day.Format(time.DateOnly)
	data := &exportData{
		Title:       rec.GetString("title"),
		ReportID:    rec.Id,
		Heading:     res.Heading,
		Columns:     shownExportColumns(res.Columns, display.Hidden),
		Rows:        exportRows(res.Rows, spec),
		Params:      res.Params,
		Defs:        spec.Params,
		Notices:     res.Notices,
		ParamLabels: labels,
		Reader:      exportReader{Zone: body.Options.TimeZone, Hour12: body.Hour12, Today: today},
		Display:     display,
		Format:      newExportFormatter(display, body.Options.TimeZone, body.Hour12),
		Made:        time.Now(),
	}
	bytes, err := writer.Write(ctx, data)
	if err != nil {
		log.ErrorContext(re.Request.Context(), "report export: file not written", "reportId", rec.Id, "format", body.Format, "err", err)
		return nil, re.InternalServerError(exportCannotWrite, nil)
	}
	return &exportFile{
		Name:     exportFileName(data.Title, spec.Params, res.Params, today, writer.Ext),
		Mime:     writer.Mime,
		Bytes:    bytes,
		Rows:     len(data.Rows),
		Title:    data.Title,
		ReportID: rec.Id,
		Format:   body.Format,
	}, nil
}

// exportRunError answers a failed read: a deadline is the export's own
// refusal; any other error is the run's (runError).
func exportRunError(re *core.RequestEvent, ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, ErrTimeout) {
		return re.Error(http.StatusRequestTimeout, exportTooLong, nil)
	}
	return runError(re, err)
}

// exportDisplayOf reads the view's display options; a request with none has
// the defaults (DEFAULT_DISPLAY).
func exportDisplayOf(raw json.RawMessage) (exportDisplay, error) {
	display := defaultDisplay
	display.Hidden = []string{}
	if len(raw) == 0 || string(raw) == "null" {
		return display, nil
	}
	if err := json.Unmarshal(raw, &display); err != nil {
		return exportDisplay{}, err
	}
	return display, nil
}

// readWholeReport runs the report page by page as the reader and joins the
// pages as mergePages (rows.ts) joins them. It returns nil with no error when
// the report holds more rows than the writer takes, or when a cap stopped
// the run, so no file is ever cut short.
func readWholeReport(ctx context.Context, re *core.RequestEvent, spec *Spec, loaded *LoadInfo, body exportBody, writer exportWriter) (*Result, error) {
	maxRows := writer.MaxRows()
	opts := body.Options
	opts.Limit = runLimits.maxPage
	opts.Cursor = ""
	if writer.ExpandAll {
		opts.ExpandAll = true
		opts.Expand, opts.Collapse = nil, nil
	}
	headers := ruleHeaders(re.Request.Header)
	var merged *Result
	rows := []Row{}
	seen := map[string]bool{}
	for {
		page, err := Run(ctx, re.App, Request{
			Spec: spec, Loaded: loaded, Params: body.Params, Options: opts,
			Auth: re.Auth, Headers: headers, RowCap: maxRows + 1,
		})
		if err != nil {
			return nil, err
		}
		rows = withoutAppendedTotals(rows, seen)
		for _, row := range page.Rows {
			if !seen[row.ID] {
				seen[row.ID] = true
				rows = append(rows, row)
			}
		}
		if merged == nil {
			merged = page
		} else {
			notices := append(merged.Notices, page.Notices...)
			merged = page
			merged.Notices = notices
		}
		if len(rows) > maxRows {
			return nil, nil
		}
		if page.Page == nil || page.Page.Next == "" {
			break
		}
		opts.Cursor = page.Page.Next
	}
	merged.Rows = rows
	merged.Notices = uniqueNotices(merged.Notices)
	capped := slices.ContainsFunc(merged.Notices, func(n Notice) bool { return n.Code == "rowCap" || n.Code == "nestCap" })
	if capped {
		return nil, nil
	}
	return merged, nil
}

// isSectionRow (rows.ts): a computed row inside a section belongs to it.
func isSectionRow(row Row) bool { return row.Kind == KindComputed && row.Section != "" }

// isWholeReportRow (rows.ts): a total, or a computed row of the whole report.
func isWholeReportRow(row Row) bool {
	return row.Kind == KindTotal || (row.Kind == KindComputed && !isSectionRow(row))
}

// withoutAppendedTotals (rows.ts) drops the whole-report rows a page appends
// after its slice: they repeat on every page, and only the newest page's
// figures are current. The dropped rows leave seen, so the next page's copy
// is kept.
func withoutAppendedTotals(rows []Row, seen map[string]bool) []Row {
	end := len(rows)
	for end > 0 && isWholeReportRow(rows[end-1]) {
		end--
		delete(seen, rows[end].ID)
	}
	return rows[:end]
}

// uniqueNotices (rows.ts): one notice per code, the notices of several pages
// joined.
func uniqueNotices(notices []Notice) []Notice {
	out := []Notice{}
	at := map[string]int{}
	for _, n := range notices {
		if i, ok := at[n.Code]; ok {
			out[i] = joinNotices(out[i], n)
			continue
		}
		at[n.Code] = len(out)
		out = append(out, n)
	}
	return out
}

// union (rows.ts) lists a then b, each item once.
func union(a, b []string) []string {
	out := []string{}
	for _, s := range append(slices.Clone(a), b...) {
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// sentencesOf (rows.ts): a notice's parts, or its message as one.
func sentencesOf(n Notice) []string {
	if n.Parts != nil {
		return n.Parts
	}
	return []string{n.Message}
}

// joinNotices (rows.ts): one code's notices of two pages, their columns,
// conditions and sentences each once.
func joinNotices(a, b Notice) Notice {
	parts := union(sentencesOf(a), sentencesOf(b))
	out := a
	out.Message = strings.Join(parts, " ")
	out.Parts = nil
	if len(parts) > 1 {
		out.Parts = parts
	}
	if a.Columns != nil || b.Columns != nil {
		out.Columns = union(a.Columns, b.Columns)
	}
	if a.Conditions != nil || b.Conditions != nil {
		out.Conditions = union(a.Conditions, b.Conditions)
	}
	return out
}

// shownExportColumns (shownColumns in format.ts): every column the reader
// did not hide; the first is never hidden.
func shownExportColumns(columns []ResultColumn, hidden []string) []ResultColumn {
	out := make([]ResultColumn, 0, len(columns))
	for i, c := range columns {
		if i == 0 || !slices.Contains(hidden, c.ID) {
			out = append(out, c)
		}
	}
	return out
}

// hasRows (rows.ts): a result with no detail and no group row is empty;
// totals alone do not count.
func hasRows(rows []Row) bool {
	return slices.ContainsFunc(rows, func(r Row) bool { return r.Kind == KindDetail || r.Kind == KindGroup })
}

// exportRows is visibleRows (rows.ts) over the joined rows: a group row that
// its own subtotal row follows has no figures, a detail row's label cell is
// not repeated, and a section's computed row under a folded section is left
// out. An empty report has no rows (D8).
func exportRows(rows []Row, spec *Spec) []exportRow {
	if !hasRows(rows) {
		return []exportRow{}
	}
	withSubtotal := map[string]bool{}
	folded := map[string]bool{}
	for _, r := range rows {
		if r.Kind == KindSubtotal && r.Section != "" {
			withSubtotal[r.Section] = true
		}
		if r.Kind == KindGroup && !r.Expanded {
			folded[r.ID] = true
		}
	}
	out := make([]exportRow, 0, len(rows))
	for _, r := range rows {
		if isSectionRow(r) && folded[r.Section] {
			continue
		}
		out = append(out, exportRow{Row: r, Cells: exportRowCells(r, withSubtotal, spec)})
	}
	return out
}

// endsInSubtotal (rows.ts): the engine ends this group row with its own
// subtotal row: an open group of a level with subtotals; in a layout report
// every group of such a level, and every layout section (level 1).
func endsInSubtotal(row Row, spec *Spec) bool {
	isLayout := len(spec.Layout) > 0
	level := row.Level
	if level == 0 {
		level = row.Depth + 1
	}
	hasSubtotals := (isLayout && level == 1) || (level-1 < len(spec.Groups) && spec.Groups[level-1].Subtotals)
	return hasSubtotals && (row.Expanded || isLayout)
}

// exportRowCells is rowCells (rows.ts).
func exportRowCells(row Row, withSubtotal map[string]bool, spec *Spec) map[string]any {
	switch {
	case row.Kind == KindGroup && (withSubtotal[row.ID] || endsInSubtotal(row, spec)):
		return map[string]any{}
	case row.Kind == KindDetail && row.LabelColumn != "":
		cells := make(map[string]any, len(row.Cells))
		for id, v := range row.Cells {
			if id != row.LabelColumn {
				cells[id] = v
			}
		}
		return cells
	}
	return row.Cells
}

// readParamLabels reads the shown labels of each choice param from a source
// whose header text names its choices (at most namedChoices of them), as the
// chips read them: the values' labels, and each whole group's label.
func readParamLabels(ctx context.Context, re *core.RequestEvent, spec *Spec, body exportBody, resolved []ResolvedParam) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	headers := ruleHeaders(re.Request.Header)
	for _, def := range spec.Params {
		r := resolvedOf(resolved, def.ID)
		if def.OptionsFrom == nil || r == nil || r.Unset || r.Type != ParamChoice {
			continue
		}
		values := choiceValues(*r)
		if len(values)+len(r.Groups) > namedChoices {
			continue
		}
		ask := OptionsRequest{
			Spec: spec, Param: def.ID, Params: body.Params, Today: body.Options.Today,
			TimeZone: body.Options.TimeZone, Auth: re.Auth, Headers: headers,
		}
		labels := map[string]string{}
		if len(values) > 0 {
			ask.Values = values
			page, err := ReadParamOptions(ctx, re.App, ask)
			if err != nil {
				return nil, err
			}
			for _, o := range page.Options {
				labels[o.Value] = optionText(o.Label, o.Type, o.Currency)
			}
		}
		if len(r.Groups) > 0 {
			ask.Values, ask.Limit = nil, 1
			page, err := ReadParamOptions(ctx, re.App, ask)
			if err != nil {
				return nil, err
			}
			for _, g := range page.Groups {
				labels[groupLabelKey(g.Value)] = optionText(g.Label, g.Type, g.Currency)
			}
		}
		out[def.ID] = labels
	}
	return out, nil
}

// optionText (param-options.ts): an option's or a group's label as shown, a
// typed label formatted as a cell of its type with the default display.
func optionText(label string, t FieldType, currency string) string {
	if t == "" {
		return label
	}
	var value any = label
	if t == Number || t == Money || t == Percent {
		n, err := strconv.ParseFloat(strings.TrimSpace(label), 64)
		if err != nil || !isFinite(n) {
			return label
		}
		value = n
	}
	return newExportFormatter(defaultDisplay, "", false).typedLabelText(t, value, currency)
}

func isFinite(n float64) bool { return n == n && n-n == 0 }

// maxFileNameRunes bounds the name part of an export's file name.
const maxFileNameRunes = 80

// exportFileName names an export's file: the report's title in lower case,
// words joined by "-", every character but letters, marks, digits and "-"
// removed, at most 80 characters ("report" when nothing is left); then the
// date part: the period ("2026-01-01-to-2026-11-15"), an as-of day
// ("as-of-2026-11-15"), a single day, or the reader's day (today) for a
// report with no date param, an unset one, or all dates. The download, the
// email attachment and the dialog's line all use this name.
func exportFileName(title string, defs []ParamDef, params []ResolvedParam, today, ext string) string {
	return fileNameStem(title) + "-" + fileNameDate(defs, params, today) + "." + ext
}

// fileNameStem is the name part of exportFileName.
func fileNameStem(title string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(title) {
		switch {
		case unicode.IsSpace(r) || r == '-':
			dash = true
		case unicode.IsLetter(r) || unicode.IsMark(r) || unicode.IsDigit(r):
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		}
	}
	stem := b.String()
	if utf8.RuneCountInString(stem) > maxFileNameRunes {
		stem = string([]rune(stem)[:maxFileNameRunes])
	}
	stem = strings.Trim(stem, "-")
	if stem == "" {
		return "report"
	}
	return stem
}

// fileNameDate is the date part of exportFileName.
func fileNameDate(defs []ParamDef, params []ResolvedParam, today string) string {
	period := firstDateParam(defs)
	if period == nil {
		return today
	}
	r := resolvedOf(params, period.ID)
	if r == nil || r.Unset {
		return today
	}
	day, _ := r.Value.(string)
	switch {
	case period.Type == ParamDateRange && r.From != "" && r.To != "":
		return r.From + "-to-" + r.To
	case period.Type == ParamAsOf && isDay(day):
		return "as-of-" + day
	case period.Type == ParamDate && isDay(day):
		return day
	}
	return today
}
