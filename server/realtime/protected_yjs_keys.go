package realtime

// ProtectedYjsRootKeys names the Y.Doc top-level shared types that
// must only be written server-side. The broker's
// UpdateContentValidator wired by the text package rejects any
// inbound update that mutates one of these keys.
//
// Adding a new protected key requires a coordinated change across:
//   - this list
//   - the text package's editor schema (must not include these keys
//     in any user-driven transaction)
//   - the docx translator's custom XML mapping
//
// The keys are intentionally short and stable; the docx custom XML
// part stores their contents as-is on flush.
var ProtectedYjsRootKeys = []string{
	"clientAuthors",
	"clientFirstSeen",
	"editEvents",
}
