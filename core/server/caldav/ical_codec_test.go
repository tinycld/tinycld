package caldav

import (
	"bytes"
	"strings"
	"testing"

	"github.com/emersion/go-ical"
	"github.com/pocketbase/pocketbase/core"
)

// testEventMap is the field map the calendar package ships, used by every codec
// test so the mapping under test is the one that runs in production.
var testEventMap = EventMap{
	Calendar:    "calendar",
	UID:         "ical_uid",
	Owner:       "created_by",
	Title:       "title",
	Description: "description",
	Location:    "location",
	Start:       "start",
	End:         "end",
	AllDay:      "all_day",
	Recurrence:  "recurrence",
	Guests:      "guests",
	Reminder:    "reminder",
	BusyStatus:  "busy_status",
	Visibility:  "visibility",
	Updated:     "updated",
	Created:     "created",
	Defaults: map[string]any{
		"busy_status": "busy",
		"visibility":  "default",
	},
}

// newEventRecord builds a standalone record on a synthetic collection carrying
// every mapped field, so the codec can be exercised without a live app.
func newEventRecord(t *testing.T, fields map[string]any) *core.Record {
	t.Helper()
	col := core.NewBaseCollection("calendar_events")
	col.Fields.Add(
		&core.TextField{Name: "calendar"},
		&core.TextField{Name: "ical_uid"},
		&core.TextField{Name: "created_by"},
		&core.TextField{Name: "title"},
		&core.TextField{Name: "description"},
		&core.TextField{Name: "location"},
		&core.TextField{Name: "start"},
		&core.TextField{Name: "end"},
		&core.BoolField{Name: "all_day"},
		&core.TextField{Name: "recurrence"},
		&core.JSONField{Name: "guests"},
		&core.NumberField{Name: "reminder"},
		&core.TextField{Name: "busy_status"},
		&core.TextField{Name: "visibility"},
		&core.TextField{Name: "updated"},
		&core.TextField{Name: "created"},
	)

	record := core.NewRecord(col)
	for k, v := range fields {
		record.Set(k, v)
	}
	return record
}

func TestRecordToCalendar_RequiresUID(t *testing.T) {
	record := newEventRecord(t, map[string]any{"title": "No UID"})
	if _, err := recordToCalendar(record, testEventMap); err == nil {
		t.Fatal("expected an error for a record with no ical_uid, got nil")
	}
}

func TestRecordToCalendar_MapsCoreFields(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid":    "urn:uuid:abc",
		"title":       "Standup",
		"description": "Daily sync",
		"location":    "Room 2",
		"start":       "2026-07-26 09:00:00.000Z",
		"end":         "2026-07-26 09:15:00.000Z",
		"busy_status": "busy",
	})

	cal, err := recordToCalendar(record, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}

	events := cal.Events()
	if len(events) != 1 {
		t.Fatalf("expected 1 VEVENT, got %d", len(events))
	}
	props := events[0].Props

	if got, _ := props.Text(ical.PropUID); got != "urn:uuid:abc" {
		t.Errorf("UID = %q", got)
	}
	if got, _ := props.Text(ical.PropSummary); got != "Standup" {
		t.Errorf("SUMMARY = %q", got)
	}
	if got, _ := props.Text(ical.PropLocation); got != "Room 2" {
		t.Errorf("LOCATION = %q", got)
	}
	if got, _ := props.Text(ical.PropTransparency); got != "OPAQUE" {
		t.Errorf("TRANSP = %q, want OPAQUE", got)
	}
}

// A "free" event must emit TRANSPARENT. A client that sees no TRANSP assumes
// OPAQUE, so staying silent would misreport free time as busy.
func TestRecordToCalendar_FreeEmitsTransparent(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid":    "urn:uuid:free",
		"busy_status": "free",
	})
	cal, err := recordToCalendar(record, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	if got, _ := cal.Events()[0].Props.Text(ical.PropTransparency); got != "TRANSPARENT" {
		t.Errorf("TRANSP = %q, want TRANSPARENT", got)
	}
}

// An unmapped field must not be emitted at all — that is what lets a package
// adopt CalDAV before it has every field.
func TestRecordToCalendar_UnmappedFieldsOmitted(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid": "urn:uuid:min",
		"title":    "Has a title",
		"location": "Has a location",
	})

	minimal := EventMap{Calendar: "calendar", UID: "ical_uid", Start: "start", End: "end"}
	cal, err := recordToCalendar(record, minimal)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	props := cal.Events()[0].Props

	if got, _ := props.Text(ical.PropSummary); got != "" {
		t.Errorf("SUMMARY = %q, want empty (Title unmapped)", got)
	}
	if got, _ := props.Text(ical.PropLocation); got != "" {
		t.Errorf("LOCATION = %q, want empty (Location unmapped)", got)
	}
	if got, _ := props.Text(ical.PropTransparency); got != "" {
		t.Errorf("TRANSP = %q, want empty (BusyStatus unmapped)", got)
	}
}

