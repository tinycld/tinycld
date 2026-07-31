// Package orgcookie maintains the cross-org switcher cookie.
//
// Each tenant upserts its own {slug, name} entry into a `tinycld_orgs` cookie
// scoped to the parent domain (Domain=.<MT_BASE_DOMAIN>) whenever a user
// authenticates, so the browser accumulates the orgs its user has actually
// signed into. The app shell's switcher UI (core/lib/org-cookie.ts — the two
// must agree on the wire shape) reads it back with document.cookie, which is
// why the cookie is deliberately NOT HttpOnly.
//
// It is a NAVIGATION HINT, not an authorization claim: the value is
// client-writable, and every org a user navigates to still authenticates them
// itself. Nothing reads it server-side.
//
// The entry deliberately carries NO URL. The cookie is writable by JS on any
// sibling tenant, so a stored URL is an attacker-controlled navigation target:
// one hostile org could rewrite another org's entry to point the switcher at
// a login clone. The client derives each org's URL from its slug and the
// parent domain of the origin it is running on — data the cookie cannot
// influence. Slugs are validated as single DNS labels on both sides for the
// same reason: a slug like "evil.example/x" must never survive into a URL.
package orgcookie

import (
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
)

// Name is the cookie name; must match ORGS_COOKIE_NAME in core's
// lib/org-cookie.ts.
const Name = "tinycld_orgs"

// maxEntries bounds the list so the cookie cannot grow past header limits;
// matches the client parser's cap. Oldest entries fall off first.
const maxEntries = 20

// slugPattern accepts exactly one lowercase DNS label — what the front
// router's subdomain dispatch produces. Anything else (dots, slashes,
// uppercase) is a planted entry and is dropped.
var slugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Entry is one org the browser knows about. The org's URL is deliberately not
// stored — see the package comment.
type Entry struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// Merge upserts entry into an existing cookie value (URL-encoded JSON array)
// and returns the new encoded value. Malformed existing values are discarded
// rather than preserved — the entry being written is the only one this tenant
// can vouch for. The upserted entry moves to the front (most recent first).
func Merge(existing string, entry Entry) string {
	entries := decode(existing)

	merged := make([]Entry, 0, len(entries)+1)
	merged = append(merged, entry)
	for _, e := range entries {
		if e.Slug == entry.Slug {
			continue
		}
		merged = append(merged, e)
		if len(merged) >= maxEntries {
			break
		}
	}

	body, err := json.Marshal(merged)
	if err != nil {
		// Entries are plain strings; marshal cannot realistically fail. Fail
		// closed to just this tenant's own entry.
		body, _ = json.Marshal([]Entry{entry})
	}
	return url.QueryEscape(string(body))
}

func decode(value string) []Entry {
	if value == "" {
		return nil
	}
	raw, err := url.QueryUnescape(value)
	if err != nil {
		return nil
	}
	var entries []Entry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil
	}
	valid := entries[:0]
	for _, e := range entries {
		if !slugPattern.MatchString(e.Slug) {
			continue
		}
		valid = append(valid, e)
	}
	return valid
}

// Cookie builds the Set-Cookie for the merged value, scoped to the parent
// domain so every sibling org (and the apex) sees it. Not HttpOnly by design:
// the switcher UI reads it from document.cookie.
func Cookie(value, baseDomain string) *http.Cookie {
	return &http.Cookie{
		Name:     Name,
		Value:    value,
		Domain:   "." + baseDomain,
		Path:     "/",
		MaxAge:   365 * 24 * 60 * 60,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}
}
