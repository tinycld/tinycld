package reports

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/rlstest"
)

// The zoo: a fictional package's collections on core's real schema (the
// users collection and its rules come from core's migrations).
//
//	zoo_sites     list: a signed-in user whose role is member
//	zoo_animals   list: the owner, or a keeper of the animal (back-relation)
//	zoo_keepers   list: the keeper themself
//	zoo_feedings  list: keepers of the fed animal (@collection rule)
//	zoo_vault     list: null (superusers only)
//
// Who sees what:
//
//	ann owns leo and kit and keeps leo and zed: animals leo, zed, kit;
//	    feedings f1, f2 (leo), f3, f4 (zed)
//	bob owns zed and mo and keeps zed and mo: animals zed, mo;
//	    feedings f3, f4 (zed), f5 (mo)
type zoo struct {
	app  *tests.TestApp
	recs map[string]*core.Record
}

func rule(s string) *string { return &s }

// zooFiscalStart is what the zoo.feedings source reports as its fiscal start
// month. A test that changes it restores it with t.Cleanup.
var zooFiscalStart = 1

func newZoo(t testing.TB) *zoo {
	t.Helper()
	ResetForTesting()
	t.Cleanup(ResetForTesting)
	app := rlstest.NewAssembledApp(t)
	z := &zoo{app: app, recs: map[string]*core.Record{}}
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	sites := core.NewBaseCollection("zoo_sites")
	// A non-plain auth field makes the resolver join the auth record's table.
	sites.ListRule = rule(`@request.auth.id != "" && @request.auth.role = "member"`)
	sites.ViewRule = sites.ListRule
	sites.Fields.Add(
		&core.TextField{Name: "name"},
		&core.SelectField{Name: "region", Values: []string{"north", "south"}, MaxSelect: 1},
	)
	z.save(t, sites)

	animals := core.NewBaseCollection("zoo_animals")
	animals.Fields.Add(
		&core.TextField{Name: "name"},
		&core.TextField{Name: "species"},
		&core.RelationField{Name: "site", CollectionId: sites.Id, MaxSelect: 1},
		&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1},
		&core.NumberField{Name: "weight"},
		&core.NumberField{Name: "price"},
		&core.DateField{Name: "born"},
		&core.BoolField{Name: "active"},
		&core.TextField{Name: "secret", Hidden: true},
	)
	z.save(t, animals)

	keepers := core.NewBaseCollection("zoo_keepers")
	keepers.ListRule = rule(`user = @request.auth.id`)
	keepers.ViewRule = keepers.ListRule
	keepers.Fields.Add(
		&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1},
		&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
	)
	z.save(t, keepers)

	// The back-relation needs zoo_keepers to exist first.
	animals.ListRule = rule(`owner = @request.auth.id || zoo_keepers_via_animal.user ?= @request.auth.id`)
	animals.ViewRule = animals.ListRule
	z.save(t, animals)

	feedings := core.NewBaseCollection("zoo_feedings")
	feedings.ListRule = rule(`@collection.zoo_keepers.user ?= @request.auth.id && @collection.zoo_keepers.animal ?= animal`)
	feedings.ViewRule = feedings.ListRule
	feedings.Fields.Add(
		&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
		&core.NumberField{Name: "cost"},
		&core.TextField{Name: "currency"},
		&core.NumberField{Name: "grams"},
		&core.DateField{Name: "fed"},
		&core.TextField{Name: "note"},
		&core.TextField{Name: "due"},
	)
	z.save(t, feedings)

	vault := core.NewBaseCollection("zoo_vault")
	vault.Fields.Add(
		&core.TextField{Name: "name"},
		&core.RelationField{Name: "animal", CollectionId: animals.Id, MaxSelect: 1},
	)
	z.save(t, vault)

	// Animals link to the vault, which no reader may list: a hidden to-one row.
	animals.Fields.Add(&core.RelationField{Name: "vault", CollectionId: vault.Id, MaxSelect: 1})
	z.save(t, animals)

	ann := z.user(t, "ann")
	bob := z.user(t, "bob")
	north := z.rec(t, "north", "zoo_sites", map[string]any{"name": "North", "region": "north"})
	south := z.rec(t, "south", "zoo_sites", map[string]any{"name": "South", "region": "south"})
	leo := z.rec(t, "leo", "zoo_animals", map[string]any{"name": "Leo", "species": "lion", "site": north.Id, "owner": ann.Id, "weight": 190, "price": 500000, "born": "2020-05-01 00:00:00.000Z", "active": true})
	zed := z.rec(t, "zed", "zoo_animals", map[string]any{"name": "Zed", "species": "zebra", "site": south.Id, "owner": bob.Id, "weight": 300, "price": 120000, "born": "2019-02-10 00:00:00.000Z", "active": true})
	mo := z.rec(t, "mo", "zoo_animals", map[string]any{"name": "Mo", "species": "monkey", "site": north.Id, "owner": bob.Id, "weight": 12, "price": 30000, "born": "2022-08-15 00:00:00.000Z"})
	kit := z.rec(t, "kit", "zoo_animals", map[string]any{"name": "Kit", "species": "cat", "owner": ann.Id, "weight": 4, "price": 5000, "active": true})
	z.rec(t, "k1", "zoo_keepers", map[string]any{"user": ann.Id, "animal": leo.Id})
	z.rec(t, "k2", "zoo_keepers", map[string]any{"user": ann.Id, "animal": zed.Id})
	z.rec(t, "k3", "zoo_keepers", map[string]any{"user": bob.Id, "animal": zed.Id})
	z.rec(t, "k4", "zoo_keepers", map[string]any{"user": bob.Id, "animal": mo.Id})
	z.feeding(t, "f1", leo, 1000, "2026-01-05", 500)
	z.feeding(t, "f2", leo, 2500, "2026-02-10", 800)
	z.feeding(t, "f3", zed, 700, "2026-01-20", 300)
	z.feeding(t, "f4", zed, 300, "2026-03-03", 100)
	z.feeding(t, "f5", mo, 150, "2026-02-14", 50)
	z.feeding(t, "f6", kit, 99, "2026-03-30", 20)
	v1 := z.rec(t, "v1", "zoo_vault", map[string]any{"name": "Gold", "animal": leo.Id})
	leo.Set("vault", v1.Id)
	if err := app.Save(leo); err != nil {
		t.Fatal(err)
	}

	registerZooSources()
	return z
}