func TestRecordToCalendar_AllDayUsesDateValue(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid": "urn:uuid:allday",
		"all_day":  true,
		"start":    "2026-07-26 00:00:00.000Z",
	})
	cal, err := recordToCalendar(record, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	dtStart := cal.Events()[0].Props.Get(ical.PropDateTimeStart)
	if dtStart == nil {
		t.Fatal("no DTSTART emitted")
	}
	if got := dtStart.Params.Get("VALUE"); got != "DATE" {
		t.Errorf("DTSTART VALUE = %q, want DATE", got)
	}
}

func TestRecurrenceTokenRoundTrip(t *testing.T) {
	for _, token := range []string{"daily", "weekly", "monthly", "yearly"} {
		rrule := recurrenceToRRule(token)
		if rrule == "" {
			t.Fatalf("%q produced an empty RRULE", token)
		}
		if back := rruleToRecurrence(rrule); back != token {
			t.Errorf("%q → %q → %q, want the original token", token, rrule, back)
		}
	}
}

// A complex RRULE must survive verbatim in both directions — this is the part a
// flat field map could not express, and the reason the codec is Go rather than
// config.
func TestRecurrenceComplexRRulePreserved(t *testing.T) {
	complex := "FREQ=WEEKLY;BYDAY=MO,WE,FR;UNTIL=20261231T235959Z"
	if got := recurrenceToRRule(complex); got != complex {
		t.Errorf("recurrenceToRRule mangled a complex rule: %q", got)
	}
	if got := rruleToRecurrence(complex); got != complex {
		t.Errorf("rruleToRecurrence mangled a complex rule: %q", got)
	}
}

func TestApplyCalendarToRecord_RequiresVEvent(t *testing.T) {
	record := newEventRecord(t, nil)
	empty := ical.NewCalendar()
	if err := applyCalendarToRecord(empty, record, testEventMap); err == nil {
		t.Fatal("expected an error for a VCALENDAR with no VEVENT")
	}
}

// A client removing the recurrence sends the VEVENT without an RRULE. Keeping
// the old value would silently refuse to un-repeat the event.
func TestApplyCalendarToRecord_ClearsAbsentRecurrence(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid":   "urn:uuid:derecur",
		"recurrence": "weekly",
	})

	cal := ical.NewCalendar()
	event := ical.NewEvent()
	event.Props.SetText(ical.PropUID, "urn:uuid:derecur")
	event.Props.SetText(ical.PropSummary, "No longer repeating")
	cal.Children = append(cal.Children, event.Component)

	if err := applyCalendarToRecord(cal, record, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}
	if got := record.GetString("recurrence"); got != "" {
		t.Errorf("recurrence = %q, want cleared", got)
	}
}

func TestFullRoundTrip_PreservesRecurrenceAndFields(t *testing.T) {
	original := newEventRecord(t, map[string]any{
		"ical_uid":    "urn:uuid:round",
		"title":       "Weekly review",
		"description": "Look back",
		"location":    "Room 1",
		"start":       "2026-07-26 15:00:00.000Z",
		"end":         "2026-07-26 16:00:00.000Z",
		"recurrence":  "FREQ=WEEKLY;BYDAY=FR;UNTIL=20261231T235959Z",
		"busy_status": "free",
		"visibility":  "private",
		"reminder":    15,
	})

	cal, err := recordToCalendar(original, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}

	decoded := newEventRecord(t, nil)
	if err := applyCalendarToRecord(cal, decoded, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}

	for _, f := range []string{"title", "description", "location", "recurrence", "busy_status", "visibility"} {
		if got, want := decoded.GetString(f), original.GetString(f); got != want {
			t.Errorf("%s = %q after round trip, want %q", f, got, want)
		}
	}
	if got := decoded.GetFloat("reminder"); got != 15 {
		t.Errorf("reminder = %v after round trip, want 15", got)
	}
	if got := decoded.GetString("start"); got != original.GetString("start") {
		t.Errorf("start = %q after round trip, want %q", got, original.GetString("start"))
	}
}

