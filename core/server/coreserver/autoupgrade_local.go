package coreserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/autoupgrade"
	"tinycld.org/core/installjob"
)

const (
	tickEvery            = time.Hour
	resultDisabledPrefix = "checks disabled: "
)

// localScheduler is the Delegate for a deployment that can rebuild itself. It
// wakes every hour; inside the owner's window it applies the newest compatible
// set through the same path as a manual version change, so the entrypoint's
// health probe commits or rolls back exactly as it does for a person.
type localScheduler struct {
	app      core.App
	now      func() time.Time
	discover func() ([]VersionInfo, error)
	solve    solveFunc
	apply    func([]installjob.VersionChange) error
	notify   func(notice)
	// disabled is why ticks do nothing ("" = active). The switch still works:
	// a development build or a test server must not rebuild itself at 3am.
	disabled string

	mu         sync.Mutex
	lastRun    time.Time
	lastResult string
}

func newLocalScheduler(app *pocketbase.PocketBase) *localScheduler {
	s := &localScheduler{
		app: app,
		now: time.Now,
		discover: func() ([]VersionInfo, error) {
			rows, err := app.FindRecordsByFilter("pkg_registry", "id != ''", "slug", 0, 0)
			if err != nil {
				return nil, err
			}
			return versionInfosForRows(rows, versionsForSpec), nil
		},
		solve: func(c map[string]string) ([]compatViolation, error) { return solveRegistryCompat(app, c) },
		apply: func(c []installjob.VersionChange) error {
			_, err := beginVersionChange(app, c, "auto")
			return err
		},
		notify: func(n notice) {
			notifyAdmins(app, func(name, email, subj, html, text string) { send(app, name, email, subj, html, text) }, n)
		},
	}
	switch {
	case isDevelopment():
		s.disabled = "development build"
	case os.Getenv("TINYCLD_AUTOUPGRADE_DISABLED") == "1":
		s.disabled = "TINYCLD_AUTOUPGRADE_DISABLED is set"
	}
	return s
}

// readSystemSetting returns "" without an error only when the row does not
// exist. Any other failure is returned: a tick must not mistake an unreadable
// switch for "off", or an unreadable window for the default one.
func readSystemSetting(app core.App, key string) (string, error) {
	row, err := app.FindFirstRecordByFilter("system_settings", "key = {:k}", map[string]any{"k": key})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", key, err)
	}
	return row.GetString("value"), nil
}

func (s *localScheduler) window() (autoupgrade.Window, error) {
	raw, err := readSystemSetting(s.app, autoupgrade.KeyWindow)
	if err != nil {
		return autoupgrade.Window{}, err
	}
	if raw == "" {
		raw = autoupgrade.DefaultWindow
	}
	w, err := autoupgrade.ParseWindow(raw)
	if err != nil {
		srvLog.Warn("auto-upgrade: bad window, using default", "window", raw, "err", err)
		w, _ = autoupgrade.ParseWindow(autoupgrade.DefaultWindow)
	}
	return w, nil
}

// PolicyChanged has nothing to push: every tick reads the stored flag.
func (s *localScheduler) PolicyChanged(_ context.Context, enabled bool) error {
	srvLog.Info("auto-upgrade policy", "enabled", enabled)
	return nil
}

func (s *localScheduler) Status(context.Context) (autoupgrade.Status, error) {
	s.mu.Lock()
	lastRun, lastResult := s.lastRun, s.lastResult
	s.mu.Unlock()
	if lastRun.IsZero() {
		lastRun, lastResult = lastAutoJob(s.app)
	}
	w, err := s.window()
	if err != nil {
		return autoupgrade.Status{}, err
	}
	return autoupgrade.Status{
		Available:  true,
		LastRun:    lastRun,
		LastResult: lastResult,
		NextCheck:  w.NextStart(s.now()),
	}, nil
}

// lastAutoJob reads the newest automatic job from the install log, so the
// status survives the restart every upgrade ends with.
func lastAutoJob(app core.App) (time.Time, string) {
	rows, err := app.FindRecordsByFilter("pkg_install_log", "trigger = 'auto'", "-created", 1, 0)
	if err != nil || len(rows) == 0 {
		return time.Time{}, ""
	}
	results := map[string]string{"success": "upgraded", "rolled_back": "rolled back", "failed": "failed", "running": "upgrading"}
	return rows[0].GetDateTime("created").Time(), results[rows[0].GetString("status")]
}

func (s *localScheduler) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(tickEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *localScheduler) record(result string) string {
	s.mu.Lock()
	s.lastRun, s.lastResult = s.now(), result
	s.mu.Unlock()
	return result
}

func (s *localScheduler) tick(_ context.Context) string {
	if s.disabled != "" {
		return s.record(resultDisabledPrefix + s.disabled)
	}
	enabled, err := readSystemSetting(s.app, autoupgrade.KeyEnabled)
	if err != nil {
		return s.record("failed: " + err.Error())
	}
	if enabled != "true" {
		return "off"
	}
	w, err := s.window()
	if err != nil {
		return s.record("failed: " + err.Error())
	}
	now := s.now()
	if !w.Contains(now) {
		return "waiting for window"
	}
	if installjob.Running() {
		return "busy"
	}
	if err := remindBlocked(s.app, now, s.notify); err != nil {
		srvLog.Warn("auto-upgrade: blocked reminders failed", "err", err)
	}

	infos, err := s.discover()
	if err != nil {
		return s.record("failed: " + err.Error())
	}
	plan, err := planUpgrade(infos, s.solve)
	if err != nil {
		return s.record("failed: " + err.Error())
	}
	if plan.Target == nil && len(plan.Violations) > 0 {
		fp := fingerprint(plan.Wanted, plan.Violations)
		if err := recordPause(s.app, fp, plan.Wanted, formatViolations(plan.Violations), now, s.notify); err != nil {
			srvLog.Warn("auto-upgrade: record pause failed", "err", err)
		}
		return s.record("paused: conflict")
	}
	if err := clearPause(s.app); err != nil {
		srvLog.Warn("auto-upgrade: clear pause failed", "err", err)
	}
	if len(plan.Target) == 0 {
		return s.record("no updates")
	}
	blocked, err := blockedFingerprints(s.app)
	if err != nil {
		return s.record("failed: " + err.Error())
	}
	if blocked[fingerprint(plan.Target, nil)] {
		return s.record("blocked")
	}

	changes := make([]installjob.VersionChange, 0, len(plan.Target))
	for _, slug := range sortedKeys(plan.Target) {
		changes = append(changes, installjob.VersionChange{Slug: slug, TargetVersion: plan.Target[slug]})
	}
	if err := s.apply(changes); err != nil {
		srvLog.Warn("auto-upgrade: apply failed", "err", err)
		return s.record("failed: " + err.Error())
	}
	return s.record("upgrading")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
