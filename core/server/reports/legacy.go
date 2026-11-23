package reports

// legacySQL turns one change of the 2026-11 report performance work off, so a
// test runs a report with the SQL the engine wrote before it and compares the
// two results (requireSameAsLegacy). Only a test sets a field.
var legacySQL struct {
	// repeatAggregates writes an aggregate a level reads twice as two values
	// of a<L>, as before levelCtx.aggregate kept each one.
	repeatAggregates bool
	// noFilterBounds leaves the bounds on the base table's stored columns out
	// of w's WHERE, as before filterBounds wrote them.
	noFilterBounds bool
	// inlineDetailRows keeps w inline in every detail statement, as before
	// materializeShared marked the ones whose CTEs read it twice or more.
	inlineDetailRows bool
	// noLookupDayBounds leaves the bounds on a looked-up source's stored day
	// column out of a lookup's WHERE, as before lookupDayBound wrote them.
	noLookupDayBounds bool
	// rangeLinkBounds writes a lookup whose where matches a link once, with
	// the link column bounded by a range, as before knownLinks added the form
	// with an equality.
	rangeLinkBounds bool
	// cellsBeforeThePage computes every cell of a detail row in the select
	// the page is cut from, as before a row's own lookups moved above the cut
	// (pagedCells).
	cellsBeforeThePage bool
	// noPartBounds reads a union source through its rule CTE with no bound
	// inside its parts, as before partBounds wrote them.
	noPartBounds bool
	// plannedLeafJoin joins a detail row to its open group with a plain JOIN
	// on the bare keys, whose order SQLite's planner picks, as before leafFrom
	// fixed the order with CROSS JOIN and took the keys' affinity off.
	plannedLeafJoin bool
	// everyLeafRows computes a paged detail statement over the rows of every
	// open group of its page, as before leafRows left out the groups the page
	// cannot reach.
	everyLeafRows bool
	// noSelectIn writes in of a Select with a list of its option values as
	// the one test of the value read, as before selectIn added the test of
	// the stored column.
	noSelectIn bool
	// joinLinkKeys reads a to-one link's key through the join to its target,
	// as before storedKey read the row's own link column.
	joinLinkKeys bool
	// noPartConstants writes w over a union's rows as one select, and a
	// detail row's if or case over w's rows, as before partArms wrote one
	// select per part with the conditions a part's constant decides and the
	// results it never picks folded (partConstantFold, partBranches,
	// pushPartFold).
	noPartConstants bool
}