func TestGuestsRoundTrip(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid": "urn:uuid:guests",
		"guests": []map[string]any{
			{"name": "Ada", "email": "ada@example.com", "rsvp": "accepted", "role": "organizer"},
			{"name": "Bob", "email": "bob@example.com", "rsvp": "declined", "role": "attendee"},
		},
	})

	cal, err := recordToCalendar(record, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	attendees := cal.Events()[0].Props.Values(ical.PropAttendee)
	if len(attendees) != 2 {
		t.Fatalf("expected 2 ATTENDEE props, got %d", len(attendees))
	}

	decoded := newEventRecord(t, nil)
	if err := applyCalendarToRecord(cal, decoded, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}

	var got []guest
	if err := decoded.UnmarshalJSONField("guests", &got); err != nil {
		t.Fatalf("unmarshal guests: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 guests after round trip, got %d", len(got))
	}
	if got[0].Email != "ada@example.com" || got[0].RSVP != "accepted" || got[0].Role != "organizer" {
		t.Errorf("first guest round-tripped wrong: %+v", got[0])
	}
	if got[1].RSVP != "declined" {
		t.Errorf("second guest RSVP = %q, want declined", got[1].RSVP)
	}
}

func TestParseTriggerMinutes(t *testing.T) {
	cases := map[string]int{
		"-PT15M":   15,
		"-PT1H":    60,
		"-PT1H30M": 90,
		"-P1D":     24 * 60,
		"-P1W":     7 * 24 * 60,
		"-PT30S":   0,
		"":         0,
	}
	for value, want := range cases {
		if got := parseTriggerMinutes(value); got != want {
			t.Errorf("parseTriggerMinutes(%q) = %d, want %d", value, got, want)
		}
	}
}

func TestAlarmRoundTrip(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid": "urn:uuid:alarm",
		"reminder": 30,
	})
	cal, err := recordToCalendar(record, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}

	decoded := newEventRecord(t, nil)
	if err := applyCalendarToRecord(cal, decoded, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}
	if got := decoded.GetFloat("reminder"); got != 30 {
		t.Errorf("reminder = %v after round trip, want 30", got)
	}
}

func TestExtractPathSegments(t *testing.T) {
	calCases := map[string]string{
		"/caldav/u/cal/abc123/":        "abc123",
		"/caldav/u/cal/abc123/evt.ics": "abc123",
		"/caldav/u/cal/":               "",
		"/caldav/u/":                   "",
		"":                             "",
	}
	for path, want := range calCases {
		if got := extractCalendarID(path); got != want {
			t.Errorf("extractCalendarID(%q) = %q, want %q", path, got, want)
		}
	}

	uidCases := map[string]string{
		"/caldav/u/cal/abc123/urn:uuid:x.ics": "urn:uuid:x",
		"/caldav/u/cal/abc123/":               "",
		"/caldav/u/cal/abc123":                "",
	}
	for path, want := range uidCases {
		if got := extractICalUID(path); got != want {
			t.Errorf("extractICalUID(%q) = %q, want %q", path, got, want)
		}
	}
}

// The wire-format round trip.
//
// The tests above build a VEVENT in memory, which hides two real bugs, because
// an in-process property and a decoded one are not the same thing:
//
//   - A DECODED RRULE has value type RECUR, and Props.Text() rejects anything
//     that is not TEXT — so reading the rule with Text() returned "" for every
//     recurrence a real client sent, silently dropping it.
//   - Emitting an RRULE with SetText() stamps VALUE=TEXT and escapes the
//     separators (`FREQ=WEEKLY\;BYDAY=TU`), which no client can parse.
//
// Both were invisible until a live PUT. So encode and decode through actual
// iCalendar bytes here, the way the protocol does.

// encodeToBytes renders a record to iCalendar text.
func encodeToBytes(t *testing.T, record *core.Record) string {
	t.Helper()
	cal, err := recordToCalendar(record, testEventMap)
	if err != nil {
		t.Fatalf("recordToCalendar: %v", err)
	}
	var buf bytes.Buffer
	if err := ical.NewEncoder(&buf).Encode(cal); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.String()
}

// decodeFromBytes parses iCalendar text the way a PUT body arrives.
func decodeFromBytes(t *testing.T, raw string) *ical.Calendar {
	t.Helper()
	cal, err := ical.NewDecoder(strings.NewReader(raw)).Decode()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return cal
}

func TestWire_DecodedRRuleIsStored(t *testing.T) {
	rrule := "FREQ=WEEKLY;BYDAY=TU;UNTIL=20261231T235959Z"
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:wire-1\r\nSUMMARY:Weekly\r\nDTSTAMP:20260726T120000Z\r\n" +
		"DTSTART:20260728T090000Z\r\nDTEND:20260728T100000Z\r\n" +
		"RRULE:" + rrule + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	record := newEventRecord(t, nil)
	if err := applyCalendarToRecord(decodeFromBytes(t, raw), record, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}
	if got := record.GetString("recurrence"); got != rrule {
		t.Errorf("recurrence = %q, want %q — a decoded RRULE is RECUR-typed, not TEXT", got, rrule)
	}
}

