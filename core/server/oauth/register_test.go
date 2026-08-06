package oauth

import (
	"testing"

	"github.com/pocketbase/pocketbase"
)

func TestRegisterBindsWithoutPanicking(t *testing.T) {
	// Register only binds hooks; it must be safe to call on a fresh app and
	// must not require the collections to exist yet (migrations run later).
	app := pocketbase.New()
	Register(app)
}

// The middleware-ordering assertion (our priority must be strictly less than
// PocketBase's loadAuthToken priority) already exists as
// TestMiddlewarePriorityRunsBeforePocketBase in middleware_test.go — it
// reaches apis.DefaultLoadAuthTokenMiddlewarePriority directly rather than
// through defaultLoadAuthTokenPriorityForTest, so it is not duplicated here.
