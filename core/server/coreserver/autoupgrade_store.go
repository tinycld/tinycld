package coreserver

import (
	"time"

	"github.com/pocketbase/pocketbase/core"
)

const reminderEvery = 7 * 24 * time.Hour

// notice is one email, before it is addressed. Kept apart from the send so the
// gate can be tested by counting notices.
type notice struct {
	Subject  string
	BodyText string
}

func notifyDue(lastNotified time.Time, sameFingerprint bool, now time.Time) bool {
	return !sameFingerprint || now.Sub(lastNotified) >= reminderEvery
}

func stateCollection(app core.App) (*core.Collection, error) {
	return app.FindCollectionByNameOrId("autoupgrade_state")
}

// recordPause keeps the one pause row in step with the latest conflict. A
// repeat of the same conflict is silent until the 7-day reminder.
func recordPause(app core.App, fp string, wanted map[string]string, reason string, now time.Time, notify func(notice)) error {
	row, err := app.FindFirstRecordByFilter("autoupgrade_state", "kind = 'pause'")
	if err != nil {
		col, cErr := stateCollection(app)
		if cErr != nil {
			return cErr
		}
		row = core.NewRecord(col)
		row.Set("kind", "pause")
		row.Set("first_seen", now)
		row.Set("fingerprint", "")
	}
	same := row.GetString("fingerprint") == fp
	if !notifyDue(row.GetDateTime("last_notified").Time(), same, now) {
		return nil
	}
	if !same {
		row.Set("first_seen", now)
	}
	row.Set("fingerprint", fp)
	row.Set("target", wanted)
	row.Set("reason", reason)
	row.Set("last_notified", now)
	if err := app.Save(row); err != nil {
		return err
	}
	notify(pauseNotice(wanted, reason, same))
	return nil
}

func clearPause(app core.App) error {
	rows, err := app.FindRecordsByFilter("autoupgrade_state", "kind = 'pause'", "", 0, 0)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if err := app.Delete(r); err != nil {
			return err
		}
	}
	return nil
}

// recordBlocked writes one blocked row per rolled-back job. The install log id
// makes it idempotent: a boot that runs the reconcile twice sends one email.
func recordBlocked(app core.App, fp string, target map[string]string, reason, installLogID string, now time.Time, notify func(notice)) error {
	if _, err := app.FindFirstRecordByFilter("autoupgrade_state",
		"kind = 'blocked' && install_log = {:id}", map[string]any{"id": installLogID}); err == nil {
		return nil
	}
	col, err := stateCollection(app)
	if err != nil {
		return err
	}
	row := core.NewRecord(col)
	row.Set("kind", "blocked")
	row.Set("fingerprint", fp)
	row.Set("target", target)
	row.Set("reason", reason)
	row.Set("install_log", installLogID)
	row.Set("first_seen", now)
	row.Set("last_notified", now)
	if err := app.Save(row); err != nil {
		return err
	}
	notify(blockedNotice(target, reason, false))
	return nil
}

func targetOf(row *core.Record) map[string]string {
	out := map[string]string{}
	_ = row.UnmarshalJSONField("target", &out)
	return out
}

func remindBlocked(app core.App, now time.Time, notify func(notice)) error {
	rows, err := app.FindRecordsByFilter("autoupgrade_state", "kind = 'blocked' && cleared = false", "", 0, 0)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !notifyDue(r.GetDateTime("last_notified").Time(), true, now) {
			continue
		}
		r.Set("last_notified", now)
		if err := app.Save(r); err != nil {
			return err
		}
		notify(blockedNotice(targetOf(r), r.GetString("reason"), true))
	}
	return nil
}

func blockedFingerprints(app core.App) (map[string]bool, error) {
	rows, err := app.FindRecordsByFilter("autoupgrade_state", "kind = 'blocked' && cleared = false", "", 0, 0)
	if err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		out[r.GetString("fingerprint")] = true
	}
	return out, nil
}
