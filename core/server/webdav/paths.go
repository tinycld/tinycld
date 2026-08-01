package webdav

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

// maxFolderDepth bounds every parent-chain walk. It caps how deep a legitimate
// tree can nest and doubles as the termination guard against a pre-existing
// cycle: a walk that hasn't reached the root after this many hops is treated as
// corrupt and stops rather than spinning forever.
const maxFolderDepth = 256

// safeIdent matches the SQL identifiers we are willing to interpolate. Source
// field/collection names reach the recursive CTE by string concatenation —
// dbx.Params cannot bind an identifier — so they are validated once at
// construction rather than trusted. See validateSource.
var safeIdent = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// parsePath strips the mount prefix and returns the remaining path segments.
// Empty segments indicate the WebDAV root, which the handler serves as a single
// synthetic directory.
//
// Inputs are run through path.Clean first, which collapses //, . and ..
// segments. If a traversal sequence escapes the prefix entirely (e.g.
// "/drive/../../etc/passwd" cleans to "/etc/passwd") this returns no segments —
// the handler treats it as the root rather than resolving anything outside the
// request's intent.
//
// For example, with prefix "/drive": "/drive/Documents/report.pdf" →
// ["Documents", "report.pdf"].
func parsePath(prefix, name string) []string {
	name = path.Clean(name)
	if name != prefix && !strings.HasPrefix(name, prefix+"/") {
		return nil
	}
	name = strings.TrimPrefix(name, prefix)
	name = strings.Trim(name, "/")

	if name == "" || name == "." {
		return nil
	}

	return strings.Split(name, "/")
}

// resolveIDByPath resolves the record id at the end of a path in a single SQL
// roundtrip via a recursive CTE driven by a JSON array of segment names.
// Returns os.ErrNotExist if any segment doesn't resolve. Empty segments return
// ("", false, nil) to signal the root.
//
// The N+1 this replaces: a per-segment filter loop costs one query per segment,
// and Finder hammers Stat during PROPFIND walks — depth-D paths against a
// directory of N children cost O(N·D) round trips. The CTE is one query
// regardless of depth; the caller pays one more FindRecordById to hydrate the
// record, or skips it when only the id is needed.
func (fs *FileSystem) resolveIDByPath(segments []string) (id string, isFolder bool, err error) {
	if len(segments) == 0 {
		return "", false, nil
	}

	segsJSON, err := json.Marshal(segments)
	if err != nil {
		return "", false, err
	}

	var row struct {
		ID       string `db:"id"`
		Idx      int    `db:"idx"`
		IsFolder bool   `db:"is_folder"`
	}

	f := fs.src.Fields
	q := fmt.Sprintf(`
WITH RECURSIVE
  segs(idx, name) AS (
    SELECT CAST(key AS INTEGER), value FROM json_each({:segs})
  ),
  walk(idx, id, is_folder) AS (
    SELECT segs.idx, di.id, di.%[2]s
      FROM segs
      JOIN %[1]s di
        ON di.%[3]s = '' AND di.%[4]s = segs.name AND segs.idx = 0
    UNION ALL
    SELECT segs.idx, di.id, di.%[2]s
      FROM segs, walk
      JOIN %[1]s di
        ON di.%[3]s = walk.id AND di.%[4]s = segs.name AND segs.idx = walk.idx + 1
  )
SELECT idx, id, is_folder FROM walk ORDER BY idx DESC LIMIT 1`,
		fs.src.Collection, f.IsFolder, f.Parent, f.Name)

	err = fs.app.DB().NewQuery(q).Bind(dbx.Params{
		"segs": string(segsJSON),
	}).One(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, os.ErrNotExist
	}
	if err != nil {
		return "", false, err
	}

	// idx counts from 0; matched-all-segments means idx == len(segments)-1.
	if row.Idx != len(segments)-1 {
		return "", false, os.ErrNotExist
	}
	return row.ID, row.IsFolder, nil
}

// resolveByPath walks the path segments to find the backing record. Returns nil
// if the path resolves to the root (no segments). Two queries total: the
// recursive CTE, then FindRecordById to hydrate.
func (fs *FileSystem) resolveByPath(segments []string) (*core.Record, error) {
	if len(segments) == 0 {
		return nil, nil
	}
	id, _, err := fs.resolveIDByPath(segments)
	if err != nil {
		return nil, err
	}
	record, err := fs.app.FindRecordById(fs.src.Collection, id)
	if err != nil {
		return nil, os.ErrNotExist
	}
	return record, nil
}

// resolveParentByPath resolves the parent folder for a given path, checking
// that user may READ it. Returns the parent's record id (empty for the root)
// and the final segment name. The parent must exist, be a folder, and be
// visible to the caller.
//
// The read check is not optional. Without it a write verb resolved a parent by
// id alone, which let a caller plant a record inside a folder they cannot see —
// squatting the globally-unique (parent, name) namespace inside another user's
// tree — and made "that folder exists but isn't yours" distinguishable from
// "that folder doesn't exist", reopening the existence oracle the leaf-name
// masking closes. No package's createRule carries a parent clause, so the rule
// engine does not cover this on its own.
//
// A parent the caller cannot read is reported as os.ErrNotExist, identical to a
// parent that isn't there.
func (fs *FileSystem) resolveParentByPath(user *core.Record, segments []string) (parentID, name string, err error) {
	if len(segments) == 0 {
		return "", "", os.ErrNotExist
	}

	name = segments[len(segments)-1]
	parentSegments := segments[:len(segments)-1]

	if len(parentSegments) == 0 {
		// The mount root is synthetic — there is no record to authorize, and the
		// collection's create rule remains the gate.
		return "", name, nil
	}

	id, isFolder, err := fs.resolveIDByPath(parentSegments)
	if err != nil {
		return "", "", os.ErrNotExist
	}
	if !isFolder {
		return "", "", os.ErrNotExist
	}

	parent, err := fs.app.FindRecordById(fs.src.Collection, id)
	if err != nil {
		return "", "", os.ErrNotExist
	}
	if err := fs.canRead(user, parent); err != nil {
		return "", "", os.ErrNotExist
	}
	return id, name, nil
}

// wouldCreateCycle reports whether reparenting movedID under newParentID would
// introduce a cycle — i.e. movedID is newParentID itself, or an ancestor of it.
// It walks the new parent's ancestor chain upward, guarded by a visited-set AND
// a depth cap so a PRE-EXISTING cycle in the data can't hang the check.
//
// A move to the root (empty newParentID) can never create a cycle.
func (fs *FileSystem) wouldCreateCycle(movedID, newParentID string) (bool, error) {
	if newParentID == "" || movedID == "" {
		return false, nil
	}
	if newParentID == movedID {
		return true, nil
	}

	seen := map[string]bool{}
	id := newParentID
	for depth := 0; id != "" && depth < maxFolderDepth; depth++ {
		if id == movedID {
			return true, nil
		}
		if seen[id] {
			// Already-cyclic ancestor chain not involving movedID: the move
			// isn't what creates the cycle, so bail out cleanly rather than
			// blocking it on that basis.
			return false, nil
		}
		seen[id] = true

		rec, err := fs.app.FindRecordById(fs.src.Collection, id)
		if err != nil {
			// A dangling parent pointer terminates the chain; no cycle.
			return false, nil
		}
		id = rec.GetString(fs.src.Fields.Parent)
	}
	return false, nil
}
