package changesignal

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// schema is what the check reads from the schema tables: the main
// database's table of each root page, and, from every database of the
// connection (main, temp and each attached one), the stored virtual tables
// and the SQL of each view.
type schema struct {
	pages map[int64]string
	// vtabs maps a stored virtual table's lowercased name to its name.
	vtabs map[string]string
	// views maps a view's lowercased name to its SQL; views of one name in
	// two databases keep both SQL texts.
	views map[string]string
}

// readSchema reads the schema of each database the connection holds. A
// temp or attached virtual table, or a temp view over an index, is read
// like a main one, so a statement over it is mapped or refused, never taken
// for a table-valued function. The databases are read on app's connection;
// inside a transaction that is the connection the statement runs on.
func readSchema(ctx context.Context, app core.App) (*schema, error) {
	var databases []struct {
		Seq  int64  `db:"seq"`
		Name string `db:"name"`
	}
	if err := app.DB().NewQuery("SELECT seq, name FROM pragma_database_list").WithContext(ctx).All(&databases); err != nil {
		return nil, fmt.Errorf("changesignal: list databases: %w", err)
	}
	s := &schema{pages: map[int64]string{}, vtabs: map[string]string{}, views: map[string]string{}}
	for _, db := range databases {
		var rows []struct {
			Type     string `db:"type"`
			Name     string `db:"name"`
			Table    string `db:"tbl_name"`
			RootPage int64  `db:"rootpage"`
			SQL      string `db:"sql"`
		}
		// Only a view's SQL and a rootless table's (a virtual table has no
		// root page) are read; TopicsRead runs this per report statement.
		quoted := `"` + strings.ReplaceAll(db.Name, `"`, `""`) + `"`
		err := app.DB().
			NewQuery(`SELECT type, name, tbl_name, COALESCE(rootpage, 0) AS rootpage,
				CASE WHEN type = 'view' OR (type = 'table' AND COALESCE(rootpage, 0) = 0) THEN COALESCE(sql, '') ELSE '' END AS sql
				FROM ` + quoted + `.sqlite_schema`).
			WithContext(ctx).
			All(&rows)
		if err != nil {
			return nil, fmt.Errorf("changesignal: read %s.sqlite_schema: %w", db.Name, err)
		}
		for _, row := range rows {
			// OpenRead's P3 names the database; tablesRead reads the main
			// database's pages only.
			if row.RootPage > 0 && db.Seq == 0 {
				s.pages[row.RootPage] = row.Table
			}
			key := strings.ToLower(row.Name)
			switch {
			case row.Type == "view":
				s.views[key] += row.SQL + "\n"
			case row.Type == "table" && isVirtualTableSQL(row.SQL):
				s.vtabs[key] = row.Name
			}
		}
	}
	return s, nil
}

var virtualTableRe = regexp.MustCompile(`(?i)^\s*create\s+virtual\s+table\b`)

// isVirtualTableSQL reports whether a sqlite_schema table's SQL creates a
// stored virtual table. A table-valued function (json_each, json_tree) is
// eponymous: it has no sqlite_schema row.
func isVirtualTableSQL(sql string) bool {
	return virtualTableRe.MatchString(sql)
}

// virtualTablesReached lists, sorted, the stored virtual tables a query
// could read: those named in its SQL, or in the SQL of a view it names,
// through views of views. A VOpen opcode carries a pointer, not a name, so
// the names are all the check has.
func (s *schema) virtualTablesReached(query string) []string {
	var reached []string
	seen := map[string]bool{}
	var scan func(sql string)
	scan = func(sql string) {
		for name := range identifiers(sql) {
			// seen also guards a view whose SQL spells its own name.
			if seen[name] {
				continue
			}
			seen[name] = true
			if table, ok := s.vtabs[name]; ok {
				reached = append(reached, table)
			}
			if viewSQL, ok := s.views[name]; ok {
				scan(viewSQL)
			}
		}
	}
	scan(query)
	sort.Strings(reached)
	return reached
}

var wordRe = regexp.MustCompile(`[\w$\x{80}-\x{10FFFF}]+`)

// identifiers returns, lowercased, every name in sql that could name a
// table: each bare word, and the text of each quoted name ("…", `…`, […]) or
// string ('…', which SQLite reads as a name where it expects one). dbx's
// {{table}} and [[alias.column]] markers are read as dbx quotes them: each
// dot-separated part is one name, whatever characters it holds. The markers
// are also blanked, so their contents read as words, and the quoted runs
// are read from the text before and after, so a name that starts with a
// bracket is kept too. A word in a comment counts: a false hit costs a
// refusal or a mapping, never a missed topic.
func identifiers(sql string) map[string]bool {
	names := map[string]bool{}
	for _, marker := range dbxMarkerRe.FindAllStringSubmatch(sql, -1) {
		inner := marker[1] + marker[2]
		names[strings.ToLower(inner)] = true
		for part := range strings.SplitSeq(inner, ".") {
			names[strings.ToLower(part)] = true
		}
	}
	blanked := sql
	for _, marker := range []string{"{{", "}}", "[[", "]]"} {
		blanked = strings.ReplaceAll(blanked, marker, "  ")
	}
	for _, word := range wordRe.FindAllString(blanked, -1) {
		names[strings.ToLower(word)] = true
	}
	quotedNames(sql, names)
	quotedNames(blanked, names)
	return names
}

// dbxMarkerRe finds dbx's {{table}} and [[alias.column]] markers with the
// characters dbx's own quoteRegex takes (dbx db.go).
var dbxMarkerRe = regexp.MustCompile(`\{\{([\w\-\. ]+)\}\}|\[\[([\w\-\. ]+)\]\]`)

// quotedNames adds the text of each quoted run in sql, read as SQLite's
// tokenizer reads it: a doubled quote is one quote, and a quote inside a
// comment starts nothing.
func quotedNames(sql string, names map[string]bool) {
	for i := 0; i < len(sql); {
		switch c := sql[i]; {
		case strings.HasPrefix(sql[i:], "--"):
			i = skipPast(sql, i+2, "\n")
		case strings.HasPrefix(sql[i:], "/*"):
			i = skipPast(sql, i+2, "*/")
		case c == '"' || c == '`' || c == '\'':
			var name strings.Builder
			i++
			for i < len(sql) {
				if sql[i] != c {
					name.WriteByte(sql[i])
					i++
					continue
				}
				if i+1 < len(sql) && sql[i+1] == c {
					name.WriteByte(c)
					i += 2
					continue
				}
				i++
				break
			}
			names[strings.ToLower(name.String())] = true
		case c == '[':
			end := skipPast(sql, i+1, "]")
			names[strings.ToLower(strings.TrimSuffix(sql[i+1:end], "]"))] = true
			i = end
		default:
			i++
		}
	}
}

// skipPast returns the index just past the first end at or after from, or
// the length of sql when there is none.
func skipPast(sql string, from int, end string) int {
	if n := strings.Index(sql[from:], end); n >= 0 {
		return from + n + len(end)
	}
	return len(sql)
}
