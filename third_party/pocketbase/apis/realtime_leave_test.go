package apis_test

import (
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/subscriptions"
	"github.com/pocketbase/pocketbase/tools/types"
)

// leaveMessage is the decoded shape of a realtime record event.
type leaveMessage struct {
	Action string         `json:"action"`
	Record map[string]any `json:"record"`
}

// leaveTopic builds a subscription topic with the given filter.
func leaveTopic(topic string, filter string) string {
	if filter == "" {
		return topic
	}

	options, _ := json.Marshal(map[string]any{"query": map[string]string{"filter": filter}})

	return topic + "?options=" + url.QueryEscape(string(options))
}

func newLeaveCollection(t *testing.T, app core.App, name string, listRule string) *core.Collection {
	t.Helper()

	collection := core.NewBaseCollection(name)
	collection.ListRule = types.Pointer(listRule)
	collection.ViewRule = types.Pointer(listRule)
	collection.Fields.Add(
		&core.TextField{Name: "status"},
		&core.TextField{Name: "note", Max: 20},
	)
	if err := app.Save(collection); err != nil {
		t.Fatal(err)
	}

	return collection
}

func newLeaveRecord(t *testing.T, app core.App, collection *core.Collection, status string) *core.Record {
	t.Helper()

	record := core.NewRecord(collection)
	record.Set("status", status)
	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}

	return record
}

func newLeaveClient(app core.App, auth *core.Record, topics ...string) *subscriptions.DefaultClient {
	client := subscriptions.NewDefaultClient()
	if auth != nil {
		client.Set(apis.RealtimeClientAuthKey, auth)
	}
	client.Subscribe(topics...)
	app.SubscriptionsBroker().Register(client)

	return client
}

// drainMessages reads every message the client receives until the channel
// stays quiet for the grace period. Sends are fire-and-forget goroutines, so
// their order is not guaranteed and the result is treated as a set.
func drainMessages(client *subscriptions.DefaultClient) []leaveMessage {
	var result []leaveMessage

	for {
		select {
		case msg := <-client.Channel():
			var decoded leaveMessage
			_ = json.Unmarshal(msg.Data, &decoded)
			result = append(result, decoded)
		case <-time.After(300 * time.Millisecond):
			return result
		}
	}
}

func countActions(messages []leaveMessage) map[string]int {
	counts := map[string]int{}
	for _, m := range messages {
		counts[m.Action]++
	}
	return counts
}

func expectActions(t *testing.T, label string, client *subscriptions.DefaultClient, expected map[string]int) []leaveMessage {
	t.Helper()

	messages := drainMessages(client)
	actual := countActions(messages)

	if len(actual) != len(expected) {
		t.Fatalf("[%s] expected actions %v, got %v", label, expected, actual)
	}

	for action, total := range expected {
		if actual[action] != total {
			t.Fatalf("[%s] expected actions %v, got %v", label, expected, actual)
		}
	}

	return messages
}

func updateLeaveRecord(t *testing.T, app core.App, id string, data map[string]any) {
	t.Helper()

	record, err := app.FindRecordById("test_leave", id)
	if err != nil {
		t.Fatal(err)
	}

	record.Load(data)

	if err := app.Save(record); err != nil {
		t.Fatal(err)
	}
}

func TestRealtimeLeaveFilteredSubscription(t *testing.T) {
	t.Parallel()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	if _, err := apis.NewRouter(testApp); err != nil {
		t.Fatal(err)
	}

	collection := newLeaveCollection(t, testApp, "test_leave", "@request.auth.id != ''")
	record := newLeaveRecord(t, testApp, collection, "open")

	user, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	filtered := newLeaveClient(testApp, user, leaveTopic("test_leave/*", "status = 'open'"))
	byId := newLeaveClient(testApp, user, leaveTopic("test_leave/"+record.Id, "status = 'open'"))
	plain := newLeaveClient(testApp, user, "test_leave/*")

	// an unrelated field keeps the record in the filter
	updateLeaveRecord(t, testApp, record.Id, map[string]any{"note": "a"})
	expectActions(t, "filtered/unrelated", filtered, map[string]int{"update": 1})
	expectActions(t, "byId/unrelated", byId, map[string]int{"update": 1})
	expectActions(t, "plain/unrelated", plain, map[string]int{"update": 1})

	// the record leaves the filter
	updateLeaveRecord(t, testApp, record.Id, map[string]any{"status": "closed"})
	leaves := expectActions(t, "filtered/leave", filtered, map[string]int{"delete": 1})
	expectActions(t, "byId/leave", byId, map[string]int{"delete": 1})
	expectActions(t, "plain/leave", plain, map[string]int{"update": 1})

	expectedRecord := map[string]any{
		"id":             record.Id,
		"collectionId":   collection.Id,
		"collectionName": collection.Name,
	}
	if len(leaves[0].Record) != len(expectedRecord) {
		t.Fatalf("expected leave record %v, got %v", expectedRecord, leaves[0].Record)
	}
	for k, v := range expectedRecord {
		if leaves[0].Record[k] != v {
			t.Fatalf("expected leave record %v, got %v", expectedRecord, leaves[0].Record)
		}
	}

	// a record outside the filter changes an unrelated field
	updateLeaveRecord(t, testApp, record.Id, map[string]any{"note": "b"})
	expectActions(t, "filtered/outside", filtered, map[string]int{})
	expectActions(t, "byId/outside", byId, map[string]int{})
	expectActions(t, "plain/outside", plain, map[string]int{"update": 1})

	// the record enters the filter again
	updateLeaveRecord(t, testApp, record.Id, map[string]any{"status": "open"})
	expectActions(t, "filtered/enter", filtered, map[string]int{"update": 1})
	expectActions(t, "byId/enter", byId, map[string]int{"update": 1})
	expectActions(t, "plain/enter", plain, map[string]int{"update": 1})
}

