package syscfg

import "testing"

// fakeProvider is a supervisor-supplied provider: values from memory, plus the
// namespaces it owns.
type fakeProvider struct {
	values   map[string]string
	prefixes []string
}

func (f fakeProvider) Get(key string) string     { return f.values[key] }
func (f fakeProvider) ManagedPrefixes() []string { return f.prefixes }

// restore puts the package global back so tests cannot leak into each other.
func restore(t *testing.T) {
	t.Helper()
	mu.RLock()
	prev, prevClaimed := current, claimed
	mu.RUnlock()
	t.Cleanup(func() {
		mu.Lock()
		current, claimed = prev, prevClaimed
		mu.Unlock()
	})
}

func TestDefaultProviderManagesNothing(t *testing.T) {
	restore(t)
	SetProvider(unmanaged{})

	if got := Get("sentry.dsn"); got != "" {
		t.Fatalf("Get on the zero provider = %q, want empty", got)
	}
	if prefixes := ManagedPrefixes(); len(prefixes) != 0 {
		t.Fatalf("ManagedPrefixes = %v, want none", prefixes)
	}
	// The standalone guarantee: with nothing managed, every key stays editable.
	for _, key := range []string{"sentry.dsn", "vapid.private_key", "mail.provider"} {
		if IsManaged(key) {
			t.Errorf("IsManaged(%q) = true on a standalone deployment, want false", key)
		}
	}
}

func TestSetResolverSuppliesValuesButManagesNothing(t *testing.T) {
	restore(t)
	SetResolver(func(key string) string {
		return map[string]string{"sentry.dsn": "https://example.invalid/1"}[key]
	})

	if got := Get("sentry.dsn"); got != "https://example.invalid/1" {
		t.Fatalf("Get = %q, want the resolver's value", got)
	}
	// A standalone deployment reads its own settings AND administers them.
	if IsManaged("sentry.dsn") {
		t.Error("IsManaged = true for a plain resolver; standalone manages nothing")
	}
}

func TestProviderSuppliesValuesAndManagedNamespaces(t *testing.T) {
	restore(t)
	SetProvider(fakeProvider{
		values:   map[string]string{"mail.provider": "postmark", "vapid.private_key": "secret"},
		prefixes: []string{"sentry.", "vapid.", "mail."},
	})

	if got := Get("mail.provider"); got != "postmark" {
		t.Fatalf("Get(mail.provider) = %q, want postmark", got)
	}

	for _, key := range []string{"mail.provider", "mail.smtp_imap_password", "sentry.dsn", "vapid.private_key"} {
		if !IsManaged(key) {
			t.Errorf("IsManaged(%q) = false, want true", key)
		}
	}
	// A key outside every managed namespace stays the deployment's own.
	for _, key := range []string{"storage_limit_bytes", "mailbox.other", "sentryish"} {
		if IsManaged(key) {
			t.Errorf("IsManaged(%q) = true, want false", key)
		}
	}
}

// A nil provider must not blank the resolver: that would silently disable mail,
// push and error reporting rather than surfacing the caller's bug.
func TestSetProviderIgnoresNil(t *testing.T) {
	restore(t)
	SetProvider(fakeProvider{values: map[string]string{"mail.provider": "smtp"}})
	SetProvider(nil)

	if got := Get("mail.provider"); got != "smtp" {
		t.Fatalf("Get after SetProvider(nil) = %q, want the previous provider's value", got)
	}
}

func TestSetResolverIgnoresNil(t *testing.T) {
	restore(t)
	SetResolver(func(string) string { return "kept" })
	SetResolver(nil)

	if got := Get("anything"); got != "kept" {
		t.Fatalf("Get after SetResolver(nil) = %q, want the previous resolver's value", got)
	}
}

// An empty prefix would match every key and lock a deployment out of its own
// settings. It must be ignored rather than treated as "everything".
func TestEmptyPrefixDoesNotManageEverything(t *testing.T) {
	restore(t)
	SetProvider(fakeProvider{prefixes: []string{"", "mail."}})

	if IsManaged("sentry.dsn") {
		t.Error("an empty prefix matched an unrelated key")
	}
	if !IsManaged("mail.provider") {
		t.Error("a real prefix alongside an empty one stopped matching")
	}
}

// emptySupervisor is a supervising composition whose config failed to load: it
// installed a provider, but has no values and manages nothing YET.
type emptySupervisor struct{}

func (emptySupervisor) Get(string) string         { return "" }
func (emptySupervisor) ManagedPrefixes() []string { return nil }

// The claim must not be inferred from "does it manage anything". A supervisor
// whose syscfg.json was truncated or unreadable still OWNS these settings, and
// the correct degraded state is "mail, push and error reporting are off" — never
// "the deployment supplies its own", which is the fallback the supervisor exists
// to prevent.
//
// Core's own wiring runs after a supervisor's, so without an explicit claim it
// would point these reads straight back at the deployment's collection.
func TestSupervisorWithNoPrefixesStillOwnsTheSeam(t *testing.T) {
	restore(t)
	SetProvider(emptySupervisor{})

	if !IsClaimed() {
		t.Fatal("a supervisor's provider did not claim the seam")
	}

	// What coreserver.RegisterSystemConfig does moments later.
	SetResolver(func(string) string { return "the deployment's own value" })

	if got := Get("mail.provider"); got != "" {
		t.Fatalf("core reclaimed the seam: Get returned %q — the deployment is now "+
			"supplying settings its operator owns", got)
	}
	if IsManaged("mail.provider") {
		t.Error("nothing is managed yet, so IsManaged should be false; only the CLAIM persists")
	}
}

// An unclaimed deployment must still be able to point the seam at its own
// settings — the standalone path, which is every self-hosted install.
func TestUnclaimedSeamAcceptsTheDeploymentsOwnResolver(t *testing.T) {
	restore(t)
	mu.Lock()
	current, claimed = unmanaged{}, false
	mu.Unlock()

	SetResolver(func(key string) string {
		return map[string]string{"mail.provider": "smtp"}[key]
	})
	if got := Get("mail.provider"); got != "smtp" {
		t.Fatalf("a standalone deployment could not supply its own settings: %q", got)
	}
}
