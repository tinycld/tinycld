package caldav

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-ical"
	"github.com/pocketbase/pocketbase/core"
)

// pbTimeFormat is PocketBase's stored datetime layout.
const pbTimeFormat = "2006-01-02 15:04:05.000Z"

// productID identifies this implementation in emitted VCALENDAR objects.
const productID = "-//TinyCld//CalDAV//EN"

// guest is the wire shape of one entry in the event's attendee json field.
type guest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	RSVP  string `json:"rsvp"`
	Role  string `json:"role"`
}

// recordToCalendar converts an event record to an iCalendar object, reading
// every field through the Source's EventMap. An unmapped field (empty name in
// the map) is simply not emitted.
func recordToCalendar(record *core.Record, m EventMap) (*ical.Calendar, error) {
	cal := ical.NewCalendar()
	cal.Props.SetText(ical.PropProductID, productID)
	cal.Props.SetText(ical.PropVersion, "2.0")

	event := ical.NewEvent()

	uid := record.GetString(m.UID)
	if uid == "" {
		return nil, fmt.Errorf("record %s missing a %s value", record.Id, m.UID)
	}
	event.Props.SetText(ical.PropUID, uid)

	setTextIfPresent(event, ical.PropSummary, record, m.Title)
	setTextIfPresent(event, ical.PropDescription, record, m.Description)
	setTextIfPresent(event, ical.PropLocation, record, m.Location)

	allDay := m.AllDay != "" && record.GetBool(m.AllDay)

	if m.Start != "" {
		if t, ok := parsePBTime(record.GetString(m.Start)); ok {
			setEventTime(event, ical.PropDateTimeStart, t, allDay)
		}
	}
	if m.End != "" {
		if t, ok := parsePBTime(record.GetString(m.End)); ok {
			setEventTime(event, ical.PropDateTimeEnd, t, allDay)
		}
	}

	if m.Recurrence != "" {
		if rrule := recurrenceToRRule(record.GetString(m.Recurrence)); rrule != "" {
			// Set the RECUR value verbatim rather than via SetText. SetText
			// stamps VALUE=TEXT and escapes the separators, emitting
			// `RRULE;VALUE=TEXT:FREQ=WEEKLY\;BYDAY=TU` — malformed to every
			// client, since an RRULE's `;` are structural, not literal text.
			prop := ical.NewProp(ical.PropRecurrenceRule)
			prop.Value = rrule
			event.Props.Set(prop)
		}
	}

	// TRANSP is always emitted: a client that sees no TRANSP assumes OPAQUE,
	// so staying silent would misreport a "free" event as busy.
	if m.BusyStatus != "" {
		if record.GetString(m.BusyStatus) == "free" {
			event.Props.SetText(ical.PropTransparency, "TRANSPARENT")
		} else {
			event.Props.SetText(ical.PropTransparency, "OPAQUE")
		}
	}

	if m.Visibility != "" {
		if vis := record.GetString(m.Visibility); vis != "" && vis != "default" {
			event.Props.SetText(ical.PropClass, strings.ToUpper(vis))
		}
	}

	addGuests(event, record, m)
	addAlarm(event, record, m)

	if m.Updated != "" {
		if t, ok := parsePBTime(record.GetString(m.Updated)); ok {
			event.Props.SetDateTime(ical.PropDateTimeStamp, t.UTC())
			event.Props.SetDateTime(ical.PropLastModified, t.UTC())
		}
	}
	if m.Created != "" {
		if t, ok := parsePBTime(record.GetString(m.Created)); ok {
			event.Props.SetDateTime(ical.PropCreated, t.UTC())
		}
	}

	cal.Children = append(cal.Children, event.Component)
	return cal, nil
}

