package main

import "testing"

// TestEmbeddedAccessorsAreNilWithoutTag pins the default build: without the
// embedassets tag the accessors return nil, so main falls back to the
// existing disk-based paths and normal dev/Docker builds need no staged tree.
func TestEmbeddedAccessorsAreNilWithoutTag(t *testing.T) {
	if embeddedWebFS() != nil {
		t.Error("expected nil web FS in an untagged build")
	}
	if embeddedMigrationsFS() != nil {
		t.Error("expected nil migrations FS in an untagged build")
	}
	if embeddedHooksFS() != nil {
		t.Error("expected nil hooks FS in an untagged build")
	}
}
