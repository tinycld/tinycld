package oauth

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
)

// The scope registry.
//
// Core knows nothing about any package. Which scopes exist, which collections
// and routes each one governs, and what the consent screen says about it are
// all declared by the package that owns them, from its own Register(app) —
// the same place it binds every other hook (search.RegisterSources,
// quota.RegisterSources, offboard.RegisterReassignable). Core reads the
// accumulated set when a request arrives.
//
// This replaces the hand-maintained tables that used to live in
// middleware.go. Those needed a core change for every package route, and a
// package whose entry was forgotten shipped default-denied for OAuth callers
// — a failure only a live integration ever noticed. A package that is not
// installed now contributes nothing without anyone maintaining a list, and
// the catalog is correct precisely because it spans every installed package.

// Scope is one grantable capability a package defines.
type Scope struct {
	// ID is "<slug>:<capability>", e.g. "boards:read". The slug prefix is
	// enforced: a package may only define scopes in its own namespace.
	ID string
	// Label is the plain-language consent copy — "Read your boards and
	// cards". A scope string tells a user nothing, so a label is required.
	Label string
}

// Access pairs the read and write rules for one collection. Each rule is
// any-of; an empty Write makes the collection read-only for OAuth callers.
type Access struct {
	Read  []string
	Write []string
}

// EndpointPrefix classifies a route family whose path carries a record id,
// which an exact-match endpoint cannot express: {"POST",
// "/api/boards/cards/", ...} covers /api/boards/cards/{id}/move. A prefix is
// broader than it looks, so it must end at a path segment boundary and name
// a route family, never a bare namespace — the bare prefix itself is a
// different route and stays default-denied.
type EndpointPrefix struct {
	Method string
	Prefix string
	Scopes []string
}

// Package is one package's complete OAuth contribution.
//
// Every scope named in Collections, Endpoints and EndpointPrefixes must be one
// this package declares in Scopes. A collection may be registered by more than
// one package — core's `labels` is used by both mail and contacts — in which
// case the rules union: either package's grant reaches it. Endpoints and
// prefixes must live under /api/<slug>/, so a package can never reclassify a
// core route or another package's.
type Package struct {
	Slug             string
	Scopes           []Scope
	Collections      map[string]Access
	Endpoints        map[string][]string
	EndpointPrefixes []EndpointPrefix
}

type prefixRule struct {
	method, prefix string
	scopes         ScopeRule
}

// collectionAccess is the merged form of Access after every package has
// contributed.
type collectionAccess struct {
	read  ScopeRule
	write ScopeRule
}

// tables is the merged, request-time view: rebuilt under the write lock on
// every registration, read under the read lock on every request.
type tables struct {
	collections map[string]collectionAccess
	endpoints   map[string]ScopeRule
	prefixes    []prefixRule
	// scopes is the catalog: profile first, then each package's scopes in
	// declaration order, packages sorted by slug so the discovery document
	// is stable across restarts.
	scopes []Scope
}

var (
	registryMu sync.RWMutex
	packages   = map[string]Package{}
	// sharedEndpoints are core-owned routes that federate over packages —
	// GET /api/search — whose rule depends on what is installed. Evaluated
	// per request so registration order cannot matter.
	sharedEndpoints = map[string]func() []string{}
	merged          = rebuild()
)

// ProfileScopeLabel is the consent copy for the baseline identity scope,
// the one scope core itself defines.
const ProfileScopeLabel = "See your name and email address"

var (
	slugPattern       = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	capabilityPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	methods           = map[string]bool{
		"GET": true, "HEAD": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
	}
)

