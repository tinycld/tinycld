package reports

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/tools/subscriptions"

	"tinycld.org/core/changesignal"
)

// A saved run's report and share topics name only the rows that can change
// that run: the report itself (its view rule), and this report's share rows
// of the reader (the list rule, the reader's own auth in the filter). A
// write to any other report or share row sends the open report nothing.

// reportTopic is the record topic of one saved report, under its view rule.
func reportTopic(id string) string { return "reports/" + id }

// shareTopic is the topic of one report's share rows of one reader: the list
// rule with a filter on the report id and the reader's user id, both record
// ids. The filter cannot read @request.auth: PocketBase refuses a client
// filter that names @request. (apis checkForSuperuserOnlyRuleFields).
func shareTopic(id, reader string) string {
	options := fmt.Sprintf(`{"query":{"filter":"report = '%s' && user = '%s'"}}`, id, reader)
	return "report_shares/*?options=" + url.QueryEscape(options)
}

// zooDataTopics are the change-signal topics of savedSpec's sources.
var zooDataTopics = []string{"collection/zoo_animals", "collection/zoo_feedings", "collection/zoo_keepers"}

// topicsPart is the "topics" member of a run body, sorted as the run sorts.
func topicsPart(topics ...string) string {
	sorted := slices.Clone(topics)
	slices.Sort(sorted)
	b, err := json.Marshal(sorted)
	if err != nil {
		panic(err)
	}
	return `"topics":` + string(b)
}

// savedTopics is the topics of reader's saved run of savedSpec on report id.
func savedTopics(id, reader string) string {
	return topicsPart(append(slices.Clone(zooDataTopics), shareTopic(id, reader), reportTopic(id))...)
}

// savedOnlyTopics is the topics of reader's manual saved run on report id.
func savedOnlyTopics(id, reader string) string {
	return topicsPart(shareTopic(id, reader), reportTopic(id))
}

func runTopics(t *testing.T, s *savedZoo, user, body string) []string {
	t.Helper()
	text := s.expect(t, user, http.MethodPost, runURL, body, http.StatusOK)
	var res struct {
		Topics []string `json:"topics"`
	}
	if err := json.Unmarshal([]byte(text), &res); err != nil {
		t.Fatalf("run body: %v\n%s", err, text)
	}
	return res.Topics
}

func TestASavedRunNamesItsOwnReportAndShareRows(t *testing.T) {
	s := newSavedZoo(t)
	r := s.id("R")
	got := runTopics(t, s, "vi", savedRun(r))
	for _, wide := range []string{"collection/reports", "collection/report_shares"} {
		if slices.Contains(got, wide) {
			t.Fatalf("a saved run listens to every write of %s: %v", wide, got)
		}
	}
	want := append(slices.Clone(zooDataTopics), shareTopic(r, s.id("vi")), reportTopic(r))
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("saved run topics\n got %q\nwant %q", got, want)
	}
	// The owner's run of the same report names the owner's share rows.
	ownerWant := append(slices.Clone(zooDataTopics), shareTopic(r, s.id("ann")), reportTopic(r))
	slices.Sort(ownerWant)
	if owner := runTopics(t, s, "ann", savedRun(r)); !slices.Equal(owner, ownerWant) {
		t.Fatalf("owner's run topics\n got %q\nwant %q", owner, ownerWant)
	}
	// The data topics are those of a draft run of the same spec, and a draft
	// has no saved-report topic.
	draft := runTopics(t, s, "vi", runBodyOf(savedSpec))
	if !slices.Equal(draft, zooDataTopics) {
		t.Fatalf("draft run topics %q, want the data topics %q", draft, zooDataTopics)
	}
	// A report id that is no record id never reaches a topic: it is refused.
	s.expect(t, "vi", http.MethodPost, runURL, savedRun(r+`' || 1=1`), http.StatusForbidden, `"no_access"`)
}

// heard is one message a listener received: its topic and record action.
type heard struct{ topic, action string }

type topicListener struct {
	conn *subscriptions.ResumableConn
}

