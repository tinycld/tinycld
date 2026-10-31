package pbs

import (
	"slices"
	"sync"
	"testing"

	"tinycld.org/core/backup/repo"
)

// One process can compose many apps at the same time, and each composition
// calls Register. They must not race to add the kind twice.
func TestRegisterIsSafeFromManyGoroutines(t *testing.T) {
	repo.ResetForTesting()
	t.Cleanup(repo.ResetForTesting)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			<-start
			Register()
		})
	}
	close(start)
	wg.Wait()

	if kinds := repo.Kinds(); !slices.Equal(kinds, []string{Kind}) {
		t.Fatalf("Kinds() = %v, want [%s]", kinds, Kind)
	}
}