// RegisterPackage records a package's scopes and the collections and routes
// they govern. Called from the package's own Register(app) at boot.
//
// Idempotent per slug, so a dev reload that re-runs Register replaces rather
// than duplicates. A malformed registration panics: it is a wiring mistake in
// the package's own code, it cannot be corrected at runtime, and the
// alternative — quietly dropping it — is exactly the silent default-deny this
// registry exists to end.
func RegisterPackage(p Package) {
	if err := validatePackage(p); err != nil {
		panic("oauth: " + err.Error())
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	for slug, other := range packages {
		if slug == p.Slug {
			continue
		}
		for _, s := range other.Scopes {
			for _, mine := range p.Scopes {
				if s.ID == mine.ID {
					panic(fmt.Sprintf("oauth: scope %q is already registered by package %q", mine.ID, slug))
				}
			}
		}
	}
	packages[p.Slug] = p
	merged = rebuild()
}

// RegisterSharedEndpoint classifies a core-owned route whose rule is derived
// from what packages are installed: the federated search admits any scope
// that permits searching some registered source. The rule is a function so
// it can be evaluated after every package has registered, whatever the order.
//
// Only core calls this. A package's own routes go through RegisterPackage,
// which confines them to /api/<slug>/.
func RegisterSharedEndpoint(method, path string, scopes func() []string) {
	if !methods[method] || !strings.HasPrefix(path, "/api/") || scopes == nil {
		panic(fmt.Sprintf("oauth: invalid shared endpoint %s %s", method, path))
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	sharedEndpoints[method+" "+path] = scopes
}

// ResetRegistry clears every registration — for tests only.
func ResetRegistry() {
	registryMu.Lock()
	defer registryMu.Unlock()
	packages = map[string]Package{}
	sharedEndpoints = map[string]func() []string{}
	merged = rebuild()
}

// AllScopes is the full catalog: the baseline profile scope plus every scope
// an installed package registered. It validates a requested scope string,
// renders the consent screen, and is advertised as scopes_supported.
func AllScopes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, len(merged.scopes))
	for i, s := range merged.scopes {
		out[i] = s.ID
	}
	return out
}

// ScopeLabels maps every scope in the catalog to its consent copy.
func ScopeLabels() map[string]string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make(map[string]string, len(merged.scopes))
	for _, s := range merged.scopes {
		out[s.ID] = s.Label
	}
	return out
}

// PackageScopes returns the scope IDs a package registered, in declaration
// order — nil when no package of that slug has registered.
func PackageScopes(slug string) []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := packages[slug]
	if !ok {
		return nil
	}
	out := make([]string, len(p.Scopes))
	for i, s := range p.Scopes {
		out[i] = s.ID
	}
	return out
}

func validatePackage(p Package) error {
	if !slugPattern.MatchString(p.Slug) {
		return fmt.Errorf("package slug %q is not a valid slug", p.Slug)
	}
	if len(p.Scopes) == 0 {
		return fmt.Errorf("package %q registers no scopes", p.Slug)
	}
	declared := map[string]bool{}
	for _, s := range p.Scopes {
		capability, ok := strings.CutPrefix(s.ID, p.Slug+":")
		if !ok || !capabilityPattern.MatchString(capability) {
			return fmt.Errorf("package %q: scope %q must be %q followed by a capability", p.Slug, s.ID, p.Slug+":")
		}
		if declared[s.ID] {
			return fmt.Errorf("package %q: scope %q declared twice", p.Slug, s.ID)
		}
		if strings.TrimSpace(s.Label) == "" || strings.Contains(s.Label, ":") {
			return fmt.Errorf("package %q: scope %q needs a plain-language label (got %q)", p.Slug, s.ID, s.Label)
		}
		declared[s.ID] = true
	}
	own := func(where string, scopes []string) error {
		for _, s := range scopes {
			if !declared[s] {
				return fmt.Errorf("package %q: %s names scope %q, which it does not declare", p.Slug, where, s)
			}
		}
		return nil
	}
	for name, access := range p.Collections {
		if name == "" {
			return fmt.Errorf("package %q: empty collection name", p.Slug)
		}
		if len(access.Read) == 0 {
			return fmt.Errorf("package %q: collection %q has no read rule", p.Slug, name)
		}
		if err := own("collection "+name, append(append([]string{}, access.Read...), access.Write...)); err != nil {
			return err
		}
	}
	routePrefix := "/api/" + p.Slug + "/"
	for key, scopes := range p.Endpoints {
		method, path, ok := strings.Cut(key, " ")
		if !ok || !methods[method] || !strings.HasPrefix(path, routePrefix) || len(path) == len(routePrefix) {
			return fmt.Errorf("package %q: endpoint %q must be \"<METHOD> %s...\"", p.Slug, key, routePrefix)
		}
		if len(scopes) == 0 {
			return fmt.Errorf("package %q: endpoint %q has no scopes", p.Slug, key)
		}
		if err := own("endpoint "+key, scopes); err != nil {
			return err
		}
	}
	for _, r := range p.EndpointPrefixes {
		if !methods[r.Method] || !strings.HasPrefix(r.Prefix, routePrefix) || !strings.HasSuffix(r.Prefix, "/") || len(r.Prefix) == len(routePrefix) {
			return fmt.Errorf("package %q: endpoint prefix %q %q must start with %s and end in \"/\"", p.Slug, r.Method, r.Prefix, routePrefix)
		}
		if len(r.Scopes) == 0 {
			return fmt.Errorf("package %q: endpoint prefix %q has no scopes", p.Slug, r.Prefix)
		}
		if err := own("endpoint prefix "+r.Prefix, r.Scopes); err != nil {
			return err
		}
	}
	return nil
}

