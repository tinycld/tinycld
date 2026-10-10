package subscriptions_test

import (
	"encoding/json/v2"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

func receive(t *testing.T, ch chan subscriptions.Message) subscriptions.Message {
	t.Helper()

	select {
	case msg, ok := <-ch:
		if !ok {
			t.Fatal("channel closed")
		}
		return msg
	case <-time.After(time.Second):
		t.Fatal("no message")
	}

	return subscriptions.Message{}
}

func expectClosed(t *testing.T, ch chan subscriptions.Message) {
	t.Helper()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("expected a closed channel, got a message")
		}
	case <-time.After(time.Second):
		t.Fatal("channel is still open")
	}
}

func expectNothing(t *testing.T, ch chan subscriptions.Message) {
	t.Helper()

	select {
	case msg := <-ch:
		t.Fatalf("expected no message, got %s", msg.Data)
	case <-time.After(50 * time.Millisecond):
	}
}

func seqOf(t *testing.T, msg subscriptions.Message) uint64 {
	t.Helper()

	var payload struct {
		Seq uint64 `json:"seq"`
	}
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		t.Fatalf("payload %s: %v", msg.Data, err)
	}

	return payload.Seq
}

func sendN(c *subscriptions.ResumableClient, n int) {
	for i := 0; i < n; i++ {
		c.Send(subscriptions.Message{Name: "topic", Data: []byte(`{"n":` + strconv.Itoa(i) + `}`)})
	}
}

func attach(t *testing.T, c *subscriptions.ResumableClient, after uint64) *subscriptions.ResumableConn {
	t.Helper()

	conn, ok := c.Attach(after)
	if !ok {
		t.Fatalf("Attach(%d) refused (last sent %d)", after, c.LastSent())
	}

	return conn
}

func TestResumableSeqPayload(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	conn := attach(t, c, 0)

	c.Send(subscriptions.Message{Name: "a", Data: []byte(`{"action":"create","record":{"id":"x"}}`)})
	c.Send(subscriptions.Message{Name: "b", Data: []byte(`{}`)})
	c.Send(subscriptions.Message{Name: "c", Data: []byte(`plain`)})

	expected := []string{
		`{"seq":1,"action":"create","record":{"id":"x"}}`,
		`{"seq":2}`,
		`plain`,
	}
	for _, want := range expected {
		if got := string(receive(t, conn.Channel()).Data); got != want {
			t.Fatalf("expected %s, got %s", want, got)
		}
	}
}

func TestResumableOrderUnderConcurrentSends(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	conn := attach(t, c, 0)

	const total = 200
	var wg sync.WaitGroup
	for i := 0; i < total; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Send(subscriptions.Message{Name: "topic", Data: []byte(`{}`)})
		}()
	}

	for i := uint64(1); i <= total; i++ {
		if seq := seqOf(t, receive(t, conn.Channel())); seq != i {
			t.Fatalf("expected seq %d, got %d", i, seq)
		}
	}
	wg.Wait()
}

func TestResumableSendDoesNotBlockWhileDetached(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})

	done := make(chan struct{})
	go func() {
		sendN(c, 100)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Send blocked on a detached client")
	}
}

func TestResumableResumeReplaysQueued(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	conn := attach(t, c, 0)

	sendN(c, 2)
	receive(t, conn.Channel())
	receive(t, conn.Channel())

	if !conn.Detach() {
		t.Fatal("Detach failed")
	}
	if !c.Detached(conn.Gen()) {
		t.Fatal("expected the client to be detached")
	}

	sendN(c, 3)

	resumed := attach(t, c, 2)
	if c.Detached(conn.Gen()) {
		t.Fatal("expected the client to be attached")
	}
	for want := uint64(3); want <= 5; want++ {
		if seq := seqOf(t, receive(t, resumed.Channel())); seq != want {
			t.Fatalf("expected seq %d, got %d", want, seq)
		}
	}
	expectNothing(t, resumed.Channel())
}

func TestResumableRefusesSkippedMessages(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	conn := attach(t, c, 0)

	sendN(c, 2)
	receive(t, conn.Channel())
	receive(t, conn.Channel())
	conn.Detach()

	// the subscriber got only seq 1: seq 2 was lost in the dead connection
	if _, ok := c.Attach(1); ok {
		t.Fatal("expected Attach(1) to be refused after seq 2 was sent")
	}
	if _, ok := c.Attach(3); ok {
		t.Fatal("expected Attach(3) to be refused: seq 3 was never sent")
	}
	attach(t, c, 2)
}

