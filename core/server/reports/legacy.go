package reports

// legacySQL turns one change of the 2026-11 report performance work off, so a
// test runs a report with the SQL the engine wrote before it and compares the
// two results (requireSameAsLegacy). Only a test sets a field.
var legacySQL struct {
	// repeatAggregates writes an aggregate a level reads twice as two values
	// of a<L>, as before levelCtx.aggregate kept each one.
	repeatAggregates bool
}
