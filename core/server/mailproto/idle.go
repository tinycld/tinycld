package mailproto

import (
	"sync"
)

// IdleNotifier provides a pub/sub mechanism for notifying IMAP sessions about
// mailbox changes. Sessions subscribe when entering IDLE and unsubscribe when
// IDLE ends; record hooks publish notifications.
//
// Exposed as a type rather than a package global so each host owns its own
// instance — a hosting host running several tenants must not let one tenant's
// notifications reach another's IDLE sessions.
type IdleNotifier struct {
	mu          sync.Mutex
	subscribers map[string]map[chan struct{}]struct{} // mailboxID → set of channels
}

// NewIdleNotifier returns a ready-to-use notifier.
func NewIdleNotifier() *IdleNotifier {
	return &IdleNotifier{
		subscribers: make(map[string]map[chan struct{}]struct{}),
	}
}

// Subscribe registers a channel to receive notifications for a mailbox.
func (n *IdleNotifier) Subscribe(mailboxID string, ch chan struct{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.subscribers[mailboxID] == nil {
		n.subscribers[mailboxID] = make(map[chan struct{}]struct{})
	}
	n.subscribers[mailboxID][ch] = struct{}{}
}

// Unsubscribe removes a channel from notifications for a mailbox.
func (n *IdleNotifier) Unsubscribe(mailboxID string, ch chan struct{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if subs, ok := n.subscribers[mailboxID]; ok {
		delete(subs, ch)
		if len(subs) == 0 {
			delete(n.subscribers, mailboxID)
		}
	}
}

// Notify sends a non-blocking signal to all subscribers of a mailbox.
func (n *IdleNotifier) Notify(mailboxID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for ch := range n.subscribers[mailboxID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}
