package repo

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/snapshot"
)

type stub struct{}

func (stub) Kind() string { return "stub" }
func (stub) Put(context.Context, *snapshot.Snapshot, func(int64)) (PutResult, error) {
	return PutResult{}, nil
}
func (stub) Manifest(context.Context, Ref) (format.Manifest, error) { return format.Manifest{}, nil }
func (stub) Fetch(context.Context, Ref, string) error               { return nil }
func (stub) List(context.Context) ([]SnapshotInfo, error)           { return nil, ErrNotSupported }

func TestRegisterAndOpen(t *testing.T) {
	t.Cleanup(ResetForTesting)
	Register("stub", func(json.RawMessage) (Repository, error) { return stub{}, nil })
	r, err := Open("stub", nil)
	if err != nil || r.Kind() != "stub" {
		t.Fatalf("open: %v %v", r, err)
	}
	if _, err := Open("nope", nil); !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("err = %v", err)
	}
	if got := Kinds(); len(got) != 1 || got[0] != "stub" {
		t.Fatalf("kinds = %v", got)
	}
}

func TestRegisterTwicePanics(t *testing.T) {
	t.Cleanup(ResetForTesting)
	Register("stub", func(json.RawMessage) (Repository, error) { return stub{}, nil })
	defer func() {
		if recover() == nil {
			t.Fatal("no panic on duplicate kind")
		}
	}()
	Register("stub", func(json.RawMessage) (Repository, error) { return stub{}, nil })
}
