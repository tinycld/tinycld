package carddav

import (
	"net/http"
	"strings"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
)

// authenticateRequest validates HTTP Basic credentials against the users auth
// collection. The identifier may be a bare username or a full email (the
// discriminator is '@'), mirroring PocketBase's identityFields for `users`. This
// is feature-agnostic — CalDAV/WebDAV will share the same Basic-Auth path.
func authenticateRequest(app *pocketbase.PocketBase, r *http.Request) (*core.Record, error) {
	identifier, password, ok := r.BasicAuth()
	if !ok || identifier == "" {
		return nil, errUnauthorized
	}

	var record *core.Record
	var err error
	if strings.Contains(identifier, "@") {
		record, err = app.FindAuthRecordByEmail("users", identifier)
	} else {
		record, err = app.FindFirstRecordByFilter(
			"users",
			"username = {:u}",
			map[string]any{"u": identifier},
		)
	}
	if err != nil || record == nil {
		return nil, errUnauthorized
	}

	if !record.ValidatePassword(password) {
		return nil, errUnauthorized
	}

	return record, nil
}

type authError struct{}

func (e *authError) Error() string { return "unauthorized" }

var errUnauthorized = &authError{}
