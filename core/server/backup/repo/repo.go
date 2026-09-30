// Package repo is where a backup goes. The engine produces a snapshot; a
// Repository stores it and reads it back. The tar/age archive is one
// Repository, and a deduplicating store (PBS) is another.
package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"tinycld.org/core/backup/format"
	"tinycld.org/core/backup/snapshot"
)

type Ref string

type PutResult struct {
	Ref           Ref
	Bytes         int64
	UploadedBytes int64
	Sha256        string
}

type SnapshotInfo struct {
	Ref     Ref       `json:"ref"`
	Created time.Time `json:"created"`
	Bytes   int64     `json:"bytes"`
}

type Repository interface {
	Kind() string
	Put(ctx context.Context, s *snapshot.Snapshot, progress func(sent int64)) (PutResult, error)
	Manifest(ctx context.Context, ref Ref) (format.Manifest, error)
	Fetch(ctx context.Context, ref Ref, dir string) error
	List(ctx context.Context) ([]SnapshotInfo, error)
}

type Opener func(cfg json.RawMessage) (Repository, error)

var (
	ErrNotSupported = errors.New("backup: this repository cannot do that")
	ErrUnknownKind  = errors.New("backup: unknown repository kind")
)

var (
	mu      sync.RWMutex
	openers = map[string]Opener{}
)

func Register(kind string, open Opener) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := openers[kind]; dup {
		panic("backup: repository kind registered twice: " + kind)
	}
	openers[kind] = open
}

func Open(kind string, cfg json.RawMessage) (Repository, error) {
	mu.RLock()
	open, ok := openers[kind]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownKind, kind)
	}
	return open(cfg)
}

func Kinds() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(openers))
	for k := range openers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func ResetForTesting() {
	mu.Lock()
	openers = map[string]Opener{}
	mu.Unlock()
}
