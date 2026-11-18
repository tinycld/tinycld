package pkgaccess

import (
	"strings"
	"testing"

	"tinycld.org/core/rlstest"
)

// Every package rule reads a user's access row by user and package: an index
// serves that read instead of a scan of the table for each rule evaluation.
func TestOrgPkgAccessHasAUserPkgIndex(t *testing.T) {
	app := rlstest.NewMigratedApp(t, rlstest.MigrationsDir(t, "../pb_migrations"))
	col, err := app.FindCollectionByNameOrId("org_pkg_access")
	if err != nil {
		t.Fatal(err)
	}
	want := "CREATE INDEX `idx_org_pkg_access_user_pkg` ON `org_pkg_access` (`user`, `pkg`)"
	if !strings.Contains(strings.Join(col.Indexes, "\n"), want) {
		t.Fatalf("indexes = %v", col.Indexes)
	}
	rows, err := app.DB().NewQuery("EXPLAIN QUERY PLAN SELECT access FROM org_pkg_access WHERE user = 'u' AND pkg = 'p'").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan, "\n"), "idx_org_pkg_access_user_pkg") {
		t.Fatalf("the read does not use the index: %v", plan)
	}
}
