package addressbook

import (
	"context"
	"errors"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func TestIsKnown(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	if _, ok := IsKnown(context.Background(), nil, "u", "a@b.c"); ok || Available() {
		t.Fatal("no lookup registered: want ok=false")
	}
	RegisterLookup(func(_ context.Context, _ core.App, user, email string) (bool, error) {
		if email == "boom@x.y" {
			return false, errors.New("boom")
		}
		return user == "u" && email == "a@b.c", nil
	})
	if known, ok := IsKnown(context.Background(), nil, "u", "A@B.C "); !ok || !known {
		t.Fatal("email must be normalised; want known")
	}
	if known, ok := IsKnown(context.Background(), nil, "u", "z@b.c"); !ok || known {
		t.Fatal("want not known")
	}
	if _, ok := IsKnown(context.Background(), nil, "u", "boom@x.y"); ok {
		t.Fatal("lookup error: want ok=false")
	}
}

func TestIsKnownEmptyEmailSkipsLookup(t *testing.T) {
	ResetForTest()
	t.Cleanup(ResetForTest)
	called := false
	RegisterLookup(func(_ context.Context, _ core.App, _, _ string) (bool, error) {
		called = true
		return true, nil
	})
	if known, ok := IsKnown(context.Background(), nil, "u", "  "); ok || known || called {
		t.Fatalf("empty email: want unknown without lookup, got known=%v ok=%v called=%v", known, ok, called)
	}
}