func TestWire_EmittedRRuleIsWellFormed(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid":   "wire-2",
		"title":      "Weekly",
		"start":      "2026-07-28 09:00:00.000Z",
		"end":        "2026-07-28 10:00:00.000Z",
		"updated":    "2026-07-26 12:00:00.000Z",
		"recurrence": "FREQ=WEEKLY;BYDAY=TU;UNTIL=20261231T235959Z",
	})

	out := encodeToBytes(t, record)

	var line string
	for _, l := range strings.Split(out, "\r\n") {
		if strings.HasPrefix(l, "RRULE") {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatal("no RRULE line emitted")
	}
	if strings.Contains(line, "VALUE=TEXT") {
		t.Errorf("RRULE emitted as VALUE=TEXT, which clients reject: %q", line)
	}
	if strings.Contains(line, `\;`) {
		t.Errorf("RRULE separators escaped, which clients cannot parse: %q", line)
	}
	if line != "RRULE:FREQ=WEEKLY;BYDAY=TU;UNTIL=20261231T235959Z" {
		t.Errorf("RRULE line = %q", line)
	}
}

// The full loop: record → bytes → record. This is what a client sync does, and
// it fails if EITHER direction mishandles the value type.
func TestWire_FullRoundTripThroughBytes(t *testing.T) {
	rrule := "FREQ=MONTHLY;BYMONTHDAY=15;COUNT=6"
	original := newEventRecord(t, map[string]any{
		"ical_uid":    "wire-3",
		"title":       "Monthly review",
		"description": "Numbers",
		"location":    "Room 3",
		"start":       "2026-08-15 14:00:00.000Z",
		"end":         "2026-08-15 15:00:00.000Z",
		"updated":     "2026-07-26 12:00:00.000Z",
		"recurrence":  rrule,
		"busy_status": "free",
		"visibility":  "private",
		"reminder":    45,
		"guests": []map[string]any{
			{"name": "Ada", "email": "ada@example.com", "rsvp": "accepted", "role": "organizer"},
		},
	})

	decoded := newEventRecord(t, nil)
	if err := applyCalendarToRecord(
		decodeFromBytes(t, encodeToBytes(t, original)), decoded, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}

	for _, f := range []string{"title", "description", "location", "recurrence", "busy_status", "visibility"} {
		if got, want := decoded.GetString(f), original.GetString(f); got != want {
			t.Errorf("%s = %q after a bytes round trip, want %q", f, got, want)
		}
	}
	if got := decoded.GetFloat("reminder"); got != 45 {
		t.Errorf("reminder = %v, want 45", got)
	}
	var guests []guest
	if err := decoded.UnmarshalJSONField("guests", &guests); err != nil {
		t.Fatalf("unmarshal guests: %v", err)
	}
	if len(guests) != 1 || guests[0].Email != "ada@example.com" || guests[0].Role != "organizer" {
		t.Errorf("guests did not survive the bytes round trip: %+v", guests)
	}
}

// A simple token must still emit a clean FREQ= rule (not VALUE=TEXT) and decode
// back to the token.
func TestWire_SimpleTokenRoundTrip(t *testing.T) {
	original := newEventRecord(t, map[string]any{
		"ical_uid":   "wire-4",
		"title":      "Daily",
		"start":      "2026-07-28 09:00:00.000Z",
		"end":        "2026-07-28 09:15:00.000Z",
		"updated":    "2026-07-26 12:00:00.000Z",
		"recurrence": "daily",
	})

	out := encodeToBytes(t, original)
	if !strings.Contains(out, "RRULE:FREQ=DAILY") {
		t.Errorf("expected a clean RRULE:FREQ=DAILY line, got:\n%s", out)
	}

	decoded := newEventRecord(t, nil)
	if err := applyCalendarToRecord(decodeFromBytes(t, out), decoded, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}
	if got := decoded.GetString("recurrence"); got != "daily" {
		t.Errorf("recurrence = %q, want the token 'daily' back", got)
	}
}

// Removing the recurrence must clear the field, over the wire too.
func TestWire_AbsentRRuleClearsStoredValue(t *testing.T) {
	record := newEventRecord(t, map[string]any{
		"ical_uid":   "wire-5",
		"recurrence": "weekly",
	})
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//EN\r\nBEGIN:VEVENT\r\n" +
		"UID:wire-5\r\nSUMMARY:No longer repeating\r\nDTSTAMP:20260726T120000Z\r\n" +
		"DTSTART:20260728T090000Z\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"

	if err := applyCalendarToRecord(decodeFromBytes(t, raw), record, testEventMap); err != nil {
		t.Fatalf("applyCalendarToRecord: %v", err)
	}
	if got := record.GetString("recurrence"); got != "" {
		t.Errorf("recurrence = %q, want cleared", got)
	}
}