// applyCalendarToRecord extracts VEVENT fields from an iCalendar object and
// applies them to a record through the EventMap. Unmapped fields are skipped,
// so a package that maps only a subset never has stray values written.
func applyCalendarToRecord(cal *ical.Calendar, record *core.Record, m EventMap) error {
	events := cal.Events()
	if len(events) == 0 {
		return fmt.Errorf("no VEVENT found in calendar data")
	}
	event := events[0]

	if m.Title != "" {
		if summary, err := event.Props.Text(ical.PropSummary); err == nil && summary != "" {
			record.Set(m.Title, summary)
		}
	}
	if m.Description != "" {
		if desc, err := event.Props.Text(ical.PropDescription); err == nil {
			record.Set(m.Description, desc)
		}
	}
	if m.Location != "" {
		if loc, err := event.Props.Text(ical.PropLocation); err == nil {
			record.Set(m.Location, loc)
		}
	}

	allDay := false
	if dtStart := event.Props.Get(ical.PropDateTimeStart); dtStart != nil {
		if dtStart.Params.Get("VALUE") == "DATE" {
			allDay = true
		}
		if t, err := dtStart.DateTime(time.UTC); err == nil && m.Start != "" {
			record.Set(m.Start, t.UTC().Format(pbTimeFormat))
		}
	}
	if dtEnd := event.Props.Get(ical.PropDateTimeEnd); dtEnd != nil {
		if t, err := dtEnd.DateTime(time.UTC); err == nil && m.End != "" {
			record.Set(m.End, t.UTC().Format(pbTimeFormat))
		}
	}
	if m.AllDay != "" {
		record.Set(m.AllDay, allDay)
	}

	// RRULE is cleared when absent, not left alone: a client removing the
	// recurrence sends the VEVENT without one, and keeping the old value would
	// silently refuse to un-repeat the event.
	//
	// Read .Value directly rather than Props.Text: a DECODED RRULE carries value
	// type RECUR, and Text() rejects anything that is not TEXT — so it returns ""
	// for every recurrence a real client sends, silently dropping the rule. (A
	// rule set in-process via SetText is TEXT and would have worked, which is
	// what made this invisible to a hand-built test.)
	if m.Recurrence != "" {
		rrule := ""
		if prop := event.Props.Get(ical.PropRecurrenceRule); prop != nil {
			rrule = strings.TrimSpace(prop.Value)
		}
		if rrule != "" {
			record.Set(m.Recurrence, rruleToRecurrence(rrule))
		} else {
			record.Set(m.Recurrence, "")
		}
	}

	if m.BusyStatus != "" {
		if transp, err := event.Props.Text(ical.PropTransparency); err == nil {
			if strings.EqualFold(transp, "TRANSPARENT") {
				record.Set(m.BusyStatus, "free")
			} else {
				record.Set(m.BusyStatus, "busy")
			}
		}
	}

	if m.Visibility != "" {
		if class, err := event.Props.Text(ical.PropClass); err == nil && class != "" {
			switch strings.ToLower(class) {
			case "public":
				record.Set(m.Visibility, "public")
			case "private", "confidential":
				record.Set(m.Visibility, "private")
			default:
				record.Set(m.Visibility, "default")
			}
		}
	}

	applyAttendees(event, record, m)
	applyAlarm(event, record, m)

	return nil
}

func setTextIfPresent(event *ical.Event, prop string, record *core.Record, field string) {
	if field == "" {
		return
	}
	if v := record.GetString(field); v != "" {
		event.Props.SetText(prop, v)
	}
}

