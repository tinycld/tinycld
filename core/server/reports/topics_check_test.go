package reports

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/changesignal"
)

// A run finds a statement's topics from the rule CTEs it reads, without
// compiling the statement again (stmt.topics). Every statement of every test
// of the package is checked against the topics of the whole statement, which
// SQLite's own plan of it names, so a statement shape that reads a collection
// some other way fails the package. The rule CTEs may name a collection the
// plan leaves out (a join no value reads, a column no select reads): such a
// topic signals a change that cannot change the result, so it is allowed; a
// missing one would leave the client showing an old result, so it is not.
var (
	topicsMismatch   []string
	topicsMismatchMu sync.Mutex
)

func TestMain(m *testing.M) {
	checkTopics = func(ctx context.Context, app core.App, sql string, params dbx.Params, topics []string) {
		want, err := changesignal.TopicsRead(ctx, app, sql, params)
		if err != nil {
			want = []string{"error: " + err.Error()}
		}
		missing := slices.DeleteFunc(slices.Clone(want), func(t string) bool { return slices.Contains(topics, t) })
		if len(missing) > 0 {
			topicsMismatchMu.Lock()
			topicsMismatch = append(topicsMismatch, fmt.Sprintf("topics %v miss %v, which the whole statement reads:\n%.2000s", topics, missing, sql))
			topicsMismatchMu.Unlock()
		}
	}
	code := m.Run()
	if len(topicsMismatch) > 0 {
		for _, msg := range topicsMismatch {
			fmt.Fprintln(os.Stderr, msg)
		}
		fmt.Fprintf(os.Stderr, "FAIL: %d statements' topics differ from the topics of the whole statement\n", len(topicsMismatch))
		code = 1
	}
	os.Exit(code)
}
