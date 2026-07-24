package fts

import (
	"net/http"
	"strconv"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// Register wires the FTS capability for the given configs: the index-sync record
// hooks (bound immediately) and a search route per config (added on serve).
// Call from coreserver.Register with the configs materialized from package
// manifests.
func Register(app *pocketbase.PocketBase, configs []Config) {
	for _, cfg := range configs {
		RegisterSync(app, cfg)
	}

	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		for _, cfg := range configs {
			registerSearchRoute(app, e, cfg)
		}
		return e.Next()
	})
}

type searchResponse struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
}

// registerSearchRoute adds GET /api/{slug}/search for one config. The route
// requires auth and scopes results to the caller's org memberships.
func registerSearchRoute(app *pocketbase.PocketBase, e *core.ServeEvent, cfg Config) {
	path := "/api/" + cfg.Slug + "/search"
	e.Router.GET(path, func(re *core.RequestEvent) error {
		if re.Auth == nil {
			return re.UnauthorizedError("Authentication required", nil)
		}

		q := re.Request.URL.Query()
		opts := SearchOpts{
			Query:          q.Get("q"),
			Limit:          clampInt(q.Get("limit"), 25, 1, 100),
			Offset:         clampInt(q.Get("offset"), 0, 0, 1<<30),
			IncludeDeleted: q.Get("deleted") == "true",
		}

		empty := searchResponse{Items: []map[string]any{}, Total: 0}
		if len(opts.Query) < 2 {
			return re.JSON(http.StatusOK, empty)
		}

		results, total, err := Search(app, cfg, re.Auth.Id, opts)
		if err != nil {
			app.Logger().Warn("fts: search query failed",
				"slug", cfg.Slug, "error", err)
			return re.JSON(http.StatusOK, empty)
		}

		items := make([]map[string]any, len(results))
		for i, r := range results {
			item := map[string]any{"id": r.ID}
			for k, v := range r.Columns {
				item[k] = v
			}
			items[i] = item
		}
		return re.JSON(http.StatusOK, searchResponse{Items: items, Total: total})
	})
}

// clampInt parses s, falling back to def, then clamps to [min, max].
func clampInt(s string, def, min, max int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
