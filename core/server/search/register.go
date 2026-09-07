package search

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/oauth"
)

// errPanic turns a recovered value into an error so a panicking source is
// reported like any other failure rather than crashing the request.
func errPanic(r any) error {
	return fmt.Errorf("panic: %v", r)
}

// Register binds GET /api/search.
//
// One endpoint for every package, rather than the per-package routes the
// palette used to call directly: the fan-out happens in-process here, so the
// normalized row shape is defined once and both the web palette and the CLI
// consume it. The per-package routes stay for their own callers.
func Register(app *pocketbase.PocketBase) {
	// The route's scope rule is derived from the sources, not declared: any
	// scope that permits searching SOME registered package admits the
	// request, and handleSearch then narrows to the sources the grant covers.
	// Demanding one specific scope would 403 a single-package token outright
	// instead of handing it the results it may see. Evaluated per request so
	// it is right whatever order packages registered in.
	oauth.RegisterSharedEndpoint("GET", "/api/search", searchScopes)

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Router.GET("/api/search", func(re *core.RequestEvent) error {
			return handleSearch(app, re)
		}).Bind(apis.RequireAuth())
		return e.Next()
	})
}

// searchScopes is the union of every registered source's scopes.
func searchScopes() []string {
	var out []string
	for _, src := range RegisteredSources() {
		for _, s := range src.Scopes {
			if !oauth.HasScope(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

func handleSearch(app core.App, re *core.RequestEvent) error {
	q := re.Request.URL.Query()

	query := Query{
		Include: splitTerms(q.Get("q")),
		Exclude: splitTerms(q.Get("not")),
		Slugs:   q["pkg"],
		Limit:   atoiOr(q.Get("limit"), 0),
		Offset:  atoiOr(q.Get("offset"), 0),
	}

	// Scoped tokens see only the packages their grant covers. A session
	// carries no scope ceiling, so nil here means "not scope limited" — and
	// selectSources treats nil differently from an empty slice, which would
	// mean "a token granting nothing".
	granted := oauth.GrantedScopes(re)

	sources := selectSources(RegisteredSources(), query.Slugs, granted)
	resp := Aggregate(re.Request.Context(), app, re.Auth.Id, query, sources)
	return re.JSON(http.StatusOK, resp)
}

// splitTerms breaks a space-separated term list. The client owns the grammar
// (chips, leading '-'), so by here the values are plain terms; sanitizing for
// FTS5 happens inside each source, which knows its own backend.
func splitTerms(raw string) []string {
	fields := strings.Fields(raw)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}

func atoiOr(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
