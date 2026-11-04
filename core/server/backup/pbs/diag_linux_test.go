//go:build linux

package pbs

import (
	"context"
	"testing"

	"github.com/osshield/gopbs/pbstest"

	"tinycld.org/core/backup/repo/repotest"
)

// On Linux each connection's record carries the kernel's TCP_INFO at close:
// RTT and retransmits are what tell a slow or lossy link from a refusal.
func TestDiagnosticsRecordTheSocketStateOnLinux(t *testing.T) {
	logs := captureLogs(t)
	srv := pbstest.NewServer(t)
	r, _ := Open(configFor(t, srv, ""))
	if _, err := r.Put(context.Background(), repotest.Fixture(t), nil); err != nil {
		t.Fatal(err)
	}
	rec := onlyRecord(t, logs.records(t, "pbs: operation finished"), "backup")
	conns := listOf(t, rec, "conns")
	if len(conns) == 0 {
		t.Fatalf("no connection recorded: %v", rec)
	}
	info, ok := conns[0]["tcp_at_close"].(map[string]any)
	if !ok || info["rtt_us"] == nil || info["total_retrans"] == nil {
		t.Fatalf("tcp_at_close %v", conns[0]["tcp_at_close"])
	}
}
