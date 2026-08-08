package main

import (
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

// searchRow mirrors core/server/search.Row. Duplicated rather than imported
// for the reason output.FormatBytes documents: tinycld.org/core is PocketBase
// hook code, and pulling it into the CLI workspace would drag the fork replace
// into every member cli build (see gen-cli.ts).
type searchRow struct {
	Slug     string         `json:"slug"`
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Subtitle string         `json:"subtitle,omitempty"`
	Meta     string         `json:"meta,omitempty"`
	Fields   map[string]any `json:"fields,omitempty"`
}

// searchResponse mirrors core/server/search.Response. Fields survives into
// --json untouched so a scripted caller can filter on package-specific values
// (mailbox_id, mime_type) that the table has no column for.
type searchResponse struct {
	Rows      []searchRow    `json:"rows"`
	Counts    map[string]int `json:"counts"`
	Partial   []string       `json:"partial,omitempty"`
	Truncated []string       `json:"truncated,omitempty"`
}

func newSearchCmd(d *deps) *cobra.Command {
	var pkgs []string
	var not string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search across every installed package",
		Long: "Search across every installed package.\n\n" +
			"Scope to packages by prefixing the query with `pkg:` (as in the web\n" +
			"palette) or by passing --pkg. Exclude a term with a leading hyphen or\n" +
			"with --not.\n\n" +
			"Quote the query so the shell keeps it as one argument. A query that\n" +
			"starts with a hyphen needs a `--` separator, otherwise it is read as\n" +
			"a flag: tinycld search -- \"-draft budget\".",
		Example: "  tinycld search budget\n" +
			"  tinycld search \"mail: drive: budget -draft\"\n" +
			"  tinycld search budget --pkg mail --not draft --json",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}

			// The same grammar the palette uses, so a query copied from the app
			// behaves identically here (see parse_query.go).
			parsed := parseQuery(args[0], searchSlugs)
			parsed.Exclude = append(parsed.Exclude, parseQuery(not, searchSlugs).Include...)

			// An exclude-only query has no meaning: FTS5 cannot subtract from
			// nothing, and the server would return an empty set with no
			// explanation. Say so instead.
			if len(parsed.Include) == 0 {
				return fmt.Errorf("a search needs at least one term to match")
			}

			scope := mergeScope(parsed.Chips, pkgs)
			if unknown := unknownSlugs(scope, searchSlugs); len(unknown) > 0 {
				return fmt.Errorf(
					"unknown package(s): %s (installed and searchable: %s)",
					strings.Join(unknown, ", "), strings.Join(searchSlugs, ", "))
			}

			q := url.Values{"q": {strings.Join(parsed.Include, " ")}}
			if len(parsed.Exclude) > 0 {
				q.Set("not", strings.Join(parsed.Exclude, " "))
			}
			for _, slug := range scope {
				q.Add("pkg", slug)
			}
			if limit > 0 {
				q.Set("limit", strconv.Itoa(limit))
			}

			c := client.NewLazy(d.resolveClientContext, d.httpClient)
			var resp searchResponse
			if err := c.GetJSON(cmd.Context(), "/api/search?"+q.Encode(), &resp); err != nil {
				return err
			}

			rows := make([][]string, len(resp.Rows))
			for i, r := range resp.Rows {
				rows[i] = []string{
					r.Slug,
					output.StripMarks(r.Title),
					output.StripMarks(r.Subtitle),
					output.StripMarks(r.Meta),
				}
			}
			if err := o.Write(cmd.OutOrStdout(),
				[]string{"PKG", "TITLE", "SUBTITLE", "META"}, rows, resp); err != nil {
				return err
			}

			reportSearchSummary(cmd, o, resp)
			return nil
		},
	}

	fl := cmd.Flags()
	fl.StringArrayVar(&pkgs, "pkg", nil, "restrict to a package (repeatable)")
	fl.StringVar(&not, "not", "", "exclude results matching these terms")
	fl.IntVar(&limit, "limit", 0, "maximum results (server caps the total)")
	return cmd
}

// reportSearchSummary writes per-package counts and names any package that
// failed or was cut short.
//
// All of it goes to stderr so `--json` keeps stdout a clean document, and
// Partial is reported even when rows came back: a search that silently dropped
// a package reads exactly like one that found nothing there, which is the worse
// of the two lies.
func reportSearchSummary(cmd *cobra.Command, o output.Options, resp searchResponse) {
	w := cmd.ErrOrStderr()
	o.Info(w, "%d result(s)%s", len(resp.Rows), formatCounts(resp.Counts))

	if len(resp.Partial) > 0 {
		// Not routed through Info: a dropped package is a correctness caveat,
		// not chatter, so --quiet must not be able to hide it.
		fmt.Fprintf(w, "warning: no results from %s (search failed or timed out)\n",
			strings.Join(sorted(resp.Partial), ", "))
	}
	if len(resp.Truncated) > 0 {
		o.Info(w, "more matches available in %s", strings.Join(sorted(resp.Truncated), ", "))
	}
}

// formatCounts renders the per-package totals as " (12 in mail, 3 in drive)",
// ordered by count so the biggest contributor reads first, then by slug so the
// output is stable between runs.
func formatCounts(counts map[string]int) string {
	type entry struct {
		slug string
		n    int
	}
	entries := make([]entry, 0, len(counts))
	for slug, n := range counts {
		if n > 0 {
			entries = append(entries, entry{slug, n})
		}
	}
	if len(entries) == 0 {
		return ""
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].n != entries[j].n {
			return entries[i].n > entries[j].n
		}
		return entries[i].slug < entries[j].slug
	})
	parts := make([]string, len(entries))
	for i, e := range entries {
		parts[i] = fmt.Sprintf("%d in %s", e.n, e.slug)
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// mergeScope unions the query's `pkg:` chips with --pkg values. The two are
// additive rather than one overriding the other: both express the same intent,
// and silently dropping either would search somewhere the user excluded.
func mergeScope(chips, flags []string) []string {
	var out []string
	for _, s := range append(append([]string{}, chips...), flags...) {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" && !contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

// unknownSlugs reports requested packages that are not installed. A --pkg typo
// would otherwise come back as a confident empty result set.
func unknownSlugs(requested, installed []string) []string {
	var unknown []string
	for _, s := range requested {
		if !contains(installed, s) {
			unknown = append(unknown, s)
		}
	}
	return unknown
}

func sorted(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}
