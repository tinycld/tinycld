package coreserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/approutes"
	"tinycld.org/core/notify"
)

// unrestoredNote mirrors supervise's UnrestoredNote
// (<state>/unrestored/<build>/unrestored.json). It is a copy, not an import,
// so the server does not link the supervisor.
type unrestoredNote struct {
	Build        string    `json:"build"`
	RolledTo     string    `json:"rolled_to"`
	At           time.Time `json:"at"`
	RestoreError string    `json:"restore_error"`
	Size         int64     `json:"size"`

	dir string // <state>/unrestored/<build>
}

const (
	unrestoredNoteName = "unrestored.json"
	unrestoredDataName = "data.db"
	// One marker per channel, so a channel that worked is not repeated
	// when the other is retried on the next boot.
	unrestoredNotifiedAppName   = "notified-app"
	unrestoredNotifiedEmailName = "notified-email"
	unrestoredNotifyType        = "core.backup.unrestored"
	unrestoredHelpTopic         = "help/core/after-a-failed-update"
	// unrestoredNotifyTimeout bounds the push deliveries.
	unrestoredNotifyTimeout = 30 * time.Second
)

// listUnrestored returns every backup a rollback could not restore, oldest
// first. A dir with the copy but no note (a crash between the supervisor's two
// renames) and a dir whose note does not parse are still listed, with only
// Build set from the dir name: a copy no one is told about would be the one
// an operator never deals with. A dir with neither file holds nothing.
func listUnrestored() ([]unrestoredNote, error) {
	root := stateUnrestoredDir()
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("coreserver: list the unrestored backups: %w", err)
	}
	var notes []unrestoredNote
	var errs []error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(root, e.Name())
		fallback := unrestoredNote{Build: e.Name(), dir: dir}
		data, err := os.ReadFile(filepath.Join(dir, unrestoredNoteName))
		if errors.Is(err, os.ErrNotExist) {
			if _, statErr := os.Stat(filepath.Join(dir, unrestoredDataName)); statErr == nil {
				notes = append(notes, fallback)
			}
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("coreserver: read the unrestored note of %s: %w", e.Name(), err))
			notes = append(notes, fallback)
			continue
		}
		var n unrestoredNote
		if err := json.Unmarshal(data, &n); err != nil {
			errs = append(errs, fmt.Errorf("coreserver: decode the unrestored note of %s: %w", e.Name(), err))
			notes = append(notes, fallback)
			continue
		}
		// The dir is what an operator acts on, so it names the build even if
		// the note says otherwise.
		n.Build, n.dir = e.Name(), dir
		notes = append(notes, n)
	}
	slices.SortFunc(notes, func(a, b unrestoredNote) int { return a.At.Compare(b.At) })
	return notes, errors.Join(errs...)
}

// unrestoredReadErrOnce keeps a read failure to one Error per process: the
// status route and the hourly tick read the dir too, and each Error goes to
// Sentry.
var unrestoredReadErrOnce sync.Once

func logUnrestoredReadErr(err error) {
	unrestoredReadErrOnce.Do(func() {
		srvLog.Error("could not read every kept unrestored backup; treating them as present", "err", err)
	})
}

// hasUnrestored reports whether any backup a rollback could not restore is
// kept. A dir that cannot be read counts as one: holding an upgrade is the
// safe side of not knowing.
func hasUnrestored() bool {
	notes, err := listUnrestored()
	if err != nil {
		logUnrestoredReadErr(err)
		return true
	}
	return len(notes) > 0
}

// unrestoredNotice is the text administrators get, in the app and by email.
func unrestoredNotice(n unrestoredNote) notice {
	body := fmt.Sprintf("A database backup could not be restored after a failed update. "+
		"The server is running on data migrated by build %s. "+
		"The backup from before that update is kept in %s. "+
		"See Help → After a failed update.\n\n"+
		"Automatic updates are paused until the backup is put back or deleted.",
		n.Build, n.dir)
	return notice{
		Subject:  "A database backup needs attention",
		BodyText: body,
		CTALabel: "Open the help topic",
		CTAPath:  unrestoredHelpTopic,
	}
}

// reportUnrestored runs once per boot, off the serve path (startBootNotices).
// For each kept backup, it notifies every administrator in the app and by
// email, and writes a marker per channel (notified-app, notified-email) when
// that channel worked. A channel that failed logs at Error and has no marker,
// so the next boot sends only that channel again.
func reportUnrestored(ctx context.Context, app core.App, mail bootMailFn) {
	notes, err := listUnrestored()
	if err != nil {
		logUnrestoredReadErr(err)
	}
	for _, n := range notes {
		msg := unrestoredNotice(n)
		deliverOnce(n, unrestoredNotifiedAppName, "in the app", func() error { return tellAdminsInApp(ctx, app, msg) })
		deliverOnce(n, unrestoredNotifiedEmailName, "by email", func() error { return mail(ctx, app, msg) })
	}
}

// deliverOnce runs deliver unless n's dir has the marker, and writes the
// marker when deliver succeeds.
func deliverOnce(n unrestoredNote, markerName, channel string, deliver func() error) {
	marker := filepath.Join(n.dir, markerName)
	if _, err := os.Stat(marker); err == nil {
		return
	}
	if err := deliver(); err != nil {
		srvLog.Error("could not tell the administrators about an unrestored backup "+channel, "build", n.Build, "dir", n.dir, "err", err)
		return
	}
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		srvLog.Error("could not record that the administrators were told about an unrestored backup", "build", n.Build, "path", marker, "err", err)
	}
}

// tellAdminsInApp fails when no administrator got the notification, so a
// boot before the first owner exists does not count as having told anyone.
func tellAdminsInApp(ctx context.Context, app core.App, msg notice) error {
	ctx, cancel := context.WithTimeout(ctx, unrestoredNotifyTimeout)
	defer cancel()
	delivered, err := notify.AdministratorsContext(ctx, app, notify.NotifyParams{
		Type:    unrestoredNotifyType,
		Package: "core",
		Title:   msg.Subject,
		Body:    msg.BodyText,
		URL:     approutes.Href(unrestoredHelpTopic),
	})
	if err != nil {
		return fmt.Errorf("coreserver: notify the administrators: %w", err)
	}
	if delivered == 0 {
		return errors.New("coreserver: notify the administrators: no administrator received it")
	}
	return nil
}
