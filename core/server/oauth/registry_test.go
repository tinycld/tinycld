package oauth

import (
	"os"
	"strings"
	"testing"
)

// Fixture packages for this package's tests. Deliberately FICTIONAL: core's
// tests must not know a real package any more than core's code does. "notes"
// and "tasks" between them exercise every shape the registry supports —
// read/write and read-only collections, a shared core collection both claim
// (labels), exact endpoints, and a per-record prefix family.
const (
	scopeNotesRead  = "notes:read"
	scopeNotesWrite = "notes:write"
	scopeTasksRead  = "tasks:read"
	scopeTasksWrite = "tasks:write"
)

func notesPackage() Package {
	return Package{
		Slug: "notes",
		Scopes: []Scope{
			{ID: scopeNotesRead, Label: "Read your notes"},
			{ID: scopeNotesWrite, Label: "Create and modify your notes"},
		},
		Collections: map[string]Access{
			"notes_items":         {Read: []string{scopeNotesRead}, Write: []string{scopeNotesWrite}},
			"notes_folder_counts": {Read: []string{scopeNotesRead}},
			"labels":              {Read: []string{scopeNotesRead}, Write: []string{scopeNotesWrite}},
		},
		Endpoints: map[string][]string{
			"GET /api/notes/search": {scopeNotesRead},
			"POST /api/notes/send":  {scopeNotesWrite},
		},
		EndpointPrefixes: []EndpointPrefix{
			{Method: "POST", Prefix: "/api/notes/items/", Scopes: []string{scopeNotesWrite}},
		},
	}
}

func tasksPackage() Package {
	return Package{
		Slug: "tasks",
		Scopes: []Scope{
			{ID: scopeTasksRead, Label: "Read your tasks"},
			{ID: scopeTasksWrite, Label: "Create and modify your tasks"},
		},
		Collections: map[string]Access{
			"tasks_items": {Read: []string{scopeTasksRead}, Write: []string{scopeTasksWrite}},
			"labels":      {Read: []string{scopeTasksRead}, Write: []string{scopeTasksWrite}},
		},
		Endpoints: map[string][]string{
			"POST /api/tasks/upload-version": {scopeTasksWrite},
			"GET /api/tasks/export":          {scopeTasksRead},
		},
	}
}

func registerFixtures() {
	RegisterPackage(notesPackage())
	RegisterPackage(tasksPackage())
}

// Every test in this package runs against the fixture registry. A test that
// needs a different registry state resets it and restores the fixtures on
// cleanup (see withEmptyRegistry).
func TestMain(m *testing.M) {
	registerFixtures()
	os.Exit(m.Run())
}

func withEmptyRegistry(t *testing.T) {
	t.Helper()
	ResetRegistry()
	t.Cleanup(func() {
		ResetRegistry()
		registerFixtures()
	})
}

// The guard this registry exists for: with nothing registered, core grants
// exactly the identity scope and classifies exactly the identity routes.
// Every other scope, collection and route is a package's to declare. If this
// test starts failing, someone has put package knowledge back into core.
func TestCoreDeclaresNoPackageScopes(t *testing.T) {
	withEmptyRegistry(t)

	if got := AllScopes(); len(got) != 1 || got[0] != ScopeProfile {
		t.Fatalf("an empty registry must expose only %q, got %v", ScopeProfile, got)
	}
	if got := ScopeLabels(); len(got) != 1 || got[ScopeProfile] != ProfileScopeLabel {
		t.Fatalf("an empty registry must label only %q, got %v", ScopeProfile, got)
	}
	if got := ScopeForRoute("GET", "/api/collections/users/records"); !onlyScope(got, ScopeProfile) {
		t.Errorf("users read = %v, want %q", got, ScopeProfile)
	}
	if got := ScopeForRoute("GET", "/oauth/userinfo"); !onlyScope(got, ScopeProfile) {
		t.Errorf("userinfo = %v, want %q", got, ScopeProfile)
	}
	for _, r := range []struct{ method, path string }{
		{"PATCH", "/api/collections/users/records/abc"},
		{"GET", "/api/collections/labels/records"},
		{"GET", "/api/collections/notes_items/records"},
		{"GET", "/api/notes/search"},
		{"GET", "/api/search"},
		{"GET", "/api/files/notes_items/rec123/body_ab12cd34ef.html"},
	} {
		if got := ScopeForRoute(r.method, r.path); len(got) != 0 {
			t.Errorf("%s %s = %v with nothing registered; core must not classify it", r.method, r.path, got)
		}
	}
}

func TestRegisterPackageBuildsCatalog(t *testing.T) {
	want := []string{ScopeProfile, scopeNotesRead, scopeNotesWrite, scopeTasksRead, scopeTasksWrite}
	if got := AllScopes(); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("AllScopes = %v, want %v (profile first, packages by slug, scopes in declaration order)", got, want)
	}
	labels := ScopeLabels()
	if labels[scopeNotesRead] != "Read your notes" || labels[ScopeProfile] != ProfileScopeLabel {
		t.Errorf("labels = %v", labels)
	}
	if got := PackageScopes("notes"); strings.Join(got, " ") != scopeNotesRead+" "+scopeNotesWrite {
		t.Errorf("PackageScopes(notes) = %v", got)
	}
	if got := PackageScopes("nope"); got != nil {
		t.Errorf("PackageScopes(unknown) = %v, want nil", got)
	}
}

