package automation

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"tinycld.org/core/mailer"
	"tinycld.org/core/notify"
)

// "tickets" and "things" are fictional packages: core names none of its own.

// An action still running at terminate is cancelled, and the terminate waits
// for it to return before the database closes. Without the cancel the action
// never learns of the terminate: the drain gives up after shutdownDrainWait
// and the action runs on against a closed database.
func TestTerminateCancelsRunningActionAndWaitsForIt(t *testing.T) {
	app, _, u := engineApp(t)
	col, _ := app.FindCollectionByNameOrId("tickets")

	started := make(chan struct{})
	returned := make(chan struct{})
	var wrote atomic.Bool
	var dbOpenAtReturn atomic.Bool
	RegisterAction("tickets:boom", func(a core.App, req ActionRequest) error {
		close(started)
		defer close(returned)
		<-req.Context().Done()
		// Still busy for a moment after the cancel, as an action with a call
		// in flight is: the worker has stopped waiting by now, so only the
		// drain's wait for the action keeps the database open.
		time.Sleep(200 * time.Millisecond)
		made := core.NewRecord(col)
		made.Set("id", core.GenerateDefaultRandomId())
		made.Set("title", "after cancel")
		made.Set("user", u.Id)
		err := MarkEngineWrite(a, req, made.Id, func() error {
			wrote.Store(true)
			return a.Save(made)
		})
		dbOpenAtReturn.Store(appIsLive(a))
		return err
	})
	makeRule(t, app, u.Id, "org", nil,
		[]any{map[string]any{"ref": "tickets:boom"}}, 0, false)

	seed := core.NewRecord(col)
	seed.Set("title", "seed")
	seed.Set("user", u.Id)
	if err := app.Save(seed); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		t.Fatal("the action never started")
	}

	terminated := make(chan struct{})
	begun := time.Now()
	go func() {
		app.(*tests.TestApp).Cleanup()
		close(terminated)
	}()
	select {
	case <-terminated:
	case <-time.After(30 * time.Second):
		t.Fatal("terminate did not return")
	}

	select {
	case <-returned:
	default:
		t.Fatal("terminate returned while the action was still running")
	}
	if took := time.Since(begun); took >= shutdownDrainWait {
		t.Fatalf("terminate took %v: the drain waited out its bound instead of cancelling the action", took)
	}
	if !dbOpenAtReturn.Load() {
		t.Fatal("the database closed before the cancelled action returned")
	}
	if wrote.Load() {
		t.Fatal("MarkEngineWrite ran a write after the action's context ended")
	}
}

// An action whose context has ended writes nothing: not a record-op's create
// or update, and not a native handler's write through MarkEngineWrite.
func TestActionWritesNothingAfterCancel(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	defs.Packages[1].Actions = append(defs.Packages[1].Actions, ActionDef{ID: "stamp", Kind: "native"})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("record-op update", func(t *testing.T) {
		err := ExecuteActionContext(ctx, app, defs, "things:set-status",
			map[string]any{"status": "filed"}, rule, openTrigger, rec, 0, nil)
		if err == nil {
			t.Fatal("a cancelled record-op update must fail")
		}
		fresh, _ := app.FindRecordById("things", rec.Id)
		if fresh.GetString("status") != "new" {
			t.Fatalf("a cancelled update was saved: status %q", fresh.GetString("status"))
		}
		if rec.GetString("status") != "new" {
			t.Fatalf("a cancelled update changed the trigger record in memory: status %q", rec.GetString("status"))
		}
	})

	t.Run("record-op create", func(t *testing.T) {
		err := ExecuteActionContext(ctx, app, defs, "core:apply-label",
			map[string]any{"label": "Finance"}, rule, openTrigger, rec, 0, nil)
		if err == nil {
			t.Fatal("a cancelled record-op create must fail")
		}
		made, _ := app.FindRecordsByFilter("thing_labels", "record_id = {:id}", "", 0, 0, map[string]any{"id": rec.Id})
		if len(made) != 0 {
			t.Fatalf("a cancelled create was saved: %d rows", len(made))
		}
	})

	t.Run("native write through MarkEngineWrite", func(t *testing.T) {
		thingsCol, _ := app.FindCollectionByNameOrId("things")
		var wrote bool
		RegisterAction("things:stamp", func(a core.App, req ActionRequest) error {
			made := core.NewRecord(thingsCol)
			made.Set("id", core.GenerateDefaultRandomId())
			made.Set("title", "stamped")
			return MarkEngineWrite(a, req, made.Id, func() error {
				wrote = true
				return a.Save(made)
			})
		})
		if err := ExecuteActionContext(ctx, app, defs, "things:stamp", nil, rule, openTrigger, rec, 0, nil); err == nil {
			t.Fatal("a cancelled native write must fail")
		}
		if wrote {
			t.Fatal("MarkEngineWrite ran the write after the context ended")
		}
		if n, _ := app.CountRecords("things"); n != 1 {
			t.Fatalf("a cancelled native write was saved: %d things", n)
		}
	})
}

