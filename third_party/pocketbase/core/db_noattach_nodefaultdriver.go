//go:build no_default_driver

package core

import "database/sql"

// ReapplyNoAttachLimits is a no-op without the default driver: the ATTACH
// restriction is implemented against modernc.org/sqlite, and a build that
// supplies its own driver supplies its own policy with it. The stub keeps
// initDataDB/initAuxDB free of build tags.
func ReapplyNoAttachLimits(sqlDB *sql.DB) error { return nil }