func parsePBTime(v string) (time.Time, bool) {
	if v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(pbTimeFormat, v)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func setEventTime(event *ical.Event, propName string, t time.Time, allDay bool) {
	prop := ical.NewProp(propName)
	if allDay {
		prop.SetDate(t)
		prop.Params.Set("VALUE", "DATE")
	} else {
		prop.SetDateTime(t.UTC())
	}
	event.Props.Set(prop)
}

func addGuests(event *ical.Event, record *core.Record, m EventMap) {
	if m.Guests == "" {
		return
	}
	raw := record.Get(m.Guests)
	if raw == nil {
		return
	}

	// The json field arrives as types.JSONRaw or a decoded any depending on how
	// the record was loaded, so round-trip through encoding/json rather than
	// asserting a concrete type.
	data, err := json.Marshal(raw)
	if err != nil {
		return
	}
	var guests []guest
	if err := json.Unmarshal(data, &guests); err != nil {
		return
	}

	for _, g := range guests {
		if g.Email == "" {
			continue
		}
		prop := ical.NewProp(ical.PropAttendee)
		prop.Value = "mailto:" + g.Email
		if g.Name != "" {
			prop.Params.Set(ical.ParamCommonName, g.Name)
		}
		if g.Role == "organizer" {
			prop.Params.Set(ical.ParamRole, "CHAIR")
		} else {
			prop.Params.Set(ical.ParamRole, "REQ-PARTICIPANT")
		}
		switch g.RSVP {
		case "accepted":
			prop.Params.Set(ical.ParamParticipationStatus, "ACCEPTED")
		case "declined":
			prop.Params.Set(ical.ParamParticipationStatus, "DECLINED")
		case "tentative":
			prop.Params.Set(ical.ParamParticipationStatus, "TENTATIVE")
		default:
			prop.Params.Set(ical.ParamParticipationStatus, "NEEDS-ACTION")
		}
		event.Props.Add(prop)
	}
}

func applyAttendees(event ical.Event, record *core.Record, m EventMap) {
	if m.Guests == "" {
		return
	}
	attendeeProps := event.Props.Values(ical.PropAttendee)
	if len(attendeeProps) == 0 {
		return
	}

	var guests []guest
	for _, prop := range attendeeProps {
		email := strings.TrimPrefix(prop.Value, "mailto:")
		if email == "" {
			continue
		}

		g := guest{
			Name:  prop.Params.Get(ical.ParamCommonName),
			Email: email,
		}

		switch strings.ToUpper(prop.Params.Get(ical.ParamParticipationStatus)) {
		case "ACCEPTED":
			g.RSVP = "accepted"
		case "DECLINED":
			g.RSVP = "declined"
		case "TENTATIVE":
			g.RSVP = "tentative"
		default:
			g.RSVP = "pending"
		}

		if strings.ToUpper(prop.Params.Get(ical.ParamRole)) == "CHAIR" {
			g.Role = "organizer"
		} else {
			g.Role = "attendee"
		}

		guests = append(guests, g)
	}

	record.Set(m.Guests, guests)
}

func addAlarm(event *ical.Event, record *core.Record, m EventMap) {
	if m.Reminder == "" {
		return
	}
	reminder := record.GetFloat(m.Reminder)
	if reminder <= 0 {
		return
	}

	alarm := ical.NewComponent(ical.CompAlarm)
	alarm.Props.SetText(ical.PropAction, "DISPLAY")
	alarm.Props.SetText(ical.PropDescription, "Reminder")

	trigger := ical.NewProp(ical.PropTrigger)
	trigger.Value = fmt.Sprintf("-PT%dM", int(reminder))
	alarm.Props.Set(trigger)

	event.Component.Children = append(event.Component.Children, alarm)
}

func applyAlarm(event ical.Event, record *core.Record, m EventMap) {
	if m.Reminder == "" {
		return
	}
	for _, child := range event.Component.Children {
		if child.Name != ical.CompAlarm {
			continue
		}
		trigger := child.Props.Get(ical.PropTrigger)
		if trigger == nil {
			continue
		}
		if mins := parseTriggerMinutes(trigger.Value); mins > 0 {
			record.Set(m.Reminder, mins)
			return
		}
	}
}

// recurrenceToRRule converts a stored recurrence value to an RRULE string.
// Simple tokens (daily, weekly, …) map to FREQ=X; anything else is already an
// RRULE from a CalDAV client and is returned as-is.
func recurrenceToRRule(rec string) string {
	switch strings.ToLower(rec) {
	case "daily":
		return "FREQ=DAILY"
	case "weekly":
		return "FREQ=WEEKLY"
	case "monthly":
		return "FREQ=MONTHLY"
	case "yearly":
		return "FREQ=YEARLY"
	case "":
		return ""
	default:
		return rec
	}
}

// rruleToRecurrence converts an RRULE string back to a stored value. FREQ-only
// rules become tokens; complex rules (BYDAY, UNTIL, INTERVAL, …) are stored
// verbatim for round-trip fidelity.
func rruleToRecurrence(rrule string) string {
	switch strings.ToUpper(strings.TrimSpace(rrule)) {
	case "FREQ=DAILY":
		return "daily"
	case "FREQ=WEEKLY":
		return "weekly"
	case "FREQ=MONTHLY":
		return "monthly"
	case "FREQ=YEARLY":
		return "yearly"
	default:
		return rrule
	}
}

// parseTriggerMinutes parses an iCal TRIGGER duration ("-PT15M", "-PT1H",
// "-P1D") into whole minutes. Seconds are ignored — the smallest unit a
// reminder field carries is a minute.
func parseTriggerMinutes(value string) int {
	v := strings.TrimPrefix(value, "-")
	v = strings.TrimPrefix(v, "+")
	v = strings.TrimPrefix(v, "PT")
	v = strings.TrimPrefix(v, "P")

	total := 0
	num := 0
	for _, c := range v {
		if c >= '0' && c <= '9' {
			num = num*10 + int(c-'0')
			continue
		}
		switch c {
		case 'W':
			total += num * 7 * 24 * 60
		case 'D':
			total += num * 24 * 60
		case 'H':
			total += num * 60
		case 'M':
			total += num
		case 'S':
			// ignored: sub-minute precision has nowhere to go
		case 'T':
			// time separator; carries no value of its own
			continue
		}
		num = 0
	}
	return total
}

// ---------------------------------------------------------------------------
// Exported codec surface
//
// A package that ingests iCalendar outside the protocol path — an ICS
// subscription poller, an importer — needs the same record↔VEVENT translation
// the CalDAV handler uses. Exporting it is what keeps there being ONE
// definition: a second copy in the feature would drift from what clients see
// over /caldav.
// ---------------------------------------------------------------------------

// ApplyVEvent writes the first VEVENT of cal onto record through m. Returns an
// error when cal carries no VEVENT.
//
// The caller owns the save, so it can choose Save vs SaveNoValidate and stamp
// its own fields (calendar, owner, UID) first.
func ApplyVEvent(cal *ical.Calendar, record *core.Record, m EventMap) error {
	return applyCalendarToRecord(cal, record, m)
}

// EncodeVEvent renders record as a single-VEVENT VCALENDAR through m.
func EncodeVEvent(record *core.Record, m EventMap) (*ical.Calendar, error) {
	return recordToCalendar(record, m)
}

// RecurrenceToRRule converts a stored recurrence value to an RRULE string,
// mapping the simple tokens ("daily"…"yearly") and passing anything else
// through. Exported so a reminder scheduler expands the same rule the protocol
// emits.
func RecurrenceToRRule(recurrence string) string {
	return recurrenceToRRule(recurrence)
}

// RRuleToRecurrence converts an RRULE string back to a stored recurrence value.
func RRuleToRecurrence(rrule string) string {
	return rruleToRecurrence(rrule)
}
