package backup

import (
	"fmt"
	"os"
	"path/filepath"
)

// availableBytes reports the free space on the filesystem holding path. It is a
// package var so a test can inject a filesystem of any size; nothing else writes
// it.
var availableBytes = statfsAvailable

// ErrNoSpace is what a run reports when the estimate does not fit. It is a
// separate error so a caller can tell "this deployment cannot hold the copy" from
// a transfer that failed halfway.
var errNoSpaceFormat = "backup: not enough free space: need about %s, have %s"

// requireFreeSpace refuses before it writes. A backup writes a whole database
// snapshot and a restore writes a pre-restore copy plus the staged archive; a
// disk that fills mid-run leaves a truncated snapshot, a half-staged restore, or
// — worst — a pb_data that cannot be rolled back to because the safety copy did
// not fit either.
//
// The estimate is deliberately generous about being wrong in one direction only:
// it is what the operation writes at its peak, not its average.
func requireFreeSpace(dir string, need int64) error {
	if need <= 0 {
		return nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	have, err := availableBytes(dir)
	if err != nil {
		// A platform that cannot answer must not block a backup: the run may
		// well fit, and refusing every run on such a host would be worse than
		// letting one fail on a full disk.
		log.Warn("could not read the free space for a backup", "dir", dir, "err", err)
		return nil
	}
	if have < need {
		return fmt.Errorf(errNoSpaceFormat, humanBytes(need), humanBytes(have))
	}
	return nil
}

// liveDatabaseBytes is what the snapshot will be at most: VACUUM INTO writes a
// compacted copy, so the live file's size is an upper bound.
func liveDatabaseBytes(app coreDataDir) int64 {
	var total int64
	for _, name := range []string{"data.db", "data.db-wal"} {
		fi, err := os.Stat(filepath.Join(app.DataDir(), name))
		if err != nil {
			continue
		}
		total += fi.Size()
	}
	return total
}

// coreDataDir is the one thing the estimate needs from the app, named so the
// helper can be exercised without a whole PocketBase.
type coreDataDir interface{ DataDir() string }

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	value, exp := float64(n), 0
	for value >= unit && exp < 4 {
		value /= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", value, "KMGT"[exp-1])
}
