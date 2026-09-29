package core

import "os"

// touchNotifyFile signals the other instances watching the notify dir that a
// shared runtime state changed.
//
// The file is LEFT IN PLACE rather than written-then-immediately-removed. The
// remove used to run on the very next line, and on a kqueue/FSEvents backend
// (macOS/BSD) that races the backend's own event delivery: the inode is gone
// before the create is reported, so fsnotify delivers NOTHING and the peer
// never reloads. Measured directly — write+remove yields zero events, while
// write, brief pause, remove yields CREATE then REMOVE. On Linux/inotify the
// create is queued synchronously, which is why this only ever showed up as a
// platform-dependent flake (TestNotifyWatcher_SettingsUpdate /
// _CollectionsUpdate failing on their 3s timeout).
//
// Leaving the file costs nothing and loses no signal: the watcher keys on the
// filename prefix, not the contents, so a later notify's overwrite is itself
// the next event (WRITE rather than CREATE — both pass the filter, and only
// REMOVE is skipped). OnTerminate already unlinks both files, and the whole
// .notify dir is excluded from backups (base_backup.go).
//
// Do NOT "tidy" this back into a write+remove pair.
func touchNotifyFile(path string) error {
	return os.WriteFile(path, nil, 0644)
}
