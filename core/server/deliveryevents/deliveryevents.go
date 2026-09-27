// Package deliveryevents lets a package hand over a provider notification
// about a message this deployment sent, without knowing who receives it.
//
// A notification about a sent message can arrive at something other than the
// package that sent it — a webhook lands wherever the provider was told to
// call, not necessarily inside the sender's own package. Whatever receives it
// must still get the event to the package that owns the message, but it must
// do so without naming that package.
//
// So this is a registry of sinks and nothing else. A package registers a
// function that recognizes and records its own messages; whoever receives a
// notification hands it to the registry instead of deciding for itself who
// sent the message. Core names no package, and a package names no receiver.
package deliveryevents

import (
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/core"
)

// Kind is the outcome a provider is reporting about a sent message.
type Kind string

const (
	Delivered Kind = "delivered"
	Bounced   Kind = "bounced"
	Complaint Kind = "complaint"
)

// Event is one provider notification about a message this deployment sent.
type Event struct {
	Kind              Kind      `json:"kind"`
	ProviderMessageID string    `json:"provider_message_id"`
	Class             string    `json:"class,omitempty"` // "soft" | "hard" | "complaint" | ""
	Reason            string    `json:"reason,omitempty"`
	At                time.Time `json:"at"`
}

// Sink recognizes and records events for the messages its package sent.
// It reports handled = false for an event it does not recognize, so Apply
// can offer the event to the next registered sink.
type Sink func(app core.App, e Event) (handled bool, err error)

type registration struct {
	slug string
	sink Sink
}

var (
	mu     sync.RWMutex
	sinks  []registration
	bySlug = map[string]bool{}
)

// Register adds a sink. Called from a package's own Register at startup.
//
// Idempotent per slug: registering twice keeps the first, so a package
// registered by two compositions does not receive the same event twice.
func Register(slug string, s Sink) {
	if slug == "" || s == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	if bySlug[slug] {
		return
	}
	bySlug[slug] = true
	sinks = append(sinks, registration{slug: slug, sink: s})
}

// Apply offers the event to every registered sink, in registration order,
// until one reports handled = true or returns an error. The first handling
// sink wins: a provider message ID belongs to exactly one package, so once a
// sink claims it there is nothing for the rest to do.
func Apply(app core.App, e Event) (handled bool, err error) {
	mu.RLock()
	snapshot := make([]registration, len(sinks))
	copy(snapshot, sinks)
	mu.RUnlock()

	for _, r := range snapshot {
		ok, err := r.sink(app, e)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// ResetForTesting clears the registry. A test that registers a sink must
// call it (t.Cleanup) so the registration does not leak into the next test.
func ResetForTesting() {
	mu.Lock()
	sinks, bySlug = nil, map[string]bool{}
	mu.Unlock()
}
