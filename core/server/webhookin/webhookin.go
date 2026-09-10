// Package webhookin receives signed webhooks from external services and hands
// them to whichever package registered that source.
//
// TRANSPORT ONLY, and the line matters. This package verifies signatures,
// rejects replays, meters requests and logs; it writes NO domain row and
// knows no collection name. Interpretation — "this payload means link these
// two records" — belongs to the package that owns the schema, because that is
// where the access rules live. A generic "any webhook may write any
// collection" facility would route around those rules, which for a
// rule-first package are its entire authorization story.
//
// Packages register from their own Register(app), so core never names a
// package: the oauth.RegisterPackage / search.RegisterSources pattern.
package webhookin

import (
	"net/http"
	"sync"

	"github.com/pocketbase/pocketbase/core"
)

// Delivery is one verified inbound webhook, handed to a source's Handle.
//
// Body is the RAW bytes the signature was computed over. A handler that needs
// structured data decodes them itself; re-encoding a decoded payload would
// not round-trip byte-for-byte and would invalidate any further verification.
type Delivery struct {
	Source     string
	Event      string
	DeliveryID string
	Body       []byte
	Header     http.Header
}

// Source is what a package registers to receive one provider's webhooks.
type Source struct {
	// Secret returns the HMAC-SHA256 signing secret for this request. A func
	// rather than a string because the secret lives in the database (it is
	// per-deployment, and rotatable) and must be read at request time, not at
	// boot. Returning an error fails the request closed.
	Secret func(app core.App, r *http.Request) (string, error)

	// SignatureHeader names the header carrying the provider's signature,
	// e.g. "X-Hub-Signature-256". Defaults to X-TinyCld-Signature-256.
	SignatureHeader string

	// EventHeader names the header carrying the event type, e.g. an
	// "X-Provider-Event" style header. Optional; Delivery.Event is blank
	// without it.
	EventHeader string

	// DeliveryID extracts the provider's unique per-delivery id, the replay
	// dedupe key. A source returning "" opts out of dedupe.
	DeliveryID func(r *http.Request) string

	// Handle interprets one verified delivery. Returning an error yields a
	// 500, which asks a well-behaved provider to retry.
	Handle func(app core.App, d Delivery) error
}

var (
	registryMu sync.RWMutex
	sources    = map[string]Source{}
)

// Register installs a source under a URL-safe name, reached at
// POST /api/webhooks/{name}. Last registration wins, matching
// automation.RegisterAction: a rebuild re-registering is not an error.
func Register(name string, s Source) {
	registryMu.Lock()
	defer registryMu.Unlock()
	sources[name] = s
}

func lookup(name string) (Source, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	s, ok := sources[name]
	return s, ok
}

func registered() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(sources))
	for name := range sources {
		names = append(names, name)
	}
	return names
}
