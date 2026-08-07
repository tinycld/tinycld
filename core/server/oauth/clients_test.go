package oauth

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// seedConfidentialClient registers a confidential client holding the hash of
// secret. Confidential clients had no test coverage before the timing
// mitigation went in — the CLI, the only client that exists today, is public
// and therefore skips the secret path entirely.
func seedConfidentialClient(t *testing.T, app core.App, clientID, secret string) *core.Record {
	t.Helper()

	clients, err := app.FindCollectionByNameOrId(clientsCollection)
	if err != nil {
		t.Fatalf("find clients: %v", err)
	}
	c := core.NewRecord(clients)
	c.Set("client_id", clientID)
	c.Set("name", "Confidential Test Client")
	c.Set("type", "confidential")
	c.Set("scopes", ScopeProfile)
	if secret != "" {
		c.Set("client_secret_hash", hashSecret(secret))
	}
	if err := app.Save(c); err != nil {
		t.Fatalf("save confidential client: %v", err)
	}
	return c
}

func TestVerifyClientSecret(t *testing.T) {
	app := newSchemaApp(t)
	client := seedConfidentialClient(t, app, "zapier", "correct-horse-battery")

	if !VerifyClientSecret(client, "correct-horse-battery") {
		t.Fatal("the registered secret must verify")
	}
	if VerifyClientSecret(client, "wrong-secret") {
		t.Fatal("a wrong secret must not verify")
	}
	if VerifyClientSecret(client, "") {
		t.Fatal("an empty secret must not verify against a real hash")
	}
}

// TestVerifyClientSecretPublicClientNeedsNoSecret pins the asymmetry that
// makes the CLI work: a public client cannot keep a secret, so PKCE binds its
// exchange instead and there is nothing here to check.
func TestVerifyClientSecretPublicClientNeedsNoSecret(t *testing.T) {
	app := newSchemaApp(t)
	seedUserAndClient(t, app)

	client, err := FindClientByClientID(app, "tinycld-cli")
	if err != nil {
		t.Fatalf("FindClientByClientID: %v", err)
	}
	if !VerifyClientSecret(client, "") {
		t.Fatal("a public client must verify without a secret")
	}
}

// TestVerifyClientSecretRejectsConfidentialClientWithNoHash covers the
// misconfiguration case: a client registered as confidential but never given a
// secret must not become a client that authenticates with ANY secret. This is
// also the branch the dummy-hash compare sits on, so it exercises that path.
func TestVerifyClientSecretRejectsConfidentialClientWithNoHash(t *testing.T) {
	app := newSchemaApp(t)
	client := seedConfidentialClient(t, app, "half-registered", "")

	for _, attempt := range []string{"", "anything", "guess"} {
		if VerifyClientSecret(client, attempt) {
			t.Fatalf("a confidential client with no stored hash must reject %q", attempt)
		}
	}
}

// TestCompareAgainstDummyHashRejectsEverything guards the dummy value itself.
// The mitigation is only sound if nothing a caller can send matches it — a
// dummy that some input could equal would turn the miss path into an accept.
// Cheap to assert, and it would catch the dummy ever being replaced with a
// constant derived from something reachable.
func TestCompareAgainstDummyHashRejectsEverything(t *testing.T) {
	if dummyClientSecretHash == "" {
		t.Fatal("dummyClientSecretHash must not be empty — an empty hash would compare equal to a missing one")
	}
	// The dummy must not collide with the hash of a plausible secret, most
	// importantly the empty string a caller sends by omitting the field.
	for _, attempt := range []string{"", "password", "oauth-dummy-client-secret "} {
		if hashSecret(attempt) == dummyClientSecretHash {
			t.Fatalf("a caller-suppliable value %q hashes to the dummy", attempt)
		}
	}
}