// listenAs subscribes a realtime client signed in as user to topics, as the
// realtime subscribe request leaves it in the broker (a resumable client, as
// apis.realtimeNewClient makes, attached at 0).
func listenAs(t *testing.T, s *savedZoo, user string, topics []string) *topicListener {
	t.Helper()
	auth, err := s.app.FindRecordById("users", s.id(user))
	if err != nil {
		t.Fatal(err)
	}
	client := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	client.Set(apis.RealtimeClientAuthKey, auth)
	client.Subscribe(topics...)
	s.app.SubscriptionsBroker().Register(client)
	conn, ok := client.Attach(0)
	if !ok {
		t.Fatal("a new resumable client refused Attach(0)")
	}
	t.Cleanup(func() { s.app.SubscriptionsBroker().Unregister(client.Id()) })
	return &topicListener{conn: conn}
}

// quiet is longer than the change signal's window, so a change-signal
// message of the write has come by the time collect returns.
const quiet = 3 * changesignal.Window

// collect returns every message that arrives within quiet.
func (l *topicListener) collect(t *testing.T) []heard {
	t.Helper()
	var got []heard
	deadline := time.After(quiet)
	for {
		select {
		case msg := <-l.conn.Channel():
			var data struct {
				Action string `json:"action"`
			}
			_ = json.Unmarshal(msg.Data, &data)
			got = append(got, heard{msg.Name, data.Action})
		case <-deadline:
			return got
		}
	}
}

// newLiveSavedZoo is the saved zoo with the change signal bound, as the
// server binds it, so a run's data topics deliver as in production.
func newLiveSavedZoo(t *testing.T) *savedZoo {
	t.Helper()
	s := newSavedZoo(t)
	changesignal.Register(s.app)
	return s
}

// The writes that cannot change vi's run of R send it nothing.
func TestASavedRunHearsNoOtherReportOrReader(t *testing.T) {
	s := newLiveSavedZoo(t)
	r := s.id("R")
	l := listenAs(t, s, "vi", runTopics(t, s, "vi", savedRun(r)))
	silent := func(what string) {
		t.Helper()
		if got := l.collect(t); len(got) != 0 {
			t.Fatalf("%s: the open report heard %v", what, got)
		}
	}

	// R2, which vi can also view, is edited by its owner.
	s.expect(t, "cy", http.MethodPatch, reportURL(s.id("R2")), `{"title":"R2 renamed"}`, http.StatusOK)
	silent("another report edited")
	// vi is given a share of another report.
	s.expect(t, "ann", http.MethodPost, sharesURL,
		fmt.Sprintf(`{"report":%q,"user":%q,"role":"viewer"}`, s.id("P"), s.id("vi")), http.StatusOK)
	silent("a share of another report added")
	// The owner shares R with another user.
	s.expect(t, "ann", http.MethodPost, sharesURL,
		fmt.Sprintf(`{"report":%q,"user":%q,"role":"viewer"}`, r, s.id("ot")), http.StatusOK)
	silent("another user's share of this report added")
	// The owner removes another user's share of R.
	s.expect(t, "ann", http.MethodDelete, shareURL(s.id("R.ed")), "", http.StatusNoContent)
	silent("another user's share of this report removed")

	// The control: the listener hears a write that changes the run, so the
	// silent cases above are silent because of the topics, not because the
	// record broadcast is not bound.
	s.expect(t, "ann", http.MethodPatch, reportURL(r), `{"title":"R renamed"}`, http.StatusOK)
	if got := l.collect(t); !slices.Contains(got, heard{reportTopic(r), "update"}) {
		t.Fatalf("this report edited: want update on %s, heard %v", reportTopic(r), got)
	}
}