func registerZooSources() {
	RegisterSource(Source{
		Key: "zoo.people", Collection: "users", Label: "People",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax}},
	})
	RegisterSource(Source{
		Key: "zoo.sites", Collection: "zoo_sites", Label: "Sites",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "region", Label: "Region", Type: Select, Options: []Option{{Value: "north", Label: "North"}, {Value: "south", Label: "South"}}},
		},
		Links: []Link{{Key: "animals", Label: "Animals", Target: "zoo.animals", Cardinality: ToMany, BackField: "site"}},
	})
	RegisterSource(Source{
		Key: "zoo.animals", Collection: "zoo_animals", Label: "Animals",
		Fields: []Field{
			{Key: "name", Label: "Name", Type: Text, Aggregates: MinMax},
			{Key: "species", Label: "Species", Type: Text, Aggregates: CountOnly},
			{Key: "weight", Label: "Weight", Type: Number, Aggregates: SumAll},
			{Key: "price", Label: "Price", Type: Money, Aggregates: SumAll},
			{Key: "born", Label: "Born", Type: Date, Aggregates: MinMax},
			{Key: "active", Label: "Active", Type: Bool},
		},
		Links: []Link{
			{Key: "site", Label: "Site", Target: "zoo.sites", Cardinality: ToOne, Field: "site"},
			{Key: "owner", Label: "Owner", Target: "zoo.people", Cardinality: ToOne, Field: "owner"},
			{Key: "vault", Label: "Vault", Target: "zoo.vault", Cardinality: ToOne, Field: "vault"},
			{Key: "feedings", Label: "Feedings", Target: "zoo.feedings", Cardinality: ToMany, BackField: "animal"},
			{Key: "keepers", Label: "Keepers", Target: "zoo.keepers", Cardinality: ToMany, BackField: "animal"},
		},
		Aliases:  map[string]string{"mass": "weight"},
		Currency: func(core.App) string { return "USD" },
	})
	RegisterSource(Source{
		Key: "zoo.feedings", Collection: "zoo_feedings", Label: "Feedings",
		Fields: []Field{
			{Key: "cost", Label: "Cost", Type: Money, Aggregates: SumAll, Currency: "currency"},
			{Key: "currency", Label: "Currency", Type: Text},
			{Key: "grams", Label: "Grams", Type: Number, Aggregates: SumAll},
			{Key: "fed", Label: "Fed", Type: Date, Aggregates: MinMax},
			{Key: "note", Label: "Note", Type: Text},
			{Key: "due", Label: "Due", Type: Date, TextDay: true, Aggregates: MinMax},
		},
		Links:            []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
		FiscalStartMonth: func(core.App) int { return zooFiscalStart },
	})
	RegisterSource(Source{
		Key: "zoo.keepers", Collection: "zoo_keepers", Label: "Keepers",
		Links: []Link{
			{Key: "user", Label: "User", Target: "zoo.people", Cardinality: ToOne, Field: "user"},
			{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"},
		},
	})
	RegisterSource(Source{
		Key: "zoo.vault", Collection: "zoo_vault", Label: "Vault",
		Fields: []Field{{Key: "name", Label: "Name", Type: Text}},
		Links:  []Link{{Key: "animal", Label: "Animal", Target: "zoo.animals", Cardinality: ToOne, Field: "animal"}},
	})
}

