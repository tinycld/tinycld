package changesignal

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/pocketbase/pocketbase/core"
)

// schema is what the check reads from sqlite_schema: the table each root
// page belongs to, the stored virtual tables, and the SQL of each view.
type schema struct {
	pages map[int64]string
	// vtabs maps a stored virtual table's lowercased name to its name.
	vtabs map[string]string
	// views maps a view's lowercased name to its SQL.
	views map[string]string
}

func readSchema(ctx context.Context, app core.App) (*schema, error) {
	var rows []struct {
		Type     string `db:"type"`
		Name     string `db:"name"`
		Table    string `db:"tbl_name"`
		RootPage int64  `db:"rootpage"`
		SQL      string `db:"sql"`
	}
	// Only a view's SQL and a rootless table's (a virtual table has no root
	// page) are read; TopicsRead runs this per report statement.
	err := app.DB().
		NewQuery(`SELECT type, name, tbl_name, COALESCE(rootpage, 0) AS rootpage,
			CASE WHEN type = 'view' OR (type = 'table' AND COALESCE(rootpage, 0) = 0) THEN COALESCE(sql, '') ELSE '' END AS sql
			FROM sqlite_schema`).
		WithContext(ctx).
		All(&rows)
	if err != nil {
		return nil, fmt.Errorf("changesignal: read sqlite_schema: %w", err)
	}
	s := &schema{pages: map[int64]string{}, vtabs: map[string]string{}, views: map[string]string{}}
	for _, row := range rows {
		if row.RootPage > 0 {
			s.pages[row.RootPage] = row.Table
		}
		switch {
		case row.Type == "view":
			s.views[strings.ToLower(row.Name)] = row.SQL
		case row.Type == "table" && isVirtualTableSQL(row.SQL):
			s.vtabs[strings.ToLower(row.Name)] = row.Name
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
// {{table}} and [[alias.column]] markers are blanked, so their contents read
// as words; the quoted runs are read from the text before and after, so a
// name that starts with a bracket is kept too. A word in a comment counts:
// a false hit costs a refusal or a mapping, never a missed topic.
func identifiers(sql string) map[string]bool {
	names := map[string]bool{}
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