// Re-registering a slug replaces its contribution rather than stacking a
// second copy — a dev reload re-runs every package's Register.
func TestRegisterPackageIsIdempotentPerSlug(t *testing.T) {
	withEmptyRegistry(t)
	RegisterPackage(notesPackage())
	narrowed := notesPackage()
	delete(narrowed.Collections, "notes_folder_counts")
	RegisterPackage(narrowed)

	if got := AllScopes(); len(got) != 3 {
		t.Errorf("AllScopes after re-registration = %v, want profile + 2", got)
	}
	if got := ScopeForRoute("GET", "/api/collections/notes_folder_counts/records"); len(got) != 0 {
		t.Errorf("a collection dropped by the re-registration must no longer be classified, got %v", got)
	}
}

func TestRegisterPackageRejectsMalformedRegistrations(t *testing.T) {
	withEmptyRegistry(t)
	cases := []struct {
		name string
		pkg  Package
		want string
	}{
		{"bad slug", Package{Slug: "Notes!", Scopes: []Scope{{ID: "x:read", Label: "x"}}}, "not a valid slug"},
		{"no scopes", Package{Slug: "notes"}, "registers no scopes"},
		{"scope outside own namespace", Package{Slug: "notes", Scopes: []Scope{{ID: "tasks:read", Label: "x"}}}, `must be "notes:"`},
		{"scope with no label", Package{Slug: "notes", Scopes: []Scope{{ID: "notes:read"}}}, "plain-language label"},
		{"label that is a scope string", Package{Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "notes:read"}}}, "plain-language label"},
		{"duplicate scope", Package{Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "a"}, {ID: "notes:read", Label: "b"}}}, "declared twice"},
		{"collection naming an undeclared scope", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "a"}},
			Collections: map[string]Access{"notes_items": {Read: []string{"tasks:read"}}},
		}, "does not declare"},
		{"collection with no read rule", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:write", Label: "a"}},
			Collections: map[string]Access{"notes_items": {Write: []string{"notes:write"}}},
		}, "no read rule"},
		{"endpoint outside /api/<slug>/", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "a"}},
			Endpoints: map[string][]string{"GET /api/admin/packages": {"notes:read"}},
		}, `must be "<METHOD> /api/notes/`},
		{"endpoint that is the bare namespace", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "a"}},
			Endpoints: map[string][]string{"GET /api/notes/": {"notes:read"}},
		}, `must be "<METHOD> /api/notes/`},
		{"endpoint with an unknown verb", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "a"}},
			Endpoints: map[string][]string{"FETCH /api/notes/search": {"notes:read"}},
		}, `must be "<METHOD> /api/notes/`},
		{"endpoint with no scopes", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:read", Label: "a"}},
			Endpoints: map[string][]string{"GET /api/notes/search": {}},
		}, "has no scopes"},
		{"prefix not ending in a slash", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:write", Label: "a"}},
			EndpointPrefixes: []EndpointPrefix{{Method: "POST", Prefix: "/api/notes/items", Scopes: []string{"notes:write"}}},
		}, `end in "/"`},
		{"prefix that is the bare namespace", Package{
			Slug: "notes", Scopes: []Scope{{ID: "notes:write", Label: "a"}},
			EndpointPrefixes: []EndpointPrefix{{Method: "POST", Prefix: "/api/notes/", Scopes: []string{"notes:write"}}},
		}, `end in "/"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatal("RegisterPackage accepted a malformed registration")
				}
				if msg, _ := r.(string); !strings.Contains(msg, c.want) {
					t.Errorf("panic = %q, want it to mention %q", msg, c.want)
				}
			}()
			RegisterPackage(c.pkg)
		})
	}
	if got := AllScopes(); len(got) != 1 {
		t.Errorf("a rejected registration must leave the registry untouched, got %v", got)
	}
}

// Two packages cannot both own a scope: the first registration wins and the
// second is a wiring error, not a silent merge.
func TestRegisterPackageRejectsScopeClaimedByAnotherPackage(t *testing.T) {
	withEmptyRegistry(t)
	RegisterPackage(notesPackage())
	defer func() {
		if recover() == nil {
			t.Fatal("a second package claiming notes:read must panic")
		}
	}()
	RegisterPackage(Package{
		Slug:   "notes-extra",
		Scopes: []Scope{{ID: "notes-extra:read", Label: "a"}, {ID: "notes:read", Label: "b"}},
	})
}

func TestRegisterSharedEndpointIsEvaluatedPerRequest(t *testing.T) {
	withEmptyRegistry(t)
	var scopes []string
	RegisterSharedEndpoint("GET", "/api/federated", func() []string { return scopes })

	if got := ScopeForRoute("GET", "/api/federated"); len(got) != 0 {
		t.Errorf("a shared endpoint deriving no scopes must default-deny, got %v", got)
	}
	scopes = []string{scopeNotesRead}
	if got := ScopeForRoute("GET", "/api/federated"); !onlyScope(got, scopeNotesRead) {
		t.Errorf("shared endpoint = %v, want the derived rule", got)
	}
	// A package cannot register outside its own namespace, so the shared
	// route is core's alone; and a shared route is method-specific.
	if got := ScopeForRoute("POST", "/api/federated"); len(got) != 0 {
		t.Errorf("POST on a GET-only shared endpoint = %v, want default-deny", got)
	}
}

func TestRegisterSharedEndpointRejectsMalformedRoutes(t *testing.T) {
	for _, c := range []struct{ method, path string }{
		{"FETCH", "/api/x"},
		{"GET", "/oauth/x"},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterSharedEndpoint(%s %s) must panic", c.method, c.path)
				}
			}()
			RegisterSharedEndpoint(c.method, c.path, func() []string { return nil })
		}()
	}
}
