package snapshot

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"tinycld.org/core/backup/format"
)

var ErrS3Storage = errors.New("backup: storage is on S3; a snapshot from outside the app cannot read it")

// ErrStorageUnknown is a snapshot from outside the app that cannot tell where
// the stored files are, because the app's settings do not decode here (they are
// encrypted with PB_ENCRYPTION). Assuming local storage would back up zero
// files and call it a success.
var ErrStorageUnknown = errors.New("backup: the app's settings cannot be read here, so it is not known whether storage is on S3; back up from inside the app")

// fillManifest reads the package set and row counts from the DB COPY, so they
// describe exactly the data the snapshot holds.
func fillManifest(db *sql.DB, m *format.Manifest) error {
	rows, err := db.Query("SELECT slug, version, npm_package FROM pkg_registry WHERE status IN ('installed','bundled') ORDER BY slug")
	if err != nil {
		return fmt.Errorf("backup: read the package registry: %w", err)
	}
	for rows.Next() {
		var slug, version, spec string
		if err := rows.Scan(&slug, &version, &spec); err != nil {
			rows.Close()
			return err
		}
		if slug == "core" {
			m.Core = version
			m.Lockfile["tinycld"] = spec
			continue
		}
		m.Lockfile[slug] = spec
		m.Packages[slug] = version
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Views are left out: they hold no rows of their own, and a count of one
	// checks a query rather than the data the backup carries. CheckCounts walks
	// this same list, so a restore never counts a view either.
	names, err := db.Query("SELECT name FROM _collections WHERE system = 0 AND type != 'view' ORDER BY name")
	if err != nil {
		return fmt.Errorf("backup: list collections: %w", err)
	}
	var cols []string
	for names.Next() {
		var n string
		if err := names.Scan(&n); err != nil {
			names.Close()
			return err
		}
		cols = append(cols, n)
	}
	if err := names.Close(); err != nil {
		return err
	}
	if err := names.Err(); err != nil {
		return err
	}
	for _, name := range cols {
		if strings.Contains(name, "`") {
			continue
		}
		var n int
		// A collection name is an identifier, not a parameter, so it is quoted
		// rather than bound.
		if err := db.QueryRow("SELECT count(*) FROM `" + name + "`").Scan(&n); err != nil {
			continue // a collection whose table will not count is not a reason to fail a backup
		}
		m.Counts.Collections[name] = n
	}
	return nil
}

// usesS3 reports whether the app's settings enable S3 storage. No settings row
// means a database that never saved any, which is local storage. Settings that
// exist but do not decode here (PB_ENCRYPTION) say nothing either way, so that
// is ErrStorageUnknown rather than a guess.
func usesS3(db *sql.DB) (bool, error) {
	var raw string
	err := db.QueryRow("SELECT value FROM _params WHERE id = 'settings'").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrStorageUnknown, err)
	}
	var s struct {
		S3 struct {
			Enabled bool `json:"enabled"`
		} `json:"s3"`
	}
	if json.Unmarshal([]byte(raw), &s) != nil {
		return false, ErrStorageUnknown
	}
	return s.S3.Enabled, nil
}
