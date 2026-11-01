package reports

import (
	"net/http"
	"slices"
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/oauth"
)

func catalogSource(t *testing.T, cat []CatalogSource, key string) CatalogSource {
	t.Helper()
	for _, s := range cat {
		if s.Key == key {
			return s
		}
	}
	t.Fatalf("catalog has no %s", key)
	return CatalogSource{}
}

func catalogField(t *testing.T, s CatalogSource, key string) CatalogField {
	t.Helper()
	for _, f := range s.Fields {
		if f.Key == key {
			return f
		}
	}
	t.Fatalf("%s has no field %s", s.Key, key)
	return CatalogField{}
}

func catalogLink(t *testing.T, s CatalogSource, key string) CatalogLink {
	t.Helper()
	for _, l := range s.Links {
		if l.Key == key {
			return l
		}
	}
	t.Fatalf("%s has no link %s", s.Key, key)
	return CatalogLink{}
}

func TestCatalogListsUsableSourcesWithTheirFieldsAndLinks(t *testing.T) {
	z := newZoo(t)
	cat := Catalog(z.app)
	keys := make([]string, len(cat))
	for i, s := range cat {
		keys[i] = s.Key
	}
	if !slices.Equal(keys, []string{"zoo.animals", "zoo.feedings", "zoo.keepers", "zoo.people", "zoo.sites", "zoo.vault"}) {
		t.Fatalf("catalog keys = %v", keys)
	}
	animals := catalogSource(t, cat, "zoo.animals")
	if animals.Label != "Animals" || animals.Fields[0].Key != "id" {
		t.Fatalf("animals = %+v", animals)
	}
	if f := catalogField(t, animals, "weight"); f.Type != Number || !slices.Equal(f.Aggregates, SumAll) {
		t.Fatalf("weight = %+v", f)
	}
	// A field registered with no aggregates offers what the engine allows it: counts.
	if f := catalogField(t, animals, "active"); !slices.Equal(f.Aggregates, CountOnly) {
		t.Fatalf("active = %+v", f)
	}
	if l := catalogLink(t, animals, "feedings"); l.Target != "zoo.feedings" || l.Cardinality != ToMany || l.Label != "Feedings" {
		t.Fatalf("feedings link = %+v", l)
	}
	if f := catalogField(t, catalogSource(t, cat, "zoo.feedings"), "cost"); f.Type != Money || f.Currency != "currency" {
		t.Fatalf("cost = %+v", f)
	}
	if f := catalogField(t, catalogSource(t, cat, "zoo.sites"), "region"); len(f.Options) != 2 || f.Options[0].Label != "North" {
		t.Fatalf("region = %+v", f)
	}
}

// The catalog hands out copies: a caller that edits one cannot change the registry.
func TestCatalogCopiesTheRegistry(t *testing.T) {
	z := newZoo(t)
	first := catalogField(t, catalogSource(t, Catalog(z.app), "zoo.animals"), "weight")
	first.Aggregates[0] = Count
	if f := catalogField(t, catalogSource(t, Catalog(z.app), "zoo.animals"), "weight"); f.Aggregates[0] != Sum {
		t.Fatalf("the registry changed: %+v", f)
	}
}

func TestCatalogLeavesOutARefusedSource(t *testing.T) {
	z := newZoo(t)
	RegisterSource(Source{Key: "zoo.ghosts", Collection: "zoo_ghosts", Label: "Ghosts", Fields: []Field{{Key: "name", Label: "Name", Type: Text}}})
	if err := CheckSources(z.app); err == nil {
		t.Fatal("CheckSources accepted zoo.ghosts, which has no collection")
	}
	for _, s := range Catalog(z.app) {
		if s.Key == "zoo.ghosts" {
			t.Fatal("the catalog offers a refused source")
		}
	}
}

func (z *zoo) get(t *testing.T, name, url, token string, status int, want []string, notWant []string) {
	t.Helper()
	headers := map[string]string{}
	if token != "" {
		headers["Authorization"] = token
	}
	sc := tests.ApiScenario{
		Name: name, Method: http.MethodGet, URL: url, Headers: headers,
		ExpectedStatus: status, ExpectedContent: want, NotExpectedContent: notWant,
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}

func TestSourcesEndpoint(t *testing.T) {
	z := newZoo(t)
	Register(z.app)
	z.get(t, "a member reads the catalog", "/api/reports/sources", z.token(t, "ann"), http.StatusOK,
		[]string{`"sources":[`, `"key":"zoo.animals"`, `"cardinality":"toMany"`, `"aggregates":["sum","avg","min","max","count","countDistinct"]`},
		// The catalog names sources, never their collections or columns.
		[]string{`zoo_feedings`, `"column"`, `"collection"`})
	z.get(t, "signed out", "/api/reports/sources", "", http.StatusUnauthorized, []string{`"status":401`}, nil)
}

// The catalog describes every package's sources; an OAuth token's scopes cover only
// some, so the route stays session-only, as the run route does.
func TestSourcesEndpointIsNotOpenToOAuthTokens(t *testing.T) {
	if got := oauth.ScopeForRoute(http.MethodGet, "/api/reports/sources"); len(got) != 0 {
		t.Fatalf("GET /api/reports/sources resolved to scopes %v, want default-deny", got)
	}
}
