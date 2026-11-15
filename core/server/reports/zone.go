package reports

import (
	"time"

	// A server image without /usr/share/zoneinfo still knows every zone, so a
	// reader's zone never silently reads as UTC (R2).
	_ "time/tzdata"
)

// instantLayout is how PocketBase stores a date or autodate value, so a bound
// instant compares with a stored one as text.
const instantLayout = "2006-01-02 15:04:05.000Z"

// zonedDay is one day of the reader's zone as the UTC instants of the local
// midnight that starts it and of the one that ends it: 23 or 25 hours apart
// across a clock change.
type zonedDay struct{ start, end string }

// utcNames are no name and the names of UTC itself. They read as nil, so a
// UTC reader's statements stay today's; the client's UTC_ZONES
// (core/reports/lib/date-time.ts) is the same set, so the server and the
// client agree on which reader is in UTC.
var utcNames = map[string]bool{"": true, "UTC": true, "Etc/UTC": true, "Etc/GMT": true, "GMT": true}

// readerZone reads the reader's IANA zone. nil is UTC: no name, a name of
// UTC itself (utcNames), a name the server does not know, and "Local", which
// is the server's own zone, never the reader's (R2).
func readerZone(name string) *time.Location {
	if utcNames[name] || name == "Local" {
		return nil
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil
	}
	return loc
}

// zoneDay is day (YYYY-MM-DD, already checked by isDay) in loc.
func zoneDay(day string, loc *time.Location) zonedDay {
	d, _ := time.Parse(time.DateOnly, day)
	start := localMidnight(d, loc)
	end := localMidnight(d.AddDate(0, 0, 1), loc)
	return zonedDay{start: start.UTC().Format(instantLayout), end: end.UTC().Format(instantLayout)}
}

// localMidnight is the first instant of day in loc. Where a clock change
// skips midnight (America/Santiago), time.Date may answer an instant of the
// day before; the day then starts when the clock jumps, so step to it. The
// loop ends: each step moves the clock forward, and a skipped whole day
// (Pacific/Apia, 2011) passes the day.
func localMidnight(day time.Time, loc *time.Location) time.Time {
	t := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	want := day.Format(time.DateOnly)
	for t.Format(time.DateOnly) < want {
		t = t.Add(15 * time.Minute)
	}
	return t
}

// withZonedDays gives a set day or date range param its days in the reader's
// zone. A UTC reader's param has none, so its SQL is today's (R1).
func withZonedDays(v paramValue, loc *time.Location) paramValue {
	if loc == nil || v.unset {
		return v
	}
	switch v.typ {
	case ParamDate, ParamAsOf:
		if day, ok := v.value.(string); ok {
			z := zoneDay(day, loc)
			v.dayZ = &z
		}
	case ParamDateRange:
		// All dates has no days to zone.
		if v.from != "" && v.to != "" {
			from, to := zoneDay(v.from, loc), zoneDay(v.to, loc)
			v.fromZ, v.toZ = &from, &to
		}
	}
	return v
}

// zonedSpan is x within [start, end): the instants of whole local days.
func zonedSpan(c fctx, x expr, start, end string) expr {
	return expr{sql: "(" + x.sql + " >= " + c.bind(start) + " AND " + x.sql + " < " + c.bind(end) + ")", typ: Bool}
}

var mirrored = map[string]string{"lt": "gt", "lte": "gte", "gt": "lt", "gte": "lte"}

// zonedCompare compares a date-time with a day of the reader's zone as
// instants (R1): gte is from the day's start, gt from its end, lt before its
// start, lte before its end. A day on the left mirrors. ok is false when the
// comparison is not zoned; the caller then compiles it as before.
func zonedCompare(c fctx, op string, a, b expr) (expr, bool) {
	if a.zoned != nil && b.typ == DateTime {
		a, b, op = b, a, mirrored[op]
	}
	if a.typ != DateTime || b.zoned == nil {
		return expr{}, false
	}
	bound, cmp := b.zoned.start, " >= "
	switch op {
	case "gt":
		bound = b.zoned.end
	case "lt":
		cmp = " < "
	case "lte":
		bound, cmp = b.zoned.end, " < "
	}
	return expr{sql: "(" + a.sql + cmp + c.bind(bound) + ")", typ: Bool}, true
}
