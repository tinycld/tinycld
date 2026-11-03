// Package changesignal tells realtime clients that a collection changed,
// and never what changed.
//
// After a record write commits, core sends a message on the topic
// `collection/<name>` to every signed-in realtime client subscribed to it.
// The payload is `{}`: any signed-in client may subscribe to any topic, so a
// message must carry no data. A client that gets one runs its query again
// under its own rules. Messages are coalesced per topic, so a batch of
// writes sends one.
//
// Packages feed it in two ways, from their own Register(app):
//   - RegisterView declares the collections a view collection's SQL reads,
//     so a write to any of them also signals the view's topic. PocketBase's
//     view parser reads only the outer FROM and JOIN, so the declared list is
//     authoritative; CheckViews proves it against SQLite's bytecode in the
//     package's tests.
//   - RegisterVirtualTable declares the collections a stored virtual table
//     holds (a full-text index kept in step with its collection; fts does
//     it for each index it syncs), so a report or a view may read it.
//   - Touch signals a collection after a raw-SQL write, which runs no model
//     hook.
//
// TopicsRead and CheckViews find what a statement reads in SQLite's
// bytecode. A table's OpenRead names its root page, but a virtual table's
// VOpen carries only a pointer, so a table-valued function (json_each,
// json_tree: no stored rows, nothing to signal) and a stored virtual table
// (an FTS5 index, whose rows change with its collection) look alike. The
// check then reads names: every identifier in the statement's SQL, and in
// the SQL of each view it names, through views of views, against the
// stored virtual tables and views of every database the connection holds
// (main, temp and each attached one). When none is a
// stored virtual table, each VOpen is a table-valued function and adds
// nothing. Otherwise each stored virtual table named must be mapped with
// RegisterVirtualTable, and adds its collections; an unmapped one is
// refused. The scan is conservative: a column or alias spelled like a
// stored virtual table counts as a read of it, so a false hit costs a
// refusal or a mapping, and a read is never missed.
//
// Core names no package; tests use fictional zoo_* collections.
package changesignal

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
)

const topicPrefix = "collection/"

// Topic is the change-signal topic of a collection.
func Topic(collection string) string {
	return topicPrefix + collection
}

var (
	mu    sync.RWMutex
	views = map[string][]string{}
	// vtabs maps a stored virtual table, by lowercased name, to the
	// collections whose rows it holds.
	vtabs = map[string][]string{}
)

// RegisterView declares the collections that the view collection's SQL
// reads, in a subquery, an EXISTS or a CTE included. The view and its
// sources are collection names, not ids. A write to any source
// then also signals the view's topic. Calling it again for the same view
// adds sources. A blank name, no sources, or a view that lists itself panics
// at boot, because such a view could never be live.
func RegisterView(view string, sources ...string) {
	if view == "" || len(sources) == 0 {
		panic(fmt.Sprintf("changesignal.RegisterView(%q): a view needs a name and at least one source", view))
	}
	mu.Lock()
	defer mu.Unlock()
	set := map[string]bool{}
	for _, source := range views[view] {
		set[source] = true
	}
	for _, source := range sources {
		if source == "" || source == view {
			panic(fmt.Sprintf("changesignal.RegisterView(%q): invalid source %q", view, source))
		}
		set[source] = true
	}
	merged := make([]string, 0, len(set))
	for source := range set {
		merged = append(merged, source)
	}
	sort.Strings(merged)
	views[view] = merged
}

// RegisteredViews returns a copy of the registry: view name to sorted sources.
func RegisteredViews() map[string][]string {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string][]string, len(views))
	for view, sources := range views {
		out[view] = slices.Clone(sources)
	}
	return out
}

// topicsFor returns the topics a write to collection signals: its own, then
// every registered view that reads it, directly or through another view.
func topicsFor(collection string) []string {
	mu.RLock()
	defer mu.RUnlock()
	seen := map[string]bool{collection: true}
	queue := []string{collection}
	var viewTopics []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for view, sources := range views {
			if seen[view] || !slices.Contains(sources, current) {
				continue
			}
			seen[view] = true
			viewTopics = append(viewTopics, Topic(view))
			queue = append(queue, view)
		}
	}
	sort.Strings(viewTopics)
	return append([]string{Topic(collection)}, viewTopics...)
}

// coveredBy returns the collections a view may read: its declared sources
// and, for a source that is itself a registered view, that view's own.
// SQLite inlines a view it reads, so its bytecode opens the base tables.
func coveredBy(view string) map[string]bool {
	mu.RLock()
	defer mu.RUnlock()
	covered := map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		for _, source := range views[name] {
			if covered[source] {
				continue
			}
			covered[source] = true
			walk(source)
		}
	}
	walk(view)
	return covered
}

// RegisterVirtualTable declares the collections whose rows a stored virtual
// table holds, such as a full-text index kept in step with its collection.
// A query that reads the table then reads those collections, so a report
// over it gets their topics, and a view over it must declare them. The
// table is matched without case, as SQLite matches names; the collections
// are collection names, not ids. Calling it again for the same table adds
// collections. A blank name, no collections, or a table that lists itself
// panics at boot, because a read of such a table could never be live.
func RegisterVirtualTable(table string, collections ...string) {
	if table == "" || len(collections) == 0 {
		panic(fmt.Sprintf("changesignal.RegisterVirtualTable(%q): a virtual table needs a name and at least one collection", table))
	}
	key := strings.ToLower(table)
	mu.Lock()
	defer mu.Unlock()
	set := map[string]bool{}
	for _, collection := range vtabs[key] {
		set[collection] = true
	}
	for _, collection := range collections {
		if collection == "" || strings.ToLower(collection) == key {
			panic(fmt.Sprintf("changesignal.RegisterVirtualTable(%q): invalid collection %q", table, collection))
		}
		set[collection] = true
	}
	merged := make([]string, 0, len(set))
	for collection := range set {
		merged = append(merged, collection)
	}
	sort.Strings(merged)
	vtabs[key] = merged
}

// RegisteredVirtualTables returns a copy of the virtual-table registry:
// lowercased table name to sorted collections.
func RegisteredVirtualTables() map[string][]string {
	mu.RLock()
	defer mu.RUnlock()
	out := make(map[string][]string, len(vtabs))
	for table, collections := range vtabs {
		out[table] = slices.Clone(collections)
	}
	return out
}

// virtualTableCollections returns the collections a stored virtual table
// holds, and false when it is not registered.
func virtualTableCollections(table string) ([]string, bool) {
	mu.RLock()
	defer mu.RUnlock()
	collections, ok := vtabs[strings.ToLower(table)]
	return slices.Clone(collections), ok
}

// ResetForTesting empties the registries. Tests register their own views
// and virtual tables.
func ResetForTesting() {
	mu.Lock()
	defer mu.Unlock()
	views = map[string][]string{}
	vtabs = map[string][]string{}
}
