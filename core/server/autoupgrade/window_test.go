package autoupgrade

import (
	"testing"
	"time"
)

func at(h, m int) time.Time { return time.Date(2026, 10, 1, h, m, 0, 0, time.Local) }

func TestParseWindow(t *testing.T) {
	w, err := ParseWindow("02:00-05:30")
	if err != nil {
		t.Fatal(err)
	}
	if w.Start != 2*time.Hour || w.End != 5*time.Hour+30*time.Minute {
		t.Fatalf("got %+v", w)
	}
	for _, bad := range []string{"", "2-5", "24:00-01:00", "02:00-02:00", "02:00_05:00", "02:60-03:00"} {
		if _, err := ParseWindow(bad); err == nil {
			t.Errorf("ParseWindow(%q) accepted", bad)
		}
	}
}

func TestWindowContains(t *testing.T) {
	day, _ := ParseWindow("02:00-05:00")
	cases := map[time.Time]bool{at(1, 59): false, at(2, 0): true, at(4, 59): true, at(5, 0): false}
	for tm, want := range cases {
		if got := day.Contains(tm); got != want {
			t.Errorf("day.Contains(%s) = %v", tm.Format("15:04"), got)
		}
	}
	night, _ := ParseWindow("23:00-01:00")
	for tm, want := range map[time.Time]bool{at(22, 59): false, at(23, 30): true, at(0, 30): true, at(1, 0): false} {
		if got := night.Contains(tm); got != want {
			t.Errorf("night.Contains(%s) = %v", tm.Format("15:04"), got)
		}
	}
}

func TestWindowNextStart(t *testing.T) {
	w, _ := ParseWindow("02:00-05:00")
	if got := w.NextStart(at(1, 0)); !got.Equal(at(2, 0)) {
		t.Errorf("before window: %s", got)
	}
	if got := w.NextStart(at(3, 0)); !got.Equal(at(2, 0).Add(24 * time.Hour)) {
		t.Errorf("inside window: %s", got)
	}
}
