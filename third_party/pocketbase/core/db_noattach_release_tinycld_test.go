//go:build !no_default_driver

package core

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"weak"
)

// A process that opens and closes many apps opens new pools for its whole
// life. Recording a pool as restricted must not keep the pool alive after it
// is closed and dropped, and its record must go with it.
func TestNoAttachDBConnect_ClosedPoolIsNotKeptAlive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	var pools []weak.Pointer[sql.DB]
	for range 20 {
		db, err := NoAttachDBConnect(path)
		if err != nil {
			t.Fatal(err)
		}
		pools = append(pools, weak.Make(db.DB()))
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		runtime.GC()
		alive, recorded := 0, 0
		for _, p := range pools {
			if p.Value() != nil {
				alive++
			}
			if _, ok := restrictedPools.Load(p); ok {
				recorded++
			}
		}
		if alive == 0 && recorded == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("of %d closed pools, %d are still alive and %d still recorded", len(pools), alive, recorded)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
