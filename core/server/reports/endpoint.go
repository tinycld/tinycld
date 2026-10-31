package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"slices"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/inflector"

	"tinycld.org/core/changesignal"
	"tinycld.org/core/logging"
)

var log = logging.ForPackage("reports")

// maxRunBody bounds a run request: a spec, its params and options.
const maxRunBody = 1 << 20

// sortedKeys lists the keys of a refused-source map in a stable order, so
// the log and the validate command name the sources the same way each run.
func sortedKeys(refused map[string]string) []string {
	return slices.Sorted(maps.Keys(refused))
}

// Register binds the report engine. When the server starts (after
// migrations) it checks every registered source and logs each one it
// refuses, so a broken source never reaches a user; then it ships the
// packages' reports (ReconcileShipped) and marks every saved report that
// cannot run (checkSavedAtStart). It serves POST /api/reports/run to
// signed-in users: a draft spec, or a saved report the caller can view.
// Saved reports and their shares are bound by RegisterSharing.
//
// The run endpoint is a read endpoint by POST because a draft spec does not
// fit in a URL. It reads only rows the caller's list rules allow and
// writes nothing. An OAuth bearer token cannot call it: oauth classifies no
// scope for it, so it is session-only by default-deny, because a report
// reads across every package's collections and a token's scopes cover
// only some.
func Register(app core.App) {
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		if err := CheckSources(e.App); err != nil {
			refused := Refused()
			for _, key := range sortedKeys(refused) {
				log.Error("report source refused", "source", key, "reason", refused[key])
			}
		}
		if err := ReconcileShipped(e.App); err != nil {
			log.Error("shipped reports not reconciled", "err", err)
		}
		checkSavedAtStart(e.App)
		e.Router.POST("/api/reports/run", handleRun).
			Bind(apis.RequireAuth("users"), apis.BodyLimit(maxRunBody))
		return e.Next()
	})
}

// checkSavedAtStart refreshes every saved report's broken mark when the
// server starts, after the sources are checked: a package install or
// upgrade restarts the server, and that is when a source, link or field can
// disappear. In dev it also logs each broken report with its owner; in
// production one line says how many and names the command that lists them.
func checkSavedAtStart(app core.App) {
	res, err := CheckSaved(app)
	if err != nil {
		log.Error("saved reports not checked", "err", err)
		return
	}
	logBrokenAtStart(res, app.IsDev())
}

func logBrokenAtStart(res *CheckResult, dev bool) {
	if len(res.Broken) == 0 {
		return
	}
	if !dev {
		log.Info("saved reports are broken; run `tinycld reports validate` to list them", "broken", len(res.Broken), "checked", res.Checked)
		return
	}
	for _, b := range res.Broken {
		log.Warn("saved report is broken", "report", b.ID, "title", b.Title, "owner", ownerLabel(b.Owner), "problems", b.Broken.Problems)
	}
}

// runBody is one report run: a draft spec, or the id of a saved report the
// caller can view. Never SQL.
type runBody struct {
	Spec     json.RawMessage            `json:"spec"`
	ReportID string                     `json:"reportId"`
	Params   map[string]json.RawMessage `json:"params"`
	Options  Options                    `json:"options"`
}

func handleRun(re *core.RequestEvent) error {
	var body runBody
	dec := json.NewDecoder(re.Request.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		return re.BadRequestError("The request is not a report run: "+err.Error(), nil)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return re.BadRequestError("The request is not a report run: it has data after the first object.", nil)
	}
	raw := []byte(body.Spec)
	saved := body.ReportID != ""
	switch {
	case saved && body.Spec != nil:
		return re.BadRequestError("The request has a spec or a reportId, not both.", nil)
	case saved:
		var err error
		if raw, err = viewableSpec(re, body.ReportID); err != nil {
			return err
		}
	case body.Spec == nil:
		return re.BadRequestError("The request has no spec and no reportId.", nil)
	}
	spec, loaded, err := Load(raw)
	if err != nil {
		return runError(re, err)
	}
	res, err := Run(re.Request.Context(), re.App, Request{
		Spec: spec, Loaded: loaded, Params: body.Params, Options: body.Options,
		Auth: re.Auth, Headers: ruleHeaders(re.Request.Header),
	})
	if err != nil {
		return runError(re, err)
	}
	if saved {
		res.Topics = withSavedTopics(res.Topics)
	}
	return re.JSON(http.StatusOK, res)
}

