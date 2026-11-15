package reports

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
)

// sightingNotes offers the notes of the sightings seen in the period, a
// date-time-filtered option list.
const sightingNotes = `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"period","type":"dateRange"},
		{"id":"notes","type":"choice","multiple":true,"optionsFrom":{"source":"zoo.sightings",
			"value":[{"field":"note"}],"label":[{"field":"note"}],
			"where":{"op":"between","args":[{"path":[{"field":"seen"}]},{"param":"period"}]}}}],
	"columns":[{"id":"note","path":[{"field":"note"}]}]}`

// sightingNotesOnDay offers the notes of the sightings seen on or after a typed
// day, a where that compares with a day typed in the condition, not a param.
const sightingNotesOnDay = `{"version":4,"source":"zoo.sightings",
	"params":[{"id":"notes","type":"choice","multiple":true,"optionsFrom":{"source":"zoo.sightings",
		"value":[{"field":"note"}],"label":[{"field":"note"}],
		"where":{"op":"gte","args":[{"path":[{"field":"seen"}]},{"value":"2026-11-14"}]}}}],
	"columns":[{"id":"note","path":[{"field":"note"}]}]}`

func TestParamOptionsReadInTheReadersZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	given := map[string]json.RawMessage{"period": json.RawMessage(nov14)}
	for name, c := range map[string]struct {
		zone string
		want []string
	}{
		"Chicago":         {chicagoZone, []string{"end14", "start14"}},
		"no zone":         {"", []string{"before14", "start14"}},
		"an unknown zone": {"Not/AZone", []string{"before14", "start14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.options(t, "ann", sightingNotes, OptionsRequest{Param: "notes", Params: given, TimeZone: c.zone})
			if got := optionLabels(res); !slices.Equal(got, c.want) {
				t.Fatalf("options = %v, want %v", got, c.want)
			}
		})
	}
}

// A typed day in an options where is a day of the reader's zone, as a param
// day is (R1): from Chicago's Nov 14 start, not the UTC day.
func TestParamOptionsTypedDayReadsInTheReadersZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	for name, c := range map[string]struct {
		zone string
		want []string
	}{
		"Chicago":         {chicagoZone, []string{"after14", "end14", "start14"}},
		"no zone":         {"", []string{"after14", "before14", "end14", "start14"}},
		"an unknown zone": {"Not/AZone", []string{"after14", "before14", "end14", "start14"}},
	} {
		t.Run(name, func(t *testing.T) {
			res := z.options(t, "ann", sightingNotesOnDay, OptionsRequest{Param: "notes", TimeZone: c.zone})
			if got := optionLabels(res); !slices.Equal(got, c.want) {
				t.Fatalf("options = %v, want %v", got, c.want)
			}
		})
	}
}

// The endpoint takes "timeZone" (decodeStrict refuses an unknown key) and
// answers in the zone.
func TestParamOptionsEndpointTakesTimeZone(t *testing.T) {
	z := newZoo(t)
	addZooSightings(t, z)
	Register(z.app)
	body := `{"spec":` + sightingNotes + `,"param":"notes","params":{"period":` + nov14 + `},"timeZone":"` + chicagoZone + `"}`
	sc := tests.ApiScenario{
		Name: "options in the reader's zone", Method: http.MethodPost, URL: "/api/reports/param-options",
		Body:                  strings.NewReader(body),
		Headers:               map[string]string{"Authorization": z.token(t, "ann")},
		ExpectedStatus:        http.StatusOK,
		ExpectedContent:       []string{`"label":"start14"`, `"label":"end14"`},
		NotExpectedContent:    []string{`"label":"before14"`, `"label":"after14"`},
		TestAppFactory:        func(testing.TB) *tests.TestApp { return z.app },
		DisableTestAppCleanup: true,
	}
	sc.Test(t)
}
