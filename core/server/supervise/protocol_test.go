package supervise

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestProtocolRoundTripsEachType(t *testing.T) {
	for _, m := range []Msg{
		{Type: MsgReady},
		{Type: MsgRestart},
		{Type: MsgRestart, Cold: true},
		{Type: MsgDrain},
	} {
		var buf bytes.Buffer
		if err := Send(&buf, m); err != nil {
			t.Fatalf("Send(%+v): %v", m, err)
		}
		if !strings.HasSuffix(buf.String(), "\n") || strings.Count(buf.String(), "\n") != 1 {
			t.Fatalf("Send(%+v) wrote %q, want one line", m, buf.String())
		}
		got, err := Recv(bufio.NewReader(&buf))
		if err != nil {
			t.Fatalf("Recv after Send(%+v): %v", m, err)
		}
		want := m
		want.Version = ProtocolVersion
		if got != want {
			t.Fatalf("round trip = %+v, want %+v", got, want)
		}
	}
}

// The supervisor matches these bytes, so the wire form is pinned, not only
// the round trip.
func TestProtocolWireForm(t *testing.T) {
	var buf bytes.Buffer
	if err := Send(&buf, Msg{Type: MsgReady}); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); got != `{"type":"ready","version":1}`+"\n" {
		t.Fatalf("ready on the wire = %q", got)
	}
}

func TestProtocolRecvRejectsNonJSON(t *testing.T) {
	_, err := Recv(bufio.NewReader(strings.NewReader("ready\n")))
	if !errors.Is(err, ErrBadMessage) {
		t.Fatalf("Recv of a line that is not JSON = %v, want ErrBadMessage", err)
	}
}

func TestProtocolRecvReadsMessagesInOrder(t *testing.T) {
	r := bufio.NewReader(strings.NewReader(`{"type":"ready","version":1}` + "\n" + `{"type":"drain","version":1}` + "\n"))
	for _, want := range []string{MsgReady, MsgDrain} {
		m, err := Recv(r)
		if err != nil || m.Type != want {
			t.Fatalf("Recv = %+v %v, want type %q", m, err, want)
		}
	}
}
