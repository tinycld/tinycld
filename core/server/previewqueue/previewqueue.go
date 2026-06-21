// Package previewqueue is a tiny in-process handoff between the synchronous
// flush path (which computes the text region, hashes, and a PageModel while it
// still holds the live document state) and the asynchronous PocketBase hook
// that actually renders and persists the preview.
//
// Why this exists (and is its own package): the flush path can't render the
// JPEG inline without blocking the request, and the async hook fires later
// with only the item ID in hand — by then the live state may be gone. The
// flush path stashes the precomputed Payload here keyed by item ID; the hook
// Takes it when it runs.
//
// Self-clearing rationale: Take deletes the entry as it reads it, so a stashed
// payload is consumed exactly once and never accumulates. A payload that is
// stashed but whose hook never fires is overwritten by the next Stash for the
// same item, so the map stays bounded without a separate eviction sweep.
package previewqueue

import (
	"sync"

	"tinycld.org/core/thumbnails/textpreview"
)

// Payload is the precomputed preview data handed from the flush path to the
// async render hook.
type Payload struct {
	Plaintext  string
	RegionHash string
	IndexHash  string
	Page       textpreview.PageModel
}

var (
	mu    sync.Mutex
	items = map[string]Payload{}
)

// Stash records p for itemID, replacing any prior unread payload.
func Stash(itemID string, p Payload) {
	mu.Lock()
	defer mu.Unlock()
	items[itemID] = p
}

// Take returns the payload stashed for itemID and removes it, so each stashed
// payload is consumed exactly once. The bool reports whether one was present.
func Take(itemID string) (Payload, bool) {
	mu.Lock()
	defer mu.Unlock()
	p, ok := items[itemID]
	delete(items, itemID)
	return p, ok
}

// ResetForTest clears the queue. Exposed only for tests — production callers
// never invoke this.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	items = map[string]Payload{}
}
