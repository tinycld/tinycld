package coreserver

import (
	"github.com/pocketbase/pocketbase/core"

	"tinycld.org/core/mailproto"
)

// The single-Register package contract: a feature package exports ONE
// composition entry point, `Register(app *pocketbase.PocketBase)`, and it runs
// unchanged wherever the app is composed. The rare package that must behave
// differently when something else owns its runtime wiring DETECTS that from
// the app instead of exporting a second function — mail is the one real case:
// self-hosted it binds its own mail ports, embedded it serves the sockets the
// supervisor hands down. Packages that bind no listener need no detection at
// all, which is the vast majority and the point of the contract.
//
// Nothing here knows what the supervisor is. Core exposes only the fact that
// one supplied this process's wiring, plus the wiring itself.

// MailSockets are pre-bound mail sockets supplied by a supervisor that owns the
// public ports, as lazy ListenFuncs. The supervisor terminates TLS and forwards
// plaintext over these. A nil entry means it manages no socket for that service
// and the process must not start it.
type MailSockets struct {
	IMAP       mailproto.ListenFunc
	Submission mailproto.ListenFunc
	InboundMX  mailproto.ListenFunc
}

// EmbeddedContext is set when a supervising process composes this app and
// supplies its runtime wiring instead of the process discovering it itself.
// Present (GetEmbeddedContext ok) only in that case; a self-hosted deployment
// has none. It carries only what a core-owned package must consult: the
// instance's identity and the listeners it must not bind for itself.
type EmbeddedContext struct {
	// InstanceID identifies this process to its supervisor. Identification and
	// logging only — nothing branches on the value.
	InstanceID string
	// Mail are the supervisor-managed mail listeners (zero value = this process
	// runs no mail listeners).
	Mail MailSockets
}

// embeddedContextKey namespaces the store entry; the app store is shared with
// PocketBase internals and package code.
const embeddedContextKey = "tinycld.embeddedContext"

// SetEmbeddedContext stamps the context. A supervising composition calls this
// exactly once, BEFORE any feature package registers, so every Register can
// see it.
func SetEmbeddedContext(app core.App, ec EmbeddedContext) {
	app.Store().Set(embeddedContextKey, ec)
}

// GetEmbeddedContext reports the embedded context, and whether this app was
// composed by a supervisor at all. Feature packages use the second return to
// skip behavior they must not have when they do not own their own wiring —
// binding a port is the canonical example.
func GetEmbeddedContext(app core.App) (EmbeddedContext, bool) {
	v := app.Store().Get(embeddedContextKey)
	ec, ok := v.(EmbeddedContext)
	return ec, ok
}