// A handler called directly, as a package's own test does, gets a usable
// context rather than nil.
func TestActionRequestContextDefaultsToBackground(t *testing.T) {
	if (ActionRequest{}).Context() == nil {
		t.Fatal("Context() must never return nil")
	}
}

// core's own native actions bind their outbound work to the action's context,
// so the engine's cancel ends a send or a post in progress.
func TestCoreActionsHonourTheActionContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("core:send-email", func(t *testing.T) {
		app, rule := runsApp(t)
		// Read at call time: a context the action derives and cancels on
		// return would read as ended afterwards whatever its parent was.
		var endedAtSend bool
		original := sendEmailNow
		sendEmailNow = func(sendCtx context.Context, _ core.App, _ *mailer.Message) error {
			endedAtSend = sendCtx.Err() != nil
			return sendCtx.Err()
		}
		t.Cleanup(func() { sendEmailNow = original })
		err := actionSendEmail(app, ActionRequest{
			Rule: rule, Ctx: ctx,
			Params: map[string]string{"to": "someone@example.com", "subject": "s"},
		})
		if !endedAtSend {
			t.Fatal("the send did not get the action's context")
		}
		if err == nil {
			t.Fatal("a send cut short by the cancel must fail the action")
		}
	})

	t.Run("core:post-webhook", func(t *testing.T) {
		var endedAtPost bool
		original := postWebhookNow
		postWebhookNow = func(postCtx context.Context, _ string, _ []byte, _ map[string]string) (int, error) {
			endedAtPost = postCtx.Err() != nil
			return 0, postCtx.Err()
		}
		t.Cleanup(func() { postWebhookNow = original })
		err := actionPostWebhook(nil, ActionRequest{
			Ctx:    ctx,
			Params: map[string]string{"url": "https://hooks.example.com/abc"},
		})
		if !endedAtPost {
			t.Fatal("the post did not get the action's context")
		}
		if err == nil {
			t.Fatal("a post cut short by the cancel must fail the action")
		}
	})

	t.Run("core:notify", func(t *testing.T) {
		t.Cleanup(ResetRegistriesForTest)
		registerCoreNativeActions()
		var delivered bool
		original := deliverNotification
		deliverNotification = func(context.Context, core.App, notify.NotifyParams) error {
			delivered = true
			return nil
		}
		t.Cleanup(func() { deliverNotification = original })
		app, rule := runsApp(t)
		defs := &Defs{Packages: []PackageDefs{{
			Slug:    "core",
			Actions: []ActionDef{{ID: "notify", Kind: "native", Params: []ParamDef{{Key: "title"}}}},
		}}}
		err := ExecuteActionContext(ctx, app, defs, "core:notify",
			map[string]any{"title": "x"}, rule, TriggerDef{}, nil, 0, nil)
		if err == nil {
			t.Fatal("core:notify must refuse to write after its context ends")
		}
		if delivered {
			t.Fatal("core:notify wrote the notification after its context ended")
		}
	})
}
