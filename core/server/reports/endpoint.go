package reports

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"

	validation "github.com/pocketbase/ozzo-validation/v4"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/inflector"

	"tinycld.org/core/logging"
)

var log = logging.ForPackage("reports")

// maxRunBody bounds a run request: a spec, its params and options.
const maxRunBody = 1 << 20

// Register binds the report engine. When the server starts (after
// migrations) it checks every registered source and logs each one it
// refuses, so a broken source never reaches a user. It serves
// POST /api/reports/run to signed-in users.
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
			refused := Refused(e.App)
			keys := make([]string, 0, len(refused))
			for key := range refused {
				keys = append(keys, key)
			}
			slices.Sort(keys)
			for _, key := range keys {
				log.Error("report source refused", "source", key, "reason", refused[key])
			}
		}
		e.Router.POST("/api/reports/run", handleRun).
			Bind(apis.RequireAuth("users"), apis.BodyLimit(maxRunBody))
		return e.Next()
	})
}

// runBody is a draft report run: a spec, never SQL.
type runBody struct {
	Spec    json.RawMessage            `json:"spec"`
	Params  map[string]json.RawMessage `json:"params"`
	Options Options                    `json:"options"`
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
	if body.Spec == nil {
		return re.BadRequestError("The request has no spec.", nil)
	}
	spec, loaded, err := Load(body.Spec)
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
	return re.JSON(http.StatusOK, res)
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

// runError answers a failed run: 400 with each invalid part of the spec,
// 422 naming each unknown source, link, field or column (and an amount too
// large to total), 408 at the time limit, 499 when the client went away. Any
// other error is logged and answered with a fixed message, so SQL text
// never reaches the client.
func runError(re *core.RequestEvent, err error) error {
	var specErr *SpecError
	switch {
	case errors.As(err, &specErr) && specErr.Status == http.StatusUnprocessableEntity:
		return re.Error(http.StatusUnprocessableEntity, "The report names something that does not exist.", specErr.Problems)
	case errors.As(err, &specErr):
		return re.Error(http.StatusBadRequest, "The report is not valid.", specErr.Problems)
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
