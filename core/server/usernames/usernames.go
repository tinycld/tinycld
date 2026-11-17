// Package usernames holds the rules for the users username, the name a person
// signs in with. It has no dependencies, so a process that does not run the
// core server can use the same rules, for example to tell a new owner the
// username their account will get.
package usernames

import (
	"regexp"
	"strings"
)

// nonUsernameChar matches anything outside [a-z0-9_-]. We strip rather than
// escape so emails like "alice.b+tag@x.com" become "alicebtag" cleanly.
var nonUsernameChar = regexp.MustCompile(`[^a-z0-9_-]`)

// Derive turns an email (or arbitrary string) into a candidate username. Only
// a wholly-empty cleaned prefix falls back to "user" — single-character emails
// like "a@x.com" are valid usernames now, so we preserve them as-is. Collision
// resolution then disambiguates ("user", "user2", "user3").
func Derive(email string) string {
	prefix := email
	if at := strings.IndexByte(email, '@'); at >= 0 {
		prefix = email[:at]
	}
	prefix = strings.ToLower(prefix)
	prefix = nonUsernameChar.ReplaceAllString(prefix, "")
	if len(prefix) == 0 {
		return "user"
	}
	return prefix
}

var usernameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// Valid enforces the rules the front-end also validates: lowercase
// alphanumeric with dash or underscore, 1..32 chars, must start with
// alphanumeric.
func Valid(s string) bool { return usernameRE.MatchString(s) }

// TombstonePrefix starts the username of every deleted (offboarded) user. A
// live account may not take it, so a live login never looks like a deleted one.
const TombstonePrefix = "deleted-"

// Reserved reports whether s is a username no live account may take.
func Reserved(s string) bool { return strings.HasPrefix(s, TombstonePrefix) }
