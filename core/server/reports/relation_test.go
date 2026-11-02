package reports

import "testing"

// A path that ends at a to-one link reads the linked row's id.
func TestAPathEndingAtALinkReadsTheLinkedRowsID(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[
		{"id":"note","path":[{"field":"note"}]},
		{"id":"animal","path":[{"link":"animal"}]}],"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "animal", z.id("leo"), z.id("leo"), z.id("zed"), z.id("zed"), nil)
	if c := res.Columns[1]; c.Type != Relation || c.Label != "Animal" {
		t.Fatalf("animal column = %+v", c)
	}
}

// A linked row the reader cannot see reads blank, as every to-one value does.
func TestALinkToAHiddenRowReadsBlank(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.animals","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"vault","path":[{"link":"vault"}]}],"sort":[{"col":"name"}]}`, Options{}, nil)
	requireRows(t, res, "name", "Kit", "Leo", "Zed", nil)
	requireRows(t, res, "vault", nil, nil, nil, nil)
}

func TestAFilterComparesARelation(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "ann", `{"version":2,"source":"zoo.feedings","columns":[{"id":"note","path":[{"field":"note"}]}],
		"filter":{"op":"eq","args":[{"path":[{"link":"animal"}]},{"value":"`+z.id("zed")+`"}]},
		"sort":[{"col":"note"}]}`, Options{}, nil)
	requireRows(t, res, "note", "f3", "f4", nil)
}

func TestIfOverRelationsNeedsOneTarget(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.feedings","columns":[{"id":"x","formula":{"op":"if","args":[
		{"value":true},{"path":[{"link":"animal"}]},{"path":[{"link":"animal"},{"link":"site"}]}]}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.0.formula", "link to different sources")
}

func TestATotalAcrossAToManyLinkStillEndsWithAField(t *testing.T) {
	app := withZooRegistry(t)
	spec, _, err := Load([]byte(`{"version":2,"source":"zoo.animals","columns":[{"id":"n","path":[{"link":"feedings","mode":"aggregate"}],"aggregate":"count"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	requireSpecError(t, Validate(app, spec), 400, "columns.0.path", "or with a link to one row")
}
