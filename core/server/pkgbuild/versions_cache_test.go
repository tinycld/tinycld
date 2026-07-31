package pkgbuild

import (
	"fmt"
	"testing"
	"time"
)

// resetVersionCache clears the package-global cache so a test starts clean and
// leaves nothing behind for the next one.
func resetVersionCache(t *testing.T) {
	t.Helper()
	versionCacheMu.Lock()
	versionCache = map[string]versionCacheEntry{}
	versionCacheKeys = nil
	versionCacheMu.Unlock()
	t.Cleanup(func() {
		versionCacheMu.Lock()
		versionCache = map[string]versionCacheEntry{}
		versionCacheKeys = nil
		versionNow = time.Now
		versionCacheMu.Unlock()
	})
}

// H3: a tenant streaming distinct specs at /v1/versions must not grow the cache
// without bound. Past the cap the oldest-inserted entry is evicted.
func TestVersionCache_BoundedBySizeCap(t *testing.T) {
	resetVersionCache(t)
	versionNow = time.Now // fresh entries, so only the size cap can fire

	total := versionCacheMax + 200
	for i := 0; i < total; i++ {
		storeVersions(fmt.Sprintf("pkg%d@1.0.0", i), versionCacheEntry{source: SourceNpm})
	}

	versionCacheMu.Lock()
	size := len(versionCache)
	keys := len(versionCacheKeys)
	_, oldestPresent := versionCache["pkg0@1.0.0"]
	_, newestPresent := versionCache[fmt.Sprintf("pkg%d@1.0.0", total-1)]
	versionCacheMu.Unlock()

	if size > versionCacheMax {
		t.Fatalf("cache grew past the cap: %d entries (max %d)", size, versionCacheMax)
	}
	if keys != size {
		t.Fatalf("insertion-order list (%d) out of sync with map (%d)", keys, size)
	}
	if oldestPresent {
		t.Fatal("oldest-inserted entry was not evicted under the cap")
	}
	if !newestPresent {
		t.Fatal("newest entry should still be cached")
	}
}

// Expired entries are swept out opportunistically on the next store, keeping the
// cache bounded by TTL under a slow churn well before the size cap fires.
func TestVersionCache_EvictsExpired(t *testing.T) {
	resetVersionCache(t)

	base := time.Now()
	versionNow = func() time.Time { return base }
	storeVersions("stale@1.0.0", versionCacheEntry{source: SourceNpm})

	// Advance past the TTL and store an unrelated entry; the stale one is swept.
	versionNow = func() time.Time { return base.Add(versionCacheTTL + time.Second) }
	storeVersions("fresh@1.0.0", versionCacheEntry{source: SourceNpm})

	versionCacheMu.Lock()
	_, stalePresent := versionCache["stale@1.0.0"]
	_, freshPresent := versionCache["fresh@1.0.0"]
	keys := len(versionCacheKeys)
	size := len(versionCache)
	versionCacheMu.Unlock()

	if stalePresent {
		t.Fatal("expired entry survived the opportunistic sweep")
	}
	if !freshPresent {
		t.Fatal("fresh entry missing after store")
	}
	if keys != size {
		t.Fatalf("insertion-order list (%d) out of sync with map (%d)", keys, size)
	}
}
