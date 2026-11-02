package reports

import (
	"encoding/json"
	"testing"
)

// A version 1 spec loads unchanged as version 2: the step is the identity,
// so the run says nothing about an upgrade and a save stores version 2.
func TestAV1SpecLoadsSilentlyAsVersion2(t *testing.T) {
	z := newZoo(t)
	raw := `{"version":1,"source":"zoo.feedings","columns":[{"id":"cost","path":[{"field":"cost"}],"aggregate":"sum"}]}`
	spec, info, err := Load([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if spec.Version != 2 || info.UpgradedFrom != 0 {
		t.Fatalf("version %d, upgradedFrom %d", spec.Version, info.UpgradedFrom)
	}
	res := z.run(t, "ann", raw, Options{}, nil)
	if len(res.Notices) != 0 {
		t.Fatalf("notices = %+v", res.Notices)
	}
	_, stored, err := loadStored([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(stored, &doc); err != nil || doc["version"] != float64(2) {
		t.Fatalf("stored = %s", stored)
	}
}

// A step that changes the document still says so.
func TestAChangingStepIsNoticed(t *testing.T) {
	withZooRegistry(t)
	steps := map[int]upgradeStep{1: func(doc map[string]any) error {
		doc["columns"] = doc["cols"]
		delete(doc, "cols")
		return nil
	}}
	_, info, err := loadWith([]byte(`{"version":1,"source":"zoo.feedings","cols":[]}`), steps, 2)
	if err != nil || info.UpgradedFrom != 1 {
		t.Fatalf("info %+v, err %v", info, err)
	}
}

func TestAVersion3SpecIsRefused(t *testing.T) {
	_, _, err := Load([]byte(`{"version":3,"source":"zoo.feedings","columns":[]}`))
	requireSpecError(t, err, 400, "version", "newer than this server reads (2)")
}
