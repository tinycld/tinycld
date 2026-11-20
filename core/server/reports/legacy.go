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
}
