package apis

import (
	"runtime"
	"testing"

	"github.com/pocketbase/pocketbase/tools/subscriptions"
)

// The resume drain sends through a realtimeDrainClient. Its sends must be
// inline, as for the ResumableClient it wraps, or two drained records reach
// the subscriber in no fixed order.
func TestRealtimeSendToDrainClientIsInline(t *testing.T) {
	// one P: a send from a new goroutine cannot run before this one yields
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))

	budget := subscriptions.NewQueueBudget(1 << 20)
	client := subscriptions.NewResumableClient(subscriptions.ResumableOptions{
		MaxMessages: 10, MaxBytes: 1 << 20, MaxPending: 10, Budget: budget,
	})
	drain := &realtimeDrainClient{Client: client}

	realtimeSend(drain, subscriptions.Message{Name: "test", Data: []byte(`{}`)})

	// a detached client reserves budget for each queued message
	if budget.Total() == 0 {
		t.Fatal("expected the message to be queued before realtimeSend returned")
	}
}
