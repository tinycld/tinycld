package caldav

import (
	"context"

	"github.com/emersion/go-ical"
	"github.com/emersion/go-webdav/caldav"
)

// Error reporting.
//
// go-webdav's caldav.Handler converts a backend error into an http.Error
// response and then returns nil, so the error never reaches request middleware.
// This decorator is the only seam where the real error is still available, which
// is why it wraps the interface rather than reporting from inside each method:
// the wrapper cannot miss a method the way scattered call sites can, and adding
// a method to the interface breaks the build here instead of silently going
// unreported.
//
// It is observation only — every method returns the inner result untouched.

// observed wraps a caldav.Backend, forwarding each non-nil error to report.
type observed struct {
	inner  caldav.Backend
	report func(ctx context.Context, op string, err error)
}

// withErrorReporting returns b wrapped so its errors are reported, or b
// unchanged when no reporter is configured — an unwrapped backend keeps the
// call path free of a pointless indirection.
func withErrorReporting(b caldav.Backend, report func(ctx context.Context, op string, err error)) caldav.Backend {
	if report == nil {
		return b
	}
	return &observed{inner: b, report: report}
}

// note reports err, except for the not-found sentinel.
//
// Filtering it matters: every unauthenticated probe and every request for
// someone else's calendar resolves to errNotFound by design, so reporting it
// would bury real failures under routine 404s. A 404 is an outcome here, not a
// bug.
func (o *observed) note(ctx context.Context, op string, err error) {
	if err == nil || IsNotFound(err) {
		return
	}
	o.report(ctx, op, err)
}

func (o *observed) CurrentUserPrincipal(ctx context.Context) (string, error) {
	out, err := o.inner.CurrentUserPrincipal(ctx)
	o.note(ctx, "CurrentUserPrincipal", err)
	return out, err
}

func (o *observed) CalendarHomeSetPath(ctx context.Context) (string, error) {
	out, err := o.inner.CalendarHomeSetPath(ctx)
	o.note(ctx, "CalendarHomeSetPath", err)
	return out, err
}

func (o *observed) CreateCalendar(ctx context.Context, cal *caldav.Calendar) error {
	err := o.inner.CreateCalendar(ctx, cal)
	o.note(ctx, "CreateCalendar", err)
	return err
}

func (o *observed) ListCalendars(ctx context.Context) ([]caldav.Calendar, error) {
	out, err := o.inner.ListCalendars(ctx)
	o.note(ctx, "ListCalendars", err)
	return out, err
}

func (o *observed) GetCalendar(ctx context.Context, path string) (*caldav.Calendar, error) {
	out, err := o.inner.GetCalendar(ctx, path)
	o.note(ctx, "GetCalendar", err)
	return out, err
}

func (o *observed) GetCalendarObject(ctx context.Context, path string, req *caldav.CalendarCompRequest) (*caldav.CalendarObject, error) {
	out, err := o.inner.GetCalendarObject(ctx, path, req)
	o.note(ctx, "GetCalendarObject", err)
	return out, err
}

func (o *observed) ListCalendarObjects(ctx context.Context, path string, req *caldav.CalendarCompRequest) ([]caldav.CalendarObject, error) {
	out, err := o.inner.ListCalendarObjects(ctx, path, req)
	o.note(ctx, "ListCalendarObjects", err)
	return out, err
}

func (o *observed) QueryCalendarObjects(ctx context.Context, path string, query *caldav.CalendarQuery) ([]caldav.CalendarObject, error) {
	out, err := o.inner.QueryCalendarObjects(ctx, path, query)
	o.note(ctx, "QueryCalendarObjects", err)
	return out, err
}

func (o *observed) PutCalendarObject(ctx context.Context, path string, cal *ical.Calendar, opts *caldav.PutCalendarObjectOptions) (*caldav.CalendarObject, error) {
	out, err := o.inner.PutCalendarObject(ctx, path, cal, opts)
	o.note(ctx, "PutCalendarObject", err)
	return out, err
}

func (o *observed) DeleteCalendarObject(ctx context.Context, path string) error {
	err := o.inner.DeleteCalendarObject(ctx, path)
	o.note(ctx, "DeleteCalendarObject", err)
	return err
}

var _ caldav.Backend = (*observed)(nil)
