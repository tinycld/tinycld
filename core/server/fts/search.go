package fts

import (
	"strconv"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase"
)

// SearchResult is one search hit: the record id plus the configured Output
// columns read from the joined collection row.
type SearchResult struct {
	ID      string
	Columns map[string]any
}

// SearchOpts parameterizes a search. Query is the raw user text (sanitized
// here). IncludeDeleted flips the soft-delete split when the config declares a
// SoftDeleteField.
type SearchOpts struct {
	Query          string
	Limit          int
	Offset         int
	IncludeDeleted bool
}

// Search runs an owner-scoped FTS5 MATCH for one config against the user's org
// memberships, returning hits (ordered by FTS rank) and the total count. It
// returns no rows — never an error to the caller path — for an empty/too-short
// query or when the user has no memberships; a genuine DB failure is returned so
// the route can log it. Column identifiers come from config (trusted); only the
// MATCH value and owner ids are bound parameters.
func Search(app *pocketbase.PocketBase, cfg Config, userID string, opts SearchOpts) ([]SearchResult, int, error) {
	match := SanitizeQuery(opts.Query)
	if match == "" {
		return nil, 0, nil
	}

	ownerIDs, err := ownerScopeIDs(app, cfg, userID)
	if err != nil {
		return nil, 0, err
	}
	if len(ownerIDs) == 0 {
		return nil, 0, nil
	}

	params := map[string]any{"match": match}
	ownerPlaceholders := make([]string, len(ownerIDs))
	for i, id := range ownerIDs {
		key := "own" + strconv.Itoa(i)
		params[key] = id
		ownerPlaceholders[i] = "{:" + key + "}"
	}
	inClause := "(" + strings.Join(ownerPlaceholders, ", ") + ")"

	// Soft-delete split is a fixed SQL fragment (no user input).
	deletedClause := ""
	if cfg.SoftDeleteField != "" {
		if opts.IncludeDeleted {
			deletedClause = " AND c." + cfg.SoftDeleteField + " != ''"
		} else {
			deletedClause = " AND c." + cfg.SoftDeleteField + " = ''"
		}
	}

	// Build SELECT list from config Output (trusted column names). We
	// deliberately never select an FTS snippet()/highlight column: it would
	// wrap raw user data in <mark> and become an XSS sink if a client rendered
	// it as HTML. Output values come from the joined collection row.
	selectCols := make([]string, 0, len(cfg.Output)+1)
	selectCols = append(selectCols, "c.id AS id")
	for _, col := range cfg.Output {
		selectCols = append(selectCols, "c."+col.Name+" AS "+col.Name)
	}

	base := " FROM " + cfg.Table +
		" JOIN " + cfg.Collection + " c ON c.id = " + cfg.Table + ".record_id" +
		" WHERE " + cfg.Table + " MATCH {:match}" +
		" AND c." + cfg.Owner.Field + " IN " + inClause +
		deletedClause

	searchSQL := "SELECT " + strings.Join(selectCols, ", ") + base +
		" ORDER BY " + cfg.Table + ".rank LIMIT {:limit} OFFSET {:offset}"

	rowParams := map[string]any{"limit": opts.Limit, "offset": opts.Offset}
	for k, v := range params {
		rowParams[k] = v
	}

	var rows []dbx.NullStringMap
	if err := app.DB().NewQuery(searchSQL).Bind(dbx.Params(rowParams)).All(&rows); err != nil {
		return nil, 0, err
	}

	results := make([]SearchResult, 0, len(rows))
	for _, row := range rows {
		item := SearchResult{Columns: make(map[string]any, len(cfg.Output))}
		if v, ok := row["id"]; ok && v.Valid {
			item.ID = v.String
		}
		for _, col := range cfg.Output {
			raw := ""
			if v, ok := row[col.Name]; ok && v.Valid {
				raw = v.String
			}
			item.Columns[col.Name] = coerce(raw, col.Type)
		}
		results = append(results, item)
	}

	countSQL := "SELECT COUNT(*) AS total" + base
	var count struct {
		Total int `db:"total"`
	}
	if err := app.DB().NewQuery(countSQL).Bind(dbx.Params(params)).One(&count); err != nil {
		// A count failure shouldn't drop the results already fetched.
		count.Total = len(results)
	}

	return results, count.Total, nil
}

// coerce converts a raw string cell to the JSON type an output column declares,
// so the response matches the collection schema. Unknown/empty type stays string.
func coerce(raw, typ string) any {
	switch typ {
	case "bool":
		// PB/SQLite booleans read back as "0"/"1" or "true"/"false".
		return raw == "1" || raw == "true"
	case "number":
		if raw == "" {
			return 0
		}
		if n, err := strconv.ParseFloat(raw, 64); err == nil {
			return n
		}
		return 0
	default:
		return raw
	}
}

// ownerScopeIDs returns the ids on cfg.Owner.Via that belong to userID — the set
// of owner references (e.g. user_org ids) search is scoped to. The user field is
// a config-supplied identifier interpolated into the filter; the value is bound.
func ownerScopeIDs(app *pocketbase.PocketBase, cfg Config, userID string) ([]string, error) {
	recs, err := app.FindRecordsByFilter(
		cfg.Owner.Via,
		cfg.Owner.UserField+" = {:user}",
		"",
		0,
		0,
		map[string]any{"user": userID},
	)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(recs))
	for i, r := range recs {
		ids[i] = r.Id
	}
	return ids, nil
}
