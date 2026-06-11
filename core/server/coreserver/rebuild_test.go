package coreserver

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRebuildManifest_RoundTrip(t *testing.T) {
	m := RebuildManifest{
		BuildID: "build-1234",
		Members: []MemberSpec{
			{Slug: "tinycld", Version: "1.2.0", Spec: "git+https://github.com/tinycld/tinycld#v1.2.0"},
			{Slug: "mail", Version: "0.3.1", Spec: "@tinycld/mail@0.3.1"},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got RebuildManifest
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.BuildID != "build-1234" || len(got.Members) != 2 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
	if got.Members[1].Slug != "mail" || got.Members[1].Spec != "@tinycld/mail@0.3.1" {
		t.Fatalf("member mismatch: %+v", got.Members[1])
	}
}

func TestRebuildManifest_MemberBySlug(t *testing.T) {
	m := RebuildManifest{Members: []MemberSpec{{Slug: "mail"}, {Slug: "calc"}}}
	if ms, ok := m.MemberBySlug("calc"); !ok || ms.Slug != "calc" {
		t.Fatalf("MemberBySlug(calc) failed: %+v ok=%v", ms, ok)
	}
	if _, ok := m.MemberBySlug("absent"); ok {
		t.Fatal("MemberBySlug(absent) should be !ok")
	}
}

func TestDesiredSet_Install(t *testing.T) {
	current := []MemberSpec{
		{Slug: "tinycld", Version: "1.0.0", Spec: "git+https://x/tinycld#v1.0.0"},
	}
	delta := setDelta{op: "install", slug: "mail", version: "0.3.1", spec: "@tinycld/mail@0.3.1"}
	m := desiredSet("build-1", current, delta)
	if _, ok := m.MemberBySlug("mail"); !ok {
		t.Fatal("mail not added")
	}
	if len(m.Members) != 2 {
		t.Fatalf("want 2 members, got %d", len(m.Members))
	}
}

func TestDesiredSet_Uninstall(t *testing.T) {
	current := []MemberSpec{{Slug: "tinycld"}, {Slug: "mail"}}
	m := desiredSet("build-2", current, setDelta{op: "uninstall", slug: "mail"})
	if _, ok := m.MemberBySlug("mail"); ok {
		t.Fatal("mail should be removed")
	}
	if _, ok := m.MemberBySlug("tinycld"); !ok {
		t.Fatal("tinycld must remain")
	}
}

func TestDesiredSet_Upgrade_OverridesSpec(t *testing.T) {
	current := []MemberSpec{
		{Slug: "tinycld"},
		{Slug: "mail", Version: "0.3.1", Spec: "@tinycld/mail@0.3.1"},
	}
	m := desiredSet("build-3", current, setDelta{op: "version", slug: "mail", version: "0.4.0", spec: "@tinycld/mail@0.4.0"})
	ms, _ := m.MemberBySlug("mail")
	if ms.Version != "0.4.0" || ms.Spec != "@tinycld/mail@0.4.0" {
		t.Fatalf("mail not upgraded: %+v", ms)
	}
}

func TestRebuild_HappyPath_Sequence(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var seq []string
	deps := rebuildDeps{
		assemble: func(m RebuildManifest, dir string) error {
			seq = append(seq, "assemble")
			return os.MkdirAll(filepath.Join(dir, "tinycld", "server", "pb_migrations"), 0o755)
		},
		pipeline:       func(j *installJob, dir string) error { seq = append(seq, "pipeline"); return nil },
		backupDB:       func() error { seq = append(seq, "backup"); return nil },
		syncMig:        func(buildDir string) (SyncResult, error) { seq = append(seq, "sync"); return SyncResult{}, nil },
		activate:       func(id string) error { seq = append(seq, "activate"); return nil },
		commitRegistry: func() error { seq = append(seq, "commit"); return nil },
		prune:          func(keep int) error { seq = append(seq, "prune"); return nil },
		restart:        func() { seq = append(seq, "restart") },
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	m := RebuildManifest{BuildID: "build-1", Members: []MemberSpec{{Slug: "tinycld", Spec: "x"}}}
	if err := rebuildWith(job, m, deps); err != nil {
		t.Fatal(err)
	}
	want := "assemble,pipeline,backup,sync,activate,commit,prune,restart"
	if got := strings.Join(seq, ","); got != want {
		t.Fatalf("sequence = %s, want %s", got, want)
	}
}

func TestRebuild_PipelineFailure_NoActivateNoRestore(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var restored, activated bool
	deps := rebuildDeps{
		assemble:  func(m RebuildManifest, dir string) error { return nil },
		pipeline:  func(j *installJob, dir string) error { return fmt.Errorf("build broke") },
		backupDB:  func() error { return nil },
		restoreDB: func() error { restored = true; return nil },
		syncMig:   func(buildDir string) (SyncResult, error) { return SyncResult{}, nil },
		activate:  func(id string) error { activated = true; return nil },
		prune:     func(keep int) error { return nil },
		restart:   func() {},
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := rebuildWith(job, RebuildManifest{BuildID: "build-2"}, deps); err == nil {
		t.Fatal("expected error")
	}
	if activated {
		t.Fatal("must NOT activate after pipeline failure")
	}
	// Failure precedes the backup, so restore should not run.
	if restored {
		t.Fatal("restore should not run when failure precedes backup")
	}
}

func TestRebuild_MigrateFailure_RestoresAndNoActivate(t *testing.T) {
	state := t.TempDir()
	t.Setenv("TINYCLD_STATE_DIR", state)
	var restored, activated bool
	deps := rebuildDeps{
		assemble:  func(m RebuildManifest, dir string) error { return nil },
		pipeline:  func(j *installJob, dir string) error { return nil },
		backupDB:  func() error { return nil },
		restoreDB: func() error { restored = true; return nil },
		syncMig:   func(buildDir string) (SyncResult, error) { return SyncResult{}, fmt.Errorf("down broke") },
		activate:  func(id string) error { activated = true; return nil },
		prune:     func(keep int) error { return nil },
		restart:   func() {},
	}
	job := &installJob{ID: "j", Done: make(chan struct{})}
	if err := rebuildWith(job, RebuildManifest{BuildID: "build-3"}, deps); err == nil {
		t.Fatal("expected error")
	}
	if !restored {
		t.Fatal("DB should be restored after a migration failure")
	}
	if activated {
		t.Fatal("must NOT activate after migration failure")
	}
}