func (z *zoo) save(t testing.TB, col *core.Collection) {
	t.Helper()
	if err := z.app.Save(col); err != nil {
		t.Fatalf("save %s: %v", col.Name, err)
	}
}

func (z *zoo) user(t testing.TB, name string) *core.Record {
	t.Helper()
	users, err := z.app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	u := core.NewRecord(users)
	u.SetEmail(name + "@zoo.test")
	u.SetPassword("password123")
	u.Set("role", "member")
	u.Set("name", name)
	u.Set("username", name)
	if err := z.app.Save(u); err != nil {
		t.Fatalf("save user %s: %v", name, err)
	}
	z.recs[name] = u
	return u
}

func (z *zoo) rec(t testing.TB, key, collection string, data map[string]any) *core.Record {
	t.Helper()
	col, err := z.app.FindCollectionByNameOrId(collection)
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	for k, v := range data {
		r.Set(k, v)
	}
	if err := z.app.Save(r); err != nil {
		t.Fatalf("save %s: %v", key, err)
	}
	z.recs[key] = r
	return r
}

func (z *zoo) feeding(t testing.TB, key string, animal *core.Record, cost int, day string, grams int) *core.Record {
	t.Helper()
	return z.rec(t, key, "zoo_feedings", map[string]any{
		"animal": animal.Id, "cost": cost, "currency": "USD", "fed": day + " 00:00:00.000Z", "grams": grams, "note": key, "due": day,
	})
}

// as is the list request a report run for this user resolves rules with.
func (z *zoo) as(name string) *core.RequestInfo {
	return listRequestInfo(z.recs[name], nil)
}

func (z *zoo) id(key string) string { return z.recs[key].Id }

func (z *zoo) token(t testing.TB, name string) string {
	t.Helper()
	token, err := z.recs[name].NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// mux is the app's real router with every OnServe route bound, as a request
// reaches it in production.
func (z *zoo) mux(t testing.TB) http.Handler {
	t.Helper()
	router, err := apis.NewRouter(z.app)
	if err != nil {
		t.Fatal(err)
	}
	var mux http.Handler
	err = z.app.OnServe().Trigger(&core.ServeEvent{App: z.app, Router: router}, func(e *core.ServeEvent) error {
		built, err := e.Router.BuildMux()
		mux = built
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return mux
}

// listIDs reads ids through PocketBase's list API as user, sorted.
func (z *zoo) listIDs(t testing.TB, mux http.Handler, collection, user, filter string) []string {
	t.Helper()
	q := url.Values{"perPage": {"500"}, "fields": {"id"}, "skipTotal": {"1"}}
	if filter != "" {
		q.Set("filter", filter)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/collections/"+collection+"/records?"+q.Encode(), nil)
	req.Header.Set("Authorization", z.token(t, user))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list %s as %s: %d %s", collection, user, rec.Code, rec.Body.String())
	}
	var body struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(body.Items))
	for _, item := range body.Items {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

// ids maps fixture keys to record ids, sorted.
func (z *zoo) ids(keys ...string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, z.id(k))
	}
	sort.Strings(out)
	return out
}
