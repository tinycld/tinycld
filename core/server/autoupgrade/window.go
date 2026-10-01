package autoupgrade

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// MinWindow is the shortest window accepted. An upgrade rebuilds and restarts
// the app, and an hourly tick must land inside the window at least once.
const MinWindow = time.Hour

var windowPattern = regexp.MustCompile(`^([01]\d|2[0-3]):([0-5]\d)-([01]\d|2[0-3]):([0-5]\d)$`)

// Window is a daily span in server-local time. End before Start means the
// span crosses midnight.
type Window struct {
	Start, End time.Duration
}

func ParseWindow(s string) (Window, error) {
	m := windowPattern.FindStringSubmatch(s)
	if m == nil {
		return Window{}, fmt.Errorf("window %q: want HH:MM-HH:MM", s)
	}
	clock := func(h, mm string) time.Duration {
		hi, _ := strconv.Atoi(h)
		mi, _ := strconv.Atoi(mm)
		return time.Duration(hi)*time.Hour + time.Duration(mi)*time.Minute
	}
	w := Window{Start: clock(m[1], m[2]), End: clock(m[3], m[4])}
	if w.Start == w.End {
		return Window{}, fmt.Errorf("window %q: start and end are the same", s)
	}
	if w.Length() < MinWindow {
		return Window{}, fmt.Errorf("window %q: must be at least 60 minutes long", s)
	}
	return w, nil
}

// Length is the span of the window, counting across midnight when End is
// before Start.
func (w Window) Length() time.Duration {
	if w.End > w.Start {
		return w.End - w.Start
	}
	return 24*time.Hour - w.Start + w.End
}

func sinceMidnight(t time.Time) time.Duration {
	h, m, s := t.Clock()
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(s)*time.Second
}

func (w Window) Contains(t time.Time) bool {
	d := sinceMidnight(t)
	if w.Start < w.End {
		return d >= w.Start && d < w.End
	}
	return d >= w.Start || d < w.End
}

// NextStart is the first window start strictly after `after`.
func (w Window) NextStart(after time.Time) time.Time {
	y, mo, d := after.Date()
	start := time.Date(y, mo, d, 0, 0, 0, 0, after.Location()).Add(w.Start)
	if !start.After(after) {
		start = start.Add(24 * time.Hour)
	}
	return start
}