func TestRealtimeLeaveRuleChange(t *testing.T) {
	t.Parallel()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	if _, err := apis.NewRouter(testApp); err != nil {
		t.Fatal(err)
	}

	collection := newLeaveCollection(t, testApp, "test_leave", "@request.auth.id != '' && status = 'open'")
	record := newLeaveRecord(t, testApp, collection, "open")

	user, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	superuser, err := testApp.FindAuthRecordByEmail(core.CollectionNameSuperusers, "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	userClient := newLeaveClient(testApp, user, "test_leave/*")
	guestClient := newLeaveClient(testApp, nil, "test_leave/*")
	superuserClient := newLeaveClient(testApp, superuser, "test_leave/*")
	superuserFiltered := newLeaveClient(testApp, superuser, leaveTopic("test_leave/*", "status = 'open'"))

	updateLeaveRecord(t, testApp, record.Id, map[string]any{"status": "closed"})

	// the rule stopped matching, so the user is told only that the record is gone
	expectActions(t, "user", userClient, map[string]int{"delete": 1})
	// never visible, so nothing to leave
	expectActions(t, "guest", guestClient, map[string]int{})
	// rules do not apply to superusers
	expectActions(t, "superuser", superuserClient, map[string]int{"update": 1})
	// but their own filter does
	expectActions(t, "superuser filtered", superuserFiltered, map[string]int{"delete": 1})
}

func TestRealtimeLeaveUpdateFailure(t *testing.T) {
	t.Parallel()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	if _, err := apis.NewRouter(testApp); err != nil {
		t.Fatal(err)
	}

	collection := newLeaveCollection(t, testApp, "test_leave", "@request.auth.id != ''")
	record := newLeaveRecord(t, testApp, collection, "open")

	user, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	filtered := newLeaveClient(testApp, user, leaveTopic("test_leave/*", "status = 'open'"))

	toUpdate, err := testApp.FindRecordById("test_leave", record.Id)
	if err != nil {
		t.Fatal(err)
	}
	toUpdate.Set("status", "closed")
	toUpdate.Set("note", "this note is longer than twenty characters")

	if err := testApp.Save(toUpdate); err == nil {
		t.Fatal("expected the update to fail validation")
	}

	expectActions(t, "filtered", filtered, map[string]int{})

	if parked := filtered.Get("leave/test_leave/" + record.Id); parked != nil {
		t.Fatalf("expected the parked leave candidates to be cleared, got %v", parked)
	}
}

func TestRealtimeLeaveInTransaction(t *testing.T) {
	t.Parallel()

	testApp, _ := tests.NewTestApp()
	defer testApp.Cleanup()

	if _, err := apis.NewRouter(testApp); err != nil {
		t.Fatal(err)
	}

	collection := newLeaveCollection(t, testApp, "test_leave", "@request.auth.id != ''")
	record := newLeaveRecord(t, testApp, collection, "open")

	user, err := testApp.FindAuthRecordByEmail("users", "test@example.com")
	if err != nil {
		t.Fatal(err)
	}

	filtered := newLeaveClient(testApp, user, leaveTopic("test_leave/*", "status = 'open'"))

	err = testApp.RunInTransaction(func(txApp core.App) error {
		updateLeaveRecord(t, txApp, record.Id, map[string]any{"status": "closed"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	expectActions(t, "filtered", filtered, map[string]int{"delete": 1})
}
