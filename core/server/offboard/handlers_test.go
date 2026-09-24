package offboard

import (
	"errors"
	"fmt"
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

// The handler tests use a fictional "widgets" package: core must not name a
// real one. widget_members is the membership-shaped table a flat FK rewrite
// cannot settle, which is the case the handler registry exists for.

type handlerCall struct {
	leaverID    string
	leaverName  string
	plan        Plan
	actorUserID string
	inTx        bool
}

func setupHandlerApp(t *testing.T) core.App {
	t.Helper()
	app := setupTestApp(t)
	ResetHandlersForTesting()
	t.Cleanup(ResetHandlersForTesting)

	members := core.NewBaseCollection("widget_members")
	members.Fields.Add(&core.TextField{Name: "widget", Required: true})
	members.Fields.Add(&core.TextField{Name: "user", Required: true})
	if err := app.Save(members); err != nil {
		t.Fatalf("save widget_members: %v", err)
	}
	return app
}

func recordingHandler(calls *[]handlerCall) Handler {
	return func(txApp core.App, leaver *core.Record, plan Plan, actorUserID string) error {
		*calls = append(*calls, handlerCall{
			leaverID:    leaver.Id,
			leaverName:  leaver.GetString("name"),
			plan:        plan,
			actorUserID: actorUserID,
			inTx:        txApp.IsTransactional(),
		})
		return nil
	}
}

func TestRegisterHandler_CalledWithPlanAndActor(t *testing.T) {
	for _, mode := range []Mode{ModeReassign, ModeDeleteMyData, ModeKeep} {
		t.Run(string(mode), func(t *testing.T) {
			app := setupHandlerApp(t)
			alice := makeUser(t, app, "alice@test.local")
			bob := makeUser(t, app, "bob@test.local")
			admin := makeUser(t, app, "admin@test.local")

			var calls []handlerCall
			RegisterHandler("widgets", recordingHandler(&calls))

			plan := Plan{Mode: mode}
			if mode == ModeReassign {
				plan.SuccessorUserID = bob.Id
			}
			if _, err := OffboardUser(app, alice.Id, plan, admin.Id); err != nil {
				t.Fatalf("OffboardUser: %v", err)
			}
			if len(calls) != 1 {
				t.Fatalf("handler calls = %d, want 1", len(calls))
			}
			got := calls[0]
			want := handlerCall{
				leaverID: alice.Id, leaverName: "T", plan: plan,
				actorUserID: admin.Id, inTx: true,
			}
			if got != want {
				t.Errorf("handler call = %+v, want %+v", got, want)
			}
		})
	}
}

// A handler sees the leaver before anonymization, and can write through the
// transaction app so its work commits with the offboard.
func TestRegisterHandler_WritesCommitWithOffboard(t *testing.T) {
	app := setupHandlerApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	col, _ := app.FindCollectionByNameOrId("widget_members")
	m := core.NewRecord(col)
	m.Set("widget", "w1")
	m.Set("user", alice.Id)
	if err := app.Save(m); err != nil {
		t.Fatalf("save member: %v", err)
	}

	RegisterHandler("widgets", func(txApp core.App, leaver *core.Record, plan Plan, _ string) error {
		rows, err := txApp.FindRecordsByFilter("widget_members", "user = {:u}", "", 0, 0,
			map[string]any{"u": leaver.Id})
		if err != nil {
			return err
		}
		for _, r := range rows {
			r.Set("user", plan.SuccessorUserID)
			if err := txApp.Save(r); err != nil {
				return err
			}
		}
		return nil
	})

	if _, err := OffboardUser(app, alice.Id, Plan{Mode: ModeReassign, SuccessorUserID: bob.Id}, alice.Id); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	fresh, _ := app.FindRecordById("widget_members", m.Id)
	if fresh.GetString("user") != bob.Id {
		t.Errorf("member user = %q, want %q", fresh.GetString("user"), bob.Id)
	}
}

// A handler error rolls back everything: the reassigned FKs, the other
// handlers' writes and the anonymization.
func TestRegisterHandler_ErrorRollsBack(t *testing.T) {
	app := setupHandlerApp(t)
	alice := makeUser(t, app, "alice@test.local")
	bob := makeUser(t, app, "bob@test.local")
	event := makeEvent(t, app, "Standup", alice.Id)
	col, _ := app.FindCollectionByNameOrId("widget_members")

	// "a-widgets" runs before "b-widgets" (name order), so its write is in
	// the transaction when the second handler fails.
	RegisterHandler("a-widgets", func(txApp core.App, leaver *core.Record, _ Plan, _ string) error {
		r := core.NewRecord(col)
		r.Set("widget", "w-written")
		r.Set("user", leaver.Id)
		return txApp.Save(r)
	})
	boom := errors.New("boom")
	RegisterHandler("b-widgets", func(core.App, *core.Record, Plan, string) error { return boom })

	_, err := OffboardUser(app, alice.Id, Plan{Mode: ModeReassign, SuccessorUserID: bob.Id}, alice.Id)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if errors.Is(err, ErrInvalidPlan) {
		t.Error("a plain handler failure must not read as an invalid plan")
	}

	fresh, _ := app.FindRecordById("test_events", event.Id)
	if fresh.GetString("created_by") != alice.Id {
		t.Errorf("event reassignment not rolled back: created_by = %q", fresh.GetString("created_by"))
	}
	if n, _ := app.CountRecords("widget_members"); n != 0 {
		t.Errorf("first handler's write not rolled back: %d widget_members rows", n)
	}
	still, _ := app.FindRecordById("users", alice.Id)
	if still.GetString("name") == "Deleted user" {
		t.Error("user anonymized despite a handler error")
	}
}

// A handler that rejects the plan wraps ErrInvalidPlan; the message reaches
// the caller without a handler-name prefix.
func TestRegisterHandler_InvalidPlanPassesThrough(t *testing.T) {
	app := setupHandlerApp(t)
	alice := makeUser(t, app, "alice@test.local")

	RegisterHandler("widgets", func(core.App, *core.Record, Plan, string) error {
		return fmt.Errorf("%w: hand your widgets over first", ErrInvalidPlan)
	})

	_, err := OffboardUser(app, alice.Id, Plan{Mode: ModeDeleteMyData}, alice.Id)
	if !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	if want := "invalid offboard plan: hand your widgets over first"; err.Error() != want {
		t.Errorf("err = %q, want %q", err.Error(), want)
	}
}

// Handlers are not called when core rejects the plan itself.
func TestRegisterHandler_NotCalledOnInvalidPlan(t *testing.T) {
	app := setupHandlerApp(t)
	alice := makeUser(t, app, "alice@test.local")
	var calls []handlerCall
	RegisterHandler("widgets", recordingHandler(&calls))

	if _, err := OffboardUser(app, alice.Id, Plan{Mode: ModeReassign}, alice.Id); !errors.Is(err, ErrInvalidPlan) {
		t.Fatalf("err = %v, want ErrInvalidPlan", err)
	}
	if len(calls) != 0 {
		t.Errorf("handler called %d times on a rejected plan", len(calls))
	}
}

// Registration is idempotent by name (first wins) and handlers run in name
// order.
func TestRegisterHandler_IdempotentByNameAndOrdered(t *testing.T) {
	app := setupHandlerApp(t)
	alice := makeUser(t, app, "alice@test.local")

	var order []string
	mk := func(label string) Handler {
		return func(core.App, *core.Record, Plan, string) error {
			order = append(order, label)
			return nil
		}
	}
	RegisterHandler("zeta", mk("zeta"))
	RegisterHandler("alpha", mk("alpha"))
	RegisterHandler("alpha", mk("alpha-duplicate"))
	RegisterHandler("", mk("unnamed"))
	RegisterHandler("nil-handler", nil)

	if _, err := OffboardUser(app, alice.Id, Plan{Mode: ModeDeleteMyData}, ""); err != nil {
		t.Fatalf("OffboardUser: %v", err)
	}
	if got := fmt.Sprint(order); got != "[alpha zeta]" {
		t.Errorf("handler order = %s, want [alpha zeta]", got)
	}
}
