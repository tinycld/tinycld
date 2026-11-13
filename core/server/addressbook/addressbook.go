// Package addressbook answers "is this address in the user's address book"
// for any package, without that package importing the one that keeps the
// address book. A deployment with no address-book package registers nothing,
// and callers must then treat the answer as unknown, not as "no".
package addressbook

import (
	"context"
	"strings"
	"sync"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/logging"
)

type Lookup func(ctx context.Context, app core.App, userID, email string) (bool, error)

var (
	mu     sync.RWMutex
	lookup Lookup
	log    = logging.ForPackage("addressbook")
)

func RegisterLookup(l Lookup) {
	mu.Lock()
	defer mu.Unlock()
	lookup = l
}

func Available() bool {
	mu.RLock()
	defer mu.RUnlock()
	return lookup != nil
}

func IsKnown(ctx context.Context, app core.App, userID, email string) (known bool, ok bool) {
	mu.RLock()
	l := lookup
	mu.RUnlock()
	if l == nil {
		return false, false
	}
	known, err := l(ctx, app, userID, strings.ToLower(strings.TrimSpace(email)))
	if err != nil {
		log.WarnContext(ctx, "address book lookup failed", "err", err)
		return false, false
	}
	return known, true
}

func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	lookup = nil
}
