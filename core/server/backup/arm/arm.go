// Package arm stages a restore for the boot swap. Its files are the contract
// between whoever stages a restore (the app itself, or another process that
// stopped the app) and the boot swap that applies it.
package arm

import (
	"archive/tar"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // the driver the staged-database integrity check opens with

	"tinycld.org/core/backup/format"
)

// StagedSentinel is written into a staging directory once the archive verified
// and the staged database passed its integrity check. It is what tells the boot
// swap that staging FINISHED, which cannot be inferred from the staged members:
// the swap moves them out of pending one at a time, so after it starts their
// absence means the opposite of what it means before.
const StagedSentinel = ".staged"

// Marker is the marker a restore leaves behind for the process that boots next.
// It carries the manifest because that process finalizes the ledger row and has
// no other way to know what it is now running.
type Marker struct {
	ID       string          `json:"id"`
	Pending  string          `json:"pending"`
	Pre      string          `json:"pre"`
	Manifest format.Manifest `json:"manifest"`
}

// Dir is the restore root: the parent of pb_data in every deployment shape,
// so it can be derived before an app (or any other process) exists.
func Dir(dataDir string) string { return filepath.Join(filepath.Dir(dataDir), "restore") }

// PendingDir is where one restore's members are staged before the boot swap
// moves them into pb_data.
func PendingDir(restoreDir, id string) string { return filepath.Join(restoreDir, "pending", id) }

// MarkerPath is where the armed marker lives, beside the staging directories.
func MarkerPath(restoreDir string) string { return filepath.Join(restoreDir, "armed") }

// WriteMarker writes the armed marker atomically, so the boot swap never reads
// a half-written one.
func WriteMarker(restoreDir string, m Marker) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(restoreDir, 0o700); err != nil {
		return err
	}
	tmp := MarkerPath(restoreDir) + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, MarkerPath(restoreDir))
}

// CheckCounts compares each collection's rows in the staged DB with the
// manifest. Only a manifest whose counts were read from the same DB copy may
// be checked this way.
func CheckCounts(dbPath string, want format.Manifest) error {
	db, err := sql.Open("sqlite", "file:"+dbPath+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	for name, n := range want.Counts.Collections {
		if strings.Contains(name, "`") {
			continue
		}
		var got int
		if err := db.QueryRow("SELECT count(*) FROM `" + name + "`").Scan(&got); err != nil {
			return fmt.Errorf("backup: count %s in the staged database: %w", name, err)
		}
		if got != n {
			return fmt.Errorf("backup: the staged database has %d rows in %s, the manifest says %d", got, name, n)
		}
	}
	return nil
}

// MarkStaged checks the staged DB and then writes the sentinel the boot swap
// trusts. counts is nil when the manifest's counts cannot be compared exactly.
func MarkStaged(pending string, counts *format.Manifest) error {
	db := filepath.Join(pending, format.MemberDB)
	if err := IntegrityCheck(db); err != nil {
		return err
	}
	if counts != nil {
		if err := CheckCounts(db, *counts); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(pending, StagedSentinel), nil, 0o644)
}

// Arm checks the staged database and, only once that passes, writes the armed
// marker. The marker must never name a pending directory whose counts were
// never checked.
func Arm(restoreDir string, m Marker) error {
	if err := MarkStaged(m.Pending, &m.Manifest); err != nil {
		return err
	}
	return WriteMarker(restoreDir, m)
}

// IntegrityCheck refuses a staged database SQLite cannot read. Without it a
// truncated or corrupt archive gets swapped in and the process restart-loops on
// a database that will never open, with the live copy already moved aside.
func IntegrityCheck(path string) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return fmt.Errorf("backup: open the staged database: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()
	rows, err := db.Query("PRAGMA integrity_check")
	if err != nil {
		return fmt.Errorf("backup: check the staged database: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var problems []string
	for rows.Next() {
		var line string
		if err := rows.Scan(&line); err != nil {
			return fmt.Errorf("backup: check the staged database: %w", err)
		}
		problems = append(problems, line)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("backup: check the staged database: %w", err)
	}
	if len(problems) == 1 && problems[0] == "ok" {
		return nil
	}
	return fmt.Errorf("backup: the staged database failed its integrity check: %s", strings.Join(problems, "; "))
}

// WriteMember stages one member with the permissions PocketBase itself writes
// under pb_data. The staged tree BECOMES pb_data, so staging it tighter would
// leave a restored deployment with a database and a storage tree no other
// process or user on the host could read.
func WriteMember(target string, body io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// ExtractTar writes a tar stream of stored files into dir. Only regular files
// are accepted, and no name may leave dir.
func ExtractTar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	clean := filepath.Clean(dir)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("%w: member %q is not a regular file", format.ErrFormat, hdr.Name)
		}
		target := filepath.Join(clean, filepath.FromSlash(hdr.Name))
		if !strings.HasPrefix(target, clean+string(os.PathSeparator)) {
			return fmt.Errorf("%w: member %q escapes the staging directory", format.ErrFormat, hdr.Name)
		}
		if err := WriteMember(target, tr); err != nil {
			return err
		}
	}
}