// readerSharesTopic writes only record ids into the filter text: any other
// text is refused before it reaches the filter.
func TestReaderSharesTopicTakesOnlyRecordIDs(t *testing.T) {
	const report, reader = "r12345678901234", "u12345678901234"
	got, err := readerSharesTopic(report, reader)
	if err != nil {
		t.Fatalf("record ids refused: %v", err)
	}
	want := "report_shares/*?options=" +
		"%7B%22query%22%3A%7B%22filter%22%3A%22report+%3D+%27r12345678901234%27+%26%26+user+%3D+%27u12345678901234%27%22%7D%7D"
	if got != want {
		t.Fatalf("shares topic\n got %q\nwant %q", got, want)
	}
	for _, bad := range []struct{ what, report, reader string }{
		{"a report id with a quote", report + `' || 1=1`, reader},
		{"a reader id with a quote", report, reader + `' || 1=1`},
		{"an empty report id", "", reader},
		{"an empty reader id", report, ""},
	} {
		if topic, err := readerSharesTopic(bad.report, bad.reader); err == nil {
			t.Fatalf("%s: want an error, got topic %q", bad.what, topic)
		}
	}
}

// The writes that change vi's run of R reach it on the topic that names them.
func TestASavedRunHearsItsReportAndItsReadersShares(t *testing.T) {
	s := newLiveSavedZoo(t)
	r := s.id("R")
	share := shareTopic(r, s.id("vi"))
	topics := runTopics(t, s, "vi", savedRun(r))
	for _, want := range []string{reportTopic(r), share} {
		if !slices.Contains(topics, want) {
			t.Fatalf("the run has no topic %s: %v", want, topics)
		}
	}
	l := listenAs(t, s, "vi", topics)
	hears := func(what, topic, action string) {
		t.Helper()
		got := l.collect(t)
		if !slices.Contains(got, heard{topic, action}) {
			t.Fatalf("%s: want %s on %s, heard %v", what, action, topic, got)
		}
	}

	s.expect(t, "ann", http.MethodPatch, reportURL(r), `{"spec":`+feedingNotes+`}`, http.StatusOK)
	hears("the report's spec edited", reportTopic(r), "update")
	s.expect(t, "ann", http.MethodPatch, shareURL(s.id("R.vi")), `{"role":"editor"}`, http.StatusOK)
	hears("the reader's share role changed", share, "update")
	s.expect(t, "ann", http.MethodDelete, shareURL(s.id("R.vi")), "", http.StatusNoContent)
	hears("the reader's share removed", share, "delete")
	s.expect(t, "ann", http.MethodPost, sharesURL,
		fmt.Sprintf(`{"report":%q,"user":%q,"role":"viewer"}`, r, s.id("vi")), http.StatusOK)
	hears("the reader's share added", share, "create")
	// A grant to a group vi is in writes a derived row of vi.
	crew := s.group(t, "crew", "vi")
	s.expect(t, "ann", http.MethodPost, sharesURL,
		fmt.Sprintf(`{"report":%q,"group":%q,"role":"viewer"}`, r, crew.Id), http.StatusOK)
	hears("a group grant to the reader's group added", share, "create")
	s.expect(t, "ann", http.MethodDelete, reportURL(r), "", http.StatusNoContent)
	hears("the report deleted", reportTopic(r), "delete")
}

// A reader who sees a report only because it is published hears when it is
// unpublished: the report leaves the reader's view.
func TestASavedRunHearsItsReportUnpublished(t *testing.T) {
	s := newLiveSavedZoo(t)
	pub := s.saved(t, "adP", "ad", "adP", savedSpec, true)
	topics := runTopics(t, s, "ot", savedRun(pub.Id))
	if !slices.Contains(topics, reportTopic(pub.Id)) {
		t.Fatalf("the run has no topic %s: %v", reportTopic(pub.Id), topics)
	}
	l := listenAs(t, s, "ot", topics)
	s.expect(t, "ad", http.MethodPatch, reportURL(pub.Id), `{"published":false}`, http.StatusOK)
	got := l.collect(t)
	if !slices.Contains(got, heard{reportTopic(pub.Id), "delete"}) {
		t.Fatalf("unpublished: want delete on %s, heard %v", reportTopic(pub.Id), got)
	}
}