// rebuild merges every registration with the entries core itself owns. Called
// with the write lock held (or before any goroutine can observe the tables).
func rebuild() tables {
	t := tables{
		collections: map[string]collectionAccess{
			// The caller's own identity. Read-only: a token edits nobody's
			// account.
			"users": {read: ScopeRule{ScopeProfile}},
		},
		endpoints: map[string]ScopeRule{
			// Advertised in the discovery document as userinfo_endpoint, so
			// an integration following the well-known metadata calls it
			// with an ordinary access token. It needs an explicit entry: it
			// lives under /oauth/ but is deliberately NOT in exemptPaths
			// (only the credential-less endpoints are), so without this it
			// would fall into default-deny and 403 the very call the server
			// tells clients to make.
			"GET /oauth/userinfo": {ScopeProfile},
		},
		scopes: []Scope{{ID: ScopeProfile, Label: ProfileScopeLabel}},
	}
	slugs := make([]string, 0, len(packages))
	for slug := range packages {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		p := packages[slug]
		t.scopes = append(t.scopes, p.Scopes...)
		for name, access := range p.Collections {
			existing := t.collections[name]
			existing.read = union(existing.read, access.Read)
			existing.write = union(existing.write, access.Write)
			t.collections[name] = existing
		}
		for key, scopes := range p.Endpoints {
			t.endpoints[key] = union(t.endpoints[key], scopes)
		}
		for _, r := range p.EndpointPrefixes {
			t.prefixes = append(t.prefixes, prefixRule{method: r.Method, prefix: r.Prefix, scopes: ScopeRule(r.Scopes)})
		}
	}
	return t
}

func union(a ScopeRule, b []string) ScopeRule {
	for _, s := range b {
		if !HasScope(a, s) {
			a = append(a, s)
		}
	}
	return a
}

// lookupEndpoint resolves an exact route, a shared (derived) route, or a
// prefix family, in that order.
func lookupEndpoint(method, path string) (ScopeRule, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	key := method + " " + path
	if s, ok := merged.endpoints[key]; ok {
		return s, true
	}
	if derive, ok := sharedEndpoints[key]; ok {
		return ScopeRule(derive()), true
	}
	for _, r := range merged.prefixes {
		// The remainder must be non-empty: the prefix ends in "/" and names
		// a route family, so the bare prefix itself is a different route.
		if method == r.method && strings.HasPrefix(path, r.prefix) && len(path) > len(r.prefix) {
			return r.scopes, true
		}
	}
	return nil, false
}

func lookupCollection(name string) (collectionAccess, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	access, ok := merged.collections[name]
	return access, ok
}
