package realtime

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWithDocEpochMergesIntoObject(t *testing.T) {
	out, err := withDocEpoch([]byte(`{"readOnly":true,"importWarnings":[]}`), 42)
	if err != nil {
		t.Fatalf("withDocEpoch: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["readOnly"] != true || got["docEpoch"] != float64(42) {
		t.Fatalf("got %v", got)
	}
	if _, ok := got["importWarnings"]; !ok {
		t.Fatal("the kind's own field was dropped")
	}
}

func TestWithDocEpochNilPayload(t *testing.T) {
	for _, in := range [][]byte{nil, {}, []byte("  ")} {
		out, err := withDocEpoch(in, 7)
		if err != nil {
			t.Fatalf("withDocEpoch(%q): %v", in, err)
		}
		if string(out) != `{"docEpoch":7}` {
			t.Fatalf("withDocEpoch(%q) = %s", in, out)
		}
	}
}

func TestWithDocEpochRejectsNonObject(t *testing.T) {
	if _, err := withDocEpoch([]byte(`[1,2]`), 7); err == nil {
		t.Fatal("an array payload was accepted")
	}
	if _, err := withDocEpoch([]byte(`"text"`), 7); err == nil {
		t.Fatal("a string payload was accepted")
	}
}

// A kind with a server document but no OnConnect still gets a hello, and
// it carries the epoch.
func TestHelloCarriesTheEpochWithoutOnConnect(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	rt := newStubRuntime()
	opts := startTestServerWithOpts(t, broker, RoomKindOptions{RuntimeProvider: rt})

	a := dialClient(t, opts, "hello-room", "alice")
	var got map[string]any
	if err := json.Unmarshal(a.hello, &got); err != nil {
		t.Fatalf("hello is not JSON: %v", err)
	}
	room := waitForRoomMembers(t, broker, "test", "hello-room", 1, time.Second)
	if got["docEpoch"] != float64(room.DocEpoch()) || room.DocEpoch() == 0 {
		t.Fatalf("hello docEpoch = %v; want %d", got["docEpoch"], room.DocEpoch())
	}
}

// A pure-relay kind keeps its protocol shape: no hello at all.
func TestNoHelloForPureRelayKind(t *testing.T) {
	broker := NewBroker()
	t.Cleanup(broker.Close)
	opts := startTestServerWithOpts(t, broker, RoomKindOptions{})
	a := dialClient(t, opts, "relay-room", "alice")
	expectNoFrame(t, a.conn, 150*time.Millisecond, "pure relay must send no hello")
}
