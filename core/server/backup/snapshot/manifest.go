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

	names, err := db.Query("SELECT name FROM _collections WHERE system = 0 ORDER BY name")
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
			continue // a view that fails to count is not a reason to fail a backup
		}
		m.Counts.Collections[name] = n
	}
	return nil
}

// usesS3 reports whether the app's settings enable S3 storage. Settings that
// are encrypted (PB_ENCRYPTION) cannot be read here and count as local.
func usesS3(db *sql.DB) bool {
	var raw string
	if err := db.QueryRow("SELECT value FROM _params WHERE id = 'settings'").Scan(&raw); err != nil {
		return false
	}
	var s struct {
		S3 struct {
			Enabled bool `json:"enabled"`
		} `json:"s3"`
	}
	if json.Unmarshal([]byte(raw), &s) != nil {
		return false
	}
	return s.S3.Enabled
}