const noAccess = "You cannot view this report."

// viewableSpec reads a saved report's spec for the caller, through the
// reports view rule as PocketBase's view API applies it. A report that does
// not exist answers as one the caller cannot view, so an id tells nobody
// whether a report exists. Any other failure, such as a database error or a
// rule that cannot be built, is a server fault: it is logged and answered
// 500, never as a refusal.
func viewableSpec(re *core.RequestEvent, id string) ([]byte, error) {
	ctx := re.Request.Context()
	col, err := re.App.FindCachedCollectionByNameOrId(reportsCollection)
	if err != nil {
		log.ErrorContext(ctx, "saved report run: collection lookup failed", "err", err)
		return nil, re.InternalServerError("The report could not run.", nil)
	}
	rec, err := re.App.FindRecordById(col, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, re.ForbiddenError(noAccess, fieldError("reportId", "no_access", noAccess))
	}
	if err != nil {
		log.ErrorContext(ctx, "saved report run: report lookup failed", "reportId", id, "err", err)
		return nil, re.InternalServerError("The report could not run.", nil)
	}
	ok, err := re.App.CanAccessRecord(rec, listRequestInfo(re.Auth, ruleHeaders(re.Request.Header)), col.ViewRule)
	if err != nil {
		log.ErrorContext(ctx, "saved report run: view rule failed", "reportId", id, "err", err)
		return nil, re.InternalServerError("The report could not run.", nil)
	}
	if !ok {
		return nil, re.ForbiddenError(noAccess, fieldError("reportId", "no_access", noAccess))
	}
	return specBytes(rec), nil
}

// withSavedTopics adds the saved-report collections to a run's topics, so
// the open report runs again when someone edits it or changes its shares,
// and learns when the reader loses access.
func withSavedTopics(topics []string) []string {
	for _, c := range []string{reportsCollection, sharesCollection} {
		if t := changesignal.Topic(c); !slices.Contains(topics, t) {
			topics = append(topics, t)
		}
	}
	slices.Sort(topics)
	return topics
}

// ruleHeaders are the headers as a rule's @request.headers reads them:
// the first value of each, its name in snake case ("X-Token" is x_token).
// They are built here, not by re.RequestInfo, which would read the body the
// handler has already decoded. A POST carries content_type and
// content_length headers that a list GET usually lacks, so a rule that reads
// those two can answer differently here than in a list.
func ruleHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		if len(v) > 0 {
			out[inflector.Snakecase(k)] = v[0]
		}
	}
	return out
}

// statusClientClosed answers a run whose client went away: nobody reads it,
// and it is not a server error.
const statusClientClosed = 499

// specStatus is how a refused spec is answered, by a run and by a save: 422
// when it names something that does not exist, else 400.
func specStatus(err *SpecError) (int, string) {
	if err.Status == http.StatusUnprocessableEntity {
		return http.StatusUnprocessableEntity, "The report names something that does not exist."
	}
	return http.StatusBadRequest, "The report is not valid."
}

// runError answers a failed run: 400 with each invalid part of the spec,
// 422 naming each unknown source, link, field or column (and an amount too
// large to total), 408 at the time limit, 499 when the client went away. Any
// other error is logged and answered with a fixed message, so SQL text
// never reaches the client.
func runError(re *core.RequestEvent, err error) error {
	var specErr *SpecError
	switch {
	case errors.As(err, &specErr):
		status, message := specStatus(specErr)
		return re.Error(status, message, specErr.Problems)
	case errors.Is(err, ErrTimeout):
		return re.Error(http.StatusRequestTimeout, "The report took too long to run.", nil)
	case errors.Is(err, ErrAmountTooLarge):
		return re.Error(http.StatusUnprocessableEntity, "An amount is too large to total.", map[string]validation.Error{
			"spec": validation.NewError("amount_too_large", "An amount in the report is too large to total."),
		})
	case errors.Is(err, ErrNoReader):
		return re.ForbiddenError("A report runs as a signed-in user.", nil)
	case errors.Is(err, context.Canceled) || re.Request.Context().Err() != nil:
		// The driver does not always wrap the cancel, so the request's own
		// context says whether the client left.
		log.DebugContext(re.Request.Context(), "report run canceled: the client went away", "err", err)
		return re.Error(statusClientClosed, "The request was canceled.", nil)
	}
	log.ErrorContext(re.Request.Context(), "report run failed", "err", err)
	return re.InternalServerError("The report could not run.", nil)
}
