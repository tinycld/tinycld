package coreserver

import (
    "encoding/json"
    "net/http"
    "testing"

    "github.com/pocketbase/pocketbase/core"
)

// tokenForUser generates a PocketBase auth token string for the given user
// record, suitable for use in Authorization headers.
func tokenForUser(app core.App, user *core.Record) (string, error) {
    return user.NewAuthToken()
}

// readJSONBody decodes a JSON response body into a map. Fatals on decode error.
func readJSONBody(t *testing.T, res *http.Response) map[string]any {
    t.Helper()
    var body map[string]any
    if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
        t.Fatalf("decode response body: %v", err)
    }
    return body
}

// relaxUsernameMinLength mutates the in-memory users collection so its
// username field accepts 1-char values, mirroring the production migration
// pb_migrations/1880000000_users_username_relax_min_length.js. PB's bundled
// test fixture ships the default users collection with `min: 3`; tests that
// derive usernames from short email prefixes (e.g. "a@x.com" → "a") need
// this relaxed minimum to save the resulting records.
func relaxUsernameMinLength(users *core.Collection) {
    f := users.Fields.GetByName("username")
    if f == nil {
        return
    }
    if tf, ok := f.(*core.TextField); ok {
        tf.Min = 1
        tf.Pattern = "^[a-z0-9][a-z0-9_-]{0,31}$"
    }
}
