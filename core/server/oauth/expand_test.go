package oauth

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

// newExpandApp adds the fixture packages' collections with relations between
// them: notes_items links to widgets_items, labels and users, and
// widgets_items links back to notes_items and to oauth_grants (a collection no
// scope governs).
func newExpandApp(t *testing.T) *tests.TestApp {
	t.Helper()
	app := newSchemaApp(t)

	find := func(name string) *core.Collection {
		col, err := app.FindCollectionByNameOrId(name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		return col
	}
	save := func(col *core.Collection) {
		if err := app.Save(col); err != nil {
			t.Fatalf("save %s: %v", col.Name, err)
		}
	}
	relation := func(name string, target *core.Collection) *core.RelationField {
		return &core.RelationField{Name: name, MaxSelect: 1, CollectionId: target.Id}
	}

	labels := core.NewBaseCollection("labels")
	labels.Fields.Add(&core.TextField{Name: "name"})
	save(labels)

	widgets := core.NewBaseCollection("widgets_items")
	widgets.Fields.Add(&core.TextField{Name: "name"})
	widgets.Fields.Add(relation("owner", find("users")))
	widgets.Fields.Add(relation("grant", find(grantsCollection)))
	save(widgets)

	notes := core.NewBaseCollection("notes_items")
	notes.Fields.Add(&core.TextField{Name: "title"})
	notes.Fields.Add(relation("label", labels))
	notes.Fields.Add(relation("widget", widgets))
	notes.Fields.Add(relation("author", find("users")))
	save(notes)

	widgets.Fields.Add(relation("note", notes))
	save(widgets)

	return app
}

func mintToken(t *testing.T, app *tests.TestApp, scopes ...string) string {
	t.Helper()
	userID, clientID := seedUserAndClient(t, app)
	grant, err := NewGrant(app, userID, clientID, scopes, "active")
	if err != nil {
		t.Fatalf("NewGrant: %v", err)
	}
	user, err := app.FindRecordById("users", userID)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	token, err := MintAccessToken(app, user, grant, AccessTokenTTL)
	if err != nil {
		t.Fatalf("MintAccessToken: %v", err)
	}
	return token
}

// A token reads a collection only when its scopes govern that collection. An
// expand returns the records of a related collection in the same response, so
// each collection on an expand path needs the same check as a direct read.
func TestEnforceGrantChecksExpandPaths(t *testing.T) {
	const list = "/api/collections/notes_items/records"
	const view = "/api/collections/notes_items/records/abc123"

	cases := []struct {
		name   string
		scopes []string
		method string
		url    string
		want   int
	}{
		{"no expand", []string{scopeNotesRead}, "GET", list, 0},
		{"target in scope", []string{scopeNotesRead}, "GET", list + "?expand=label", 0},
		{"list, target out of scope", []string{scopeNotesRead}, "GET", list + "?expand=widget", 403},
		{"view, target out of scope", []string{scopeNotesRead}, "GET", view + "?expand=widget", 403},
		{"one path of several out of scope", []string{scopeNotesRead}, "GET", list + "?expand=label,widget", 403},
		{"second expand parameter", []string{scopeNotesRead}, "GET", list + "?expand=label&expand=widget", 403},
		{"spaces, as PocketBase reads them", []string{scopeNotesRead}, "GET", list + "?expand=%20wid%20get%20", 403},
		{"incomplete path", []string{scopeNotesRead}, "GET", list + "?expand=widget.", 403},
		{"back-relation out of scope", []string{scopeNotesRead}, "GET", list + "?expand=widgets_items_via_note", 403},
		{"old back-relation syntax", []string{scopeNotesRead}, "GET", list + "?expand=widgets_items(note)", 403},
		{"users without the profile scope", []string{scopeNotesRead}, "GET", list + "?expand=author", 403},
		{"users with the profile scope", []string{scopeNotesRead, ScopeProfile}, "GET", list + "?expand=author", 0},
		{"not a relation: PocketBase expands nothing", []string{scopeNotesRead}, "GET", list + "?expand=title", 0},
		{"unknown field: PocketBase expands nothing", []string{scopeNotesRead}, "GET", list + "?expand=nothing.widget", 0},
		{"create response", []string{scopeNotesWrite}, "POST", list + "?expand=widget", 403},
		{"update response", []string{scopeNotesWrite}, "PATCH", view + "?expand=widget", 403},
		{"write scope does not read an expand", []string{scopeNotesWrite}, "PATCH", view + "?expand=label", 403},
		{"chain in scope", []string{scopeNotesRead, scopeWidgetsRead}, "GET", list + "?expand=widget.note.label", 0},
		{"chain leaves scope at depth 2", []string{scopeNotesRead, scopeWidgetsRead}, "GET", list + "?expand=widget.owner", 403},
		{"chain reaches an unclassified collection", []string{scopeNotesRead, scopeWidgetsRead}, "GET", list + "?expand=widget.grant", 403},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app := newExpandApp(t)
			re := newRequestEvent(app, c.method, c.url, mintToken(t, app, c.scopes...))
			err := enforceGrant(re)
			if c.want == 0 {
				if err != nil {
					t.Fatalf("enforceGrant refused %s %s: %v", c.method, c.url, err)
				}
				return
			}
			if got := apiStatus(err); got != c.want {
				t.Fatalf("enforceGrant status = %d, want %d (%v)", got, c.want, err)
			}
			if re.Auth != nil {
				t.Fatal("enforceGrant must not set e.Auth when it refuses the request")
			}
		})
	}
}
