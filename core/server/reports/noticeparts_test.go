package reports

import (
	"reflect"
	"strings"
	"testing"
)

// A notice of more than one sentence gives each in parts, in its message's
// order, so pages that share a sentence join it once (the client's
// mergePages): here a blank column's sentence, then a sort key's.
func TestANoticeOfSeveralSentencesGivesThemAsParts(t *testing.T) {
	z := newZoo(t)
	spec := `{"version":3,"source":"zoo.sites","columns":[
		{"id":"name","path":[{"field":"name"}]},
		{"id":"usdFees","formula":` + mixedFees + `}],
		"sort":[{"formula":` + mixedFees + `},{"col":"name"}]}`
	res := z.run(t, "bob", spec, Options{}, nil)
	n := conditionNotice(res, "sort.0.formula")
	if n == nil {
		t.Fatalf("no notice names the sort key: %+v", res.Notices)
	}
	want := []string{
		"A looked-up total mixes currencies, so it shows no value.",
		"The formula of sort key 1 " + sortsAsBlank,
	}
	if !reflect.DeepEqual(n.Parts, want) || n.Message != strings.Join(want, " ") {
		t.Fatalf("parts = %q, message = %q", n.Parts, n.Message)
	}
}

// A notice of one sentence is its message alone: it gives no parts.
func TestANoticeOfOneSentenceGivesNoParts(t *testing.T) {
	z := newZoo(t)
	res := z.run(t, "bob", sitesWithSums, Options{}, nil)
	if len(res.Notices) != 1 || res.Notices[0].Parts != nil {
		t.Fatalf("notices = %+v", res.Notices)
	}
}
