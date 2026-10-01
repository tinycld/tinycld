package coreserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/autoupgrade"
	"tinycld.org/core/installjob"
)

func setSetting(t *testing.T, key, value string) func(*localScheduler) {
	return func(s *localScheduler) {
		row, err := s.app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": key})
		if err != nil {
			col, _ := s.app.FindCollectionByNameOrId("system_settings")
			row = core.NewRecord(col)
			row.Set("key", key)
		}
		row.Set("value", value)
		mustNil(t, s.app.Save(row))
	}
}

func testScheduler(t *testing.T, at time.Time, in []VersionInfo, solve solveFunc) (*localScheduler, *[][]installjob.VersionChange, *[]notice) {
	t.Helper()
	app := adminConsoleTestApp(t)
	var applied [][]installjob.VersionChange
	var sent []notice
	s := &localScheduler{
		app:      app,
		now:      func() time.Time { return at },
		discover: func() ([]VersionInfo, error) { return in, nil },
		solve:    solve,
		apply: func(c []installjob.VersionChange) error {
			applied = append(applied, c)
			return nil
		},
		notify: func(n notice) { sent = append(sent, n) },
	}
	return s, &applied, &sent
}

var inWindow = time.Date(2026, 10, 1, 3, 0, 0, 0, time.Local)
var okSolve = func(map[string]string) ([]compatViolation, error) { return nil, nil }

func TestTickAppliesNewestInWindow(t *testing.T) {
	s, applied, _ := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	if got := s.tick(context.Background()); got != "upgrading" {
		t.Fatalf("result %q", got)
	}
	if len(*applied) != 1 || (*applied)[0][0].TargetVersion != "0.6.0" {
		t.Fatalf("applied %+v", *applied)
	}
}

func TestTickSkipsOutsideWindowAndWhenOff(t *testing.T) {
	s, applied, _ := testScheduler(t, time.Date(2026, 10, 1, 12, 0, 0, 0, time.Local), infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	if got := s.tick(context.Background()); got != "waiting for window" {
		t.Fatalf("result %q", got)
	}
	s.now = func() time.Time { return inWindow }
	setSetting(t, autoupgrade.KeyEnabled, "false")(s)
	if got := s.tick(context.Background()); got != "off" {
		t.Fatalf("result %q", got)
	}
	if len(*applied) != 0 {
		t.Fatal("applied while skipped")
	}
}

func TestTickHonorsCustomWindow(t *testing.T) {
	s, _, _ := testScheduler(t, time.Date(2026, 10, 1, 12, 30, 0, 0, time.Local), infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	setSetting(t, autoupgrade.KeyWindow, "12:00-13:00")(s)
	if got := s.tick(context.Background()); got != "upgrading" {
		t.Fatalf("result %q", got)
	}
}

func TestTickPausesOnConflict(t *testing.T) {
	v := []compatViolation{{Package: "mail", Requires: "@tinycld/core", Range: ">=0.6", Found: "0.5.4"}}
	s, applied, sent := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.5.1"}),
		func(map[string]string) ([]compatViolation, error) { return v, nil })
	if got := s.tick(context.Background()); got != "paused: conflict" {
		t.Fatalf("result %q", got)
	}
	s.tick(context.Background())
	if len(*applied) != 0 || len(*sent) != 1 {
		t.Fatalf("applied=%d sent=%d", len(*applied), len(*sent))
	}
}

func TestTickClearsPauseWhenResolved(t *testing.T) {
	v := []compatViolation{{Package: "mail", Requires: "@tinycld/core", Range: ">=0.6", Found: "0.5.4"}}
	conflict := true
	s, _, _ := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.5.1"}),
		func(map[string]string) ([]compatViolation, error) {
			if conflict {
				return v, nil
			}
			return nil, nil
		})
	s.tick(context.Background())
	conflict = false
	if got := s.tick(context.Background()); got != "upgrading" {
		t.Fatalf("result %q", got)
	}
	if rows, _ := s.app.FindRecordsByFilter("autoupgrade_state", "kind = 'pause'", "", 0, 0); len(rows) != 0 {
		t.Fatal("pause row not cleared")
	}
}

func TestTickSkipsBlockedSet(t *testing.T) {
	s, applied, _ := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	logID := newInstallLogRow(t, s.app)
	fp := fingerprint(map[string]string{"mail": "0.6.0"}, nil)
	mustNil(t, recordBlocked(s.app, fp, map[string]string{"mail": "0.6.0"}, "rolled back", logID, inWindow, func(notice) {}))
	if got := s.tick(context.Background()); got != "blocked" {
		t.Fatalf("result %q", got)
	}
	if len(*applied) != 0 {
		t.Fatal("applied a blocked set")
	}
}

func TestTickDisabled(t *testing.T) {
	s, applied, _ := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	s.disabled = "development build"
	if got := s.tick(context.Background()); got != "checks disabled: development build" {
		t.Fatalf("result %q", got)
	}
	if len(*applied) != 0 {
		t.Fatal("applied while disabled")
	}
}

func TestTickApplyErrorIsReported(t *testing.T) {
	s, _, _ := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	s.apply = func([]installjob.VersionChange) error { return errors.New("npm down") }
	if got := s.tick(context.Background()); got != "failed: npm down" {
		t.Fatalf("result %q", got)
	}
	st, err := s.Status(context.Background())
	mustNil(t, err)
	if !st.Available || st.LastResult != "failed: npm down" || st.NextCheck.IsZero() {
		t.Fatalf("status %+v", st)
	}
}

// dropSystemSettings makes every system_settings lookup fail with an error
// other than sql.ErrNoRows. Deleting the collection is not enough here: a
// missing collection is itself reported as sql.ErrNoRows. Dropping the table
// under a live collection makes the real query fail with "no such table".
func dropSystemSettings(t *testing.T, app core.App) {
	t.Helper()
	_, err := app.DB().NewQuery("DROP TABLE system_settings").Execute()
	mustNil(t, err)
}

func TestReadSystemSettingOnlyTreatsNoRowsAsMissing(t *testing.T) {
	app := adminConsoleTestApp(t)
	if v, err := readSystemSetting(app, "autoupgrade.nope"); err != nil || v != "" {
		t.Fatalf("missing row: got %q, %v", v, err)
	}
	if v, err := readSystemSetting(app, autoupgrade.KeyEnabled); err != nil || v != "true" {
		t.Fatalf("seeded row: got %q, %v", v, err)
	}
	dropSystemSettings(t, app)
	if _, err := readSystemSetting(app, autoupgrade.KeyEnabled); err == nil {
		t.Fatal("want an error when the lookup fails")
	}
}

func TestTickSkipsWhenSettingsUnreadable(t *testing.T) {
	s, applied, _ := testScheduler(t, inWindow, infos([3]string{"mail", "0.5.0", "0.6.0"}), okSolve)
	dropSystemSettings(t, s.app)
	got := s.tick(context.Background())
	if !strings.HasPrefix(got, "failed: ") {
		t.Fatalf("result %q", got)
	}
	if len(*applied) != 0 {
		t.Fatal("applied although the switch could not be read")
	}
	if _, err := s.Status(context.Background()); err == nil {
		t.Fatal("Status must report the read failure")
	}
}
