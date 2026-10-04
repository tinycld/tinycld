package notify

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"tinycld.org/core/readonly"
	"tinycld.org/core/readonly/readonlytest"
)

func TestCommentMention_WaitsForReadOnlyToEnd(t *testing.T) {
	f := seedMentionFixture(t)
	mention := mkMention(t, f.app, f, "notepads_comments")
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	probe, waiting := readonlytest.WaitProbe(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		handleCommentMentionWhenWritable(probe, f.app, mention)
	}()
	select {
	case <-waiting:
	case <-done:
		t.Fatal("the mention notification was written without waiting for read-only mode to end")
	case <-time.After(5 * time.Second):
		t.Fatal("the mention notification never reached the read-only wait")
	}
	if n := findLatestNotification(t, f.app, f.mentionUser.Id); n != nil {
		t.Fatal("a mention notification was written while read-only")
	}

	readonly.Leave()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the mention notification was not written after read-only mode ended")
	}
	if n := findLatestNotification(t, f.app, f.mentionUser.Id); n == nil {
		t.Fatal("expected the mention notification after read-only mode ended")
	}
}

func TestCommentMention_DroppedAndLoggedWhenReadOnlyOutlastsTheWait(t *testing.T) {
	f := seedMentionFixture(t)
	mention := mkMention(t, f.app, f, "notepads_comments")
	logs := readonlytest.CaptureLogs(t)
	readonly.Enter()
	t.Cleanup(readonly.Leave)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	handleCommentMentionWhenWritable(ctx, f.app, mention)

	if n := findLatestNotification(t, f.app, f.mentionUser.Id); n != nil {
		t.Fatal("a mention notification was written while read-only")
	}
	r, ok := readonlytest.Find(logs(), mentionDroppedMsg)
	if !ok {
		t.Fatalf("no %q log for the dropped notification", mentionDroppedMsg)
	}
	if r.Level != slog.LevelWarn {
		t.Fatalf("want Warn for a dropped mention notification, got %v", r.Level)
	}
	if r.Attrs["mentionID"] != mention.Id {
		t.Fatalf("the log must name the mention, got %v", r.Attrs)
	}
}
