package webhookin

import (
	"net/http"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestRegister_ThenLookup(t *testing.T) {
	resetRegistry(t)

	Register("acme", Source{
		Secret:     func(core.App, *http.Request) (string, error) { return "s3cret", nil },
		DeliveryID: func(r *http.Request) string { return r.Header.Get("X-Acme-Delivery") },
		Handle:     func(core.App, Delivery) error { return nil },
	})

	got, ok := lookup("acme")
	if !ok {
		t.Fatal("lookup(acme) not found after Register")
	}
	if got.Handle == nil {
		t.Error("registered source lost its Handle")
	}
}

func TestLookup_UnknownSource(t *testing.T) {
	resetRegistry(t)
	if _, ok := lookup("nope"); ok {
		t.Error("lookup of an unregistered source succeeded")
	}
}

func TestRegister_LastWins(t *testing.T) {
	resetRegistry(t)
	Register("acme", Source{Handle: func(core.App, Delivery) error { return nil }})
	Register("acme", Source{Handle: func(core.App, Delivery) error { return errSentinel }})

	got, _ := lookup("acme")
	if err := got.Handle(nil, Delivery{}); err != errSentinel {
		t.Error("a re-registration did not replace the earlier source")
	}
}

func TestRegistered_ListsNames(t *testing.T) {
	resetRegistry(t)
	Register("acme", Source{})
	Register("widgets", Source{})

	names := registered()
	if len(names) != 2 {
		t.Fatalf("registered() = %v, want 2 entries", names)
	}
}
