package yjsdoc

import (
	"fmt"
	"sort"

	ycrdt "github.com/skyterra/y-crdt"
)

// RootKeysOfUpdate reports which top-level keys an update writes to, without
// touching any live document.
//
// It applies the update to a fresh, never-touched Doc and reads the resulting
// Share map. y-crdt populates Share lazily — a root appears either when client
// code calls doc.Get* (which this never does) or when an inbound update
// integrates an item whose parent is that root. So the keys present afterwards
// are exactly the roots the update wrote. This is a structural check against
// the decoded update, not substring matching, so user content that merely
// mentions a key name cannot trigger a false positive.
//
// The intended use is an UpdateContentValidator that admits only the roots a
// room is supposed to own — boards accepts `card:<id>` and nothing else, which
// keeps a crafted client from parking data under an arbitrary key in a document
// shared by everyone on the board.
//
// Two documented limits, both inherited from how Yjs encodes updates:
//
//   - A pure-delete payload references items by (clientID, clock) and never
//     names its root, so it writes nothing to Share and reports no keys. A
//     caller cannot use this to authorize deletions.
//   - Malformed bytes decode to nothing rather than erroring, so garbage
//     reports no keys. That is harmless: the broker's own ApplyUpdate is an
//     equivalent no-op on the same input.
func RootKeysOfUpdate(roomID string, update []byte) ([]string, error) {
	probe := ycrdt.NewDoc(roomID+"-probe", false, nil, nil, false)
	// The probe needs the same observer patch as a production document:
	// without it a legitimate write that fans out observers panics during
	// decode, and the recover below would turn that into a spurious error.
	InstallPatcher(probe)

	if err := applyForProbe(probe, update); err != nil {
		return nil, fmt.Errorf("yjsdoc: could not decode update for room %s: %w", roomID, err)
	}
	if probe.Share == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(probe.Share))
	for key := range probe.Share {
		keys = append(keys, key)
	}
	// Sorted so a caller's error messages and tests are deterministic; Go map
	// iteration order is not.
	sort.Strings(keys)
	return keys, nil
}

// applyForProbe runs ApplyUpdate against the probe, converting a panic into an
// error so hostile input cannot take down the caller's goroutine.
func applyForProbe(probe *Doc, update []byte) (returnedErr error) {
	defer func() {
		if rec := recover(); rec != nil {
			returnedErr = fmt.Errorf("panic decoding update: %v", rec)
		}
	}()
	ycrdt.ApplyUpdate(probe, update, nil)
	return nil
}