func TestResumableRequeuesUntakenMessage(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	conn := attach(t, c, 0)

	// nobody reads the channel, so seq 1 is never handed over, whether or
	// not the pump has taken it from the queue yet
	sendN(c, 1)
	conn.Detach()

	resumed := attach(t, c, 0)
	if seq := seqOf(t, receive(t, resumed.Channel())); seq != 1 {
		t.Fatalf("expected seq 1, got %d", seq)
	}
}

func TestResumablePreemptsAttachedConnection(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	old := attach(t, c, 0)

	conn := attach(t, c, 0)
	expectClosed(t, old.Channel())

	if old.Detach() {
		t.Fatal("expected a stale Detach to do nothing")
	}

	sendN(c, 1)
	if seq := seqOf(t, receive(t, conn.Channel())); seq != 1 {
		t.Fatalf("expected seq 1, got %d", seq)
	}
}

func TestResumableOverflowWhileDetached(t *testing.T) {
	var overflowed *subscriptions.ResumableClient
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{
		MaxMessages: 2,
		OnOverflow: func(client *subscriptions.ResumableClient) {
			overflowed = client
			client.Discard()
		},
	})

	sendN(c, 2)
	if overflowed != nil {
		t.Fatal("overflowed at the limit")
	}

	sendN(c, 1)
	if overflowed != c {
		t.Fatal("expected OnOverflow past the limit")
	}
	if _, ok := c.Attach(0); ok {
		t.Fatal("expected an overflowed client to refuse Attach")
	}
}

func TestResumableOverflowDiscardsByDefault(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{MaxBytes: 10})

	c.Send(subscriptions.Message{Name: "topic", Data: []byte(`{"long":"value"}`)})

	if !c.IsDiscarded() {
		t.Fatal("expected the client to be discarded")
	}
}

func TestResumableBudget(t *testing.T) {
	budget := subscriptions.NewQueueBudget(40)
	a := subscriptions.NewResumableClient(subscriptions.ResumableOptions{Budget: budget})
	b := subscriptions.NewResumableClient(subscriptions.ResumableOptions{Budget: budget})

	queued := len(`{"seq":1,"v":"aaaaaaaa"}`)
	a.Send(subscriptions.Message{Name: "topic", Data: []byte(`{"v":"aaaaaaaa"}`)})
	if budget.Total() != queued {
		t.Fatalf("expected %d reserved bytes, got %d", queued, budget.Total())
	}

	b.Send(subscriptions.Message{Name: "topic", Data: []byte(`{"v":"bbbbbbbb"}`)})
	if !b.IsDiscarded() {
		t.Fatal("expected the client past the budget to overflow")
	}
	if a.IsDiscarded() {
		t.Fatal("expected the client within the budget to stay")
	}
	if budget.Total() != queued {
		t.Fatalf("expected the overflowed client to release its bytes, got %d", budget.Total())
	}

	// an attached client reserves nothing
	conn := attach(t, a, 0)
	if budget.Total() != 0 {
		t.Fatalf("expected Attach to release the reservation, got %d", budget.Total())
	}
	receive(t, conn.Channel())

	// detaching reserves what is still queued
	a.Send(subscriptions.Message{Name: "topic", Data: []byte(`{}`)})
	conn.Detach()
	if budget.Total() != len(`{"seq":2}`) {
		t.Fatalf("expected the queued message to be reserved, got %d", budget.Total())
	}

	a.Discard()
	if budget.Total() != 0 {
		t.Fatalf("expected Discard to release everything, got %d", budget.Total())
	}
}

func TestResumableDiscard(t *testing.T) {
	c := subscriptions.NewResumableClient(subscriptions.ResumableOptions{})
	conn := attach(t, c, 0)

	c.Discard()
	expectClosed(t, conn.Channel())

	c.Send(subscriptions.Message{Name: "topic", Data: []byte(`{}`)})
	if conn.Detach() {
		t.Fatal("expected Detach of a discarded client to fail")
	}
	if c.Detached(conn.Gen()) {
		t.Fatal("expected a discarded client not to report detached")
	}
	if _, ok := c.Attach(0); ok {
		t.Fatal("expected a discarded client to refuse Attach")
	}

	// safe to call again
	c.Discard()
}
