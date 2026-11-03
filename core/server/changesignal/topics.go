package changesignal

import (
	"context"
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// TopicsRead returns, sorted, the topic of every collection a SELECT
// statement reads: a CTE, a subquery, an EXISTS and a covering index
// included, as CheckViews finds them. A table-valued function (json_each)
// adds no topic; a stored virtual table adds the topics of the collections
// RegisterVirtualTable maps it to, and an unmapped one is an error (see the
// package comment). A client that shows the statement's
// result subscribes to these topics to know when to run it again. A system
// collection is left out: it sends no signal. A view's own topic is not
// listed, because SQLite reads the view's base tables, and their topics carry
// every change the view shows.
func TopicsRead(ctx context.Context, app core.App, query string, params dbx.Params) ([]string, error) {
	s, err := readSchema(ctx, app)
	if err != nil {
		return nil, err
	}
	tables, err := tablesRead(ctx, app, s, query, params)
	if err != nil {
		return nil, fmt.Errorf("changesignal: %w", err)
	}
	topics := make([]string, 0, len(tables))
	for _, table := range tables {
		col, err := app.FindCachedCollectionByNameOrId(table)
		if err != nil || col.System || col.Name != table {
			continue
		}
		topics = append(topics, Topic(col.Name))
	}
	return topics, nil
}
