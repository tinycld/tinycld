package tenantcfg

import "tinycld.org/core/caldav"

// The CalDAV wire format, mirroring tinycld.org/core/caldav's Source rather than
// marshalling it directly — same rationale as the CardDAV types in davconfig.go:
// core's types carry no JSON tags, so encoding them would silently depend on Go
// field names staying stable across a repo boundary.
//
// Source.OnError is deliberately absent. It is a Go func value, so it cannot
// cross a process boundary; a tenant reports CalDAV errors through its own
// logging rather than the host's Sentry hub.

type CalDAVSource struct {
	Slug               string      `json:"slug"`
	Prefix             string      `json:"prefix"`
	CalendarCollection string      `json:"calendarCollection"`
	EventCollection    string      `json:"eventCollection"`
	Calendar           CalendarMap `json:"calendar"`
	Event              EventMap    `json:"event"`
}

type CalendarMap struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type EventMap struct {
	Calendar    string `json:"calendar"`
	UID         string `json:"uid"`
	Owner       string `json:"owner"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       string `json:"start"`
	End         string `json:"end"`
	AllDay      string `json:"allDay"`
	Recurrence  string `json:"recurrence"`
	Guests      string `json:"guests"`
	Reminder    string `json:"reminder"`
	BusyStatus  string `json:"busyStatus"`
	Visibility  string `json:"visibility"`
	Updated     string `json:"updated"`
	Created     string `json:"created"`

	// Defaults must cross the wire: they cover schema-required fields a minimal
	// VEVENT omits, so a tenant without them rejects those PUTs outright.
	Defaults map[string]any `json:"defaults"`
}

// EncodeCalDAV converts core's Sources into the wire form.
func EncodeCalDAV(sources []caldav.Source) []CalDAVSource {
	out := make([]CalDAVSource, 0, len(sources))
	for _, s := range sources {
		out = append(out, CalDAVSource{
			Slug:               s.Slug,
			Prefix:             s.Prefix,
			CalendarCollection: s.CalendarCollection,
			EventCollection:    s.EventCollection,
			Calendar: CalendarMap{
				Name:        s.Calendar.Name,
				Description: s.Calendar.Description,
			},
			Event: EventMap{
				Calendar:    s.Event.Calendar,
				UID:         s.Event.UID,
				Owner:       s.Event.Owner,
				Title:       s.Event.Title,
				Description: s.Event.Description,
				Location:    s.Event.Location,
				Start:       s.Event.Start,
				End:         s.Event.End,
				AllDay:      s.Event.AllDay,
				Recurrence:  s.Event.Recurrence,
				Guests:      s.Event.Guests,
				Reminder:    s.Event.Reminder,
				BusyStatus:  s.Event.BusyStatus,
				Visibility:  s.Event.Visibility,
				Updated:     s.Event.Updated,
				Created:     s.Event.Created,
				Defaults:    s.Event.Defaults,
			},
		})
	}
	return out
}

// DecodeCalDAV converts the wire form back into core's Sources.
func DecodeCalDAV(sources []CalDAVSource) []caldav.Source {
	out := make([]caldav.Source, 0, len(sources))
	for _, s := range sources {
		out = append(out, caldav.Source{
			Slug:               s.Slug,
			Prefix:             s.Prefix,
			CalendarCollection: s.CalendarCollection,
			EventCollection:    s.EventCollection,
			Calendar: caldav.CalendarMap{
				Name:        s.Calendar.Name,
				Description: s.Calendar.Description,
			},
			Event: caldav.EventMap{
				Calendar:    s.Event.Calendar,
				UID:         s.Event.UID,
				Owner:       s.Event.Owner,
				Title:       s.Event.Title,
				Description: s.Event.Description,
				Location:    s.Event.Location,
				Start:       s.Event.Start,
				End:         s.Event.End,
				AllDay:      s.Event.AllDay,
				Recurrence:  s.Event.Recurrence,
				Guests:      s.Event.Guests,
				Reminder:    s.Event.Reminder,
				BusyStatus:  s.Event.BusyStatus,
				Visibility:  s.Event.Visibility,
				Updated:     s.Event.Updated,
				Created:     s.Event.Created,
				Defaults:    s.Event.Defaults,
			},
		})
	}
	return out
}
