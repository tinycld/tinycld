package installjob

import (
	"sync"
	"testing"
)

// The property the whole package exists for: two callers racing to start an
// operation, and exactly one winning. Before this package the state was a
// package-level var in one composition's package, so a second composition got
// its own empty copy and BOTH would have believed they won.
//
// RUN THIS UNDER -race. Verified: deleting Claim's lock still passes a plain
// `go test` (the window is too narrow to lose reliably) and fails immediately
// under -race. A green run without the flag proves nothing about the interlock.
func TestClaim_OnlyOneWinnerUnderRace(t *testing.T) {
	t.Cleanup(func() { Release(Current()) })

	const racers = 32
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		won  int
		lost int
	)
	start := make(chan struct{})

	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, ok := Claim(New("install", "acme", "@tinycld/acme"))
			mu.Lock()
			defer mu.Unlock()
			if ok {
				won++
			} else {
				lost++
			}
		}()
	}
	close(start)
	wg.Wait()

	if won != 1 {
		t.Fatalf("claims that won = %d, want exactly 1", won)
	}
	if lost != racers-1 {
		t.Fatalf("claims that lost = %d, want %d", lost, racers-1)
	}
}

func TestClaim_ReportsTheBusyJobAndReleases(t *testing.T) {
	first := New("install", "acme", "@tinycld/acme")
	if _, ok := Claim(first); !ok {
		t.Fatal("first claim must win against an idle interlock")
	}
	if !Running() {
		t.Fatal("Running must be true while a job holds the interlock")
	}

	busy, ok := Claim(New("uninstall", "globex", ""))
	if ok {
		t.Fatal("second claim must lose")
	}
	if busy != first {
		t.Fatalf("busy job = %+v, want the first one", busy)
	}
	if info := busy.Info(); info["action"] != "install" || info["slug"] != "acme" {
		t.Fatalf("Info = %v, want the running job's action and slug", info)
	}

	Release(first)
	if Running() {
		t.Fatal("Running must be false once the holder releases")
	}
	if _, ok := Claim(New("install", "third", "")); !ok {
		t.Fatal("a claim after release must win")
	}
	Release(Current())
}

// Release compares before clearing, so a late unwind cannot evict whoever holds
// the interlock now.
func TestRelease_DoesNotEvictADifferentHolder(t *testing.T) {
	stale := New("install", "stale", "")
	holder := New("install", "holder", "")
	if _, ok := Claim(holder); !ok {
		t.Fatal("claim must win")
	}
	t.Cleanup(func() { Release(holder) })

	Release(stale)

	if Current() != holder {
		t.Fatal("releasing a job that does not hold the interlock must not clear it")
	}
}

func TestRecordProgress_UpdatesStateAndFansOut(t *testing.T) {
	job := New("install", "acme", "")
	ch := job.Subscribe(4)

	line := job.RecordProgress("build", 40, "compiling")
	if line != "[40%] build: compiling" {
		t.Fatalf("recorded line = %q", line)
	}
	if job.Progress != 40 || job.Step != "build" {
		t.Fatalf("job = %d%% %q, want 40%% build", job.Progress, job.Step)
	}

	evt := <-ch
	data, ok := evt.Data.(ProgressData)
	if evt.Event != "progress" || !ok || data.Message != "compiling" {
		t.Fatalf("event = %+v", evt)
	}
	if got := job.Snapshot(); len(got) != 1 || got[0] != line {
		t.Fatalf("snapshot = %v", got)
	}
}

// A listener that stopped reading must not stall the install.
func TestRecordProgress_DropsRatherThanBlocks(t *testing.T) {
	job := New("install", "acme", "")
	job.Subscribe(1) // never drained

	done := make(chan struct{})
	go func() {
		for i := 0; i < 50; i++ {
			job.RecordProgress("build", i, "step")
		}
		close(done)
	}()

	<-done // a blocking fanout would hang here rather than finish
	if len(job.Snapshot()) != 50 {
		t.Fatalf("recorded %d lines, want 50", len(job.Snapshot()))
	}
}

func TestUnsubscribe_StopsDelivery(t *testing.T) {
	job := New("install", "acme", "")
	ch := job.Subscribe(4)
	job.Unsubscribe(ch)

	job.RecordProgress("build", 10, "after unsubscribe")

	select {
	case evt := <-ch:
		t.Fatalf("unsubscribed listener still received %+v", evt)
	default:
	}
}
