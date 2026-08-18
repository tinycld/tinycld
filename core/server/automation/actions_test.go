// tinycld/core/server/automation/actions_test.go
package automation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"tinycld.org/core/rlstest"
)

// allowAuthedWrites gives a fixture collection the update/delete rules a real
// feature collection always declares in its migration. The trigger-record
// authorization floor evaluates them as the rule owner, and treats a nil rule
// (PB's superuser-only) as a refusal — so a fixture with no rules at all would
// test a configuration no deployment ships.
func allowAuthedWrites(t *testing.T, app core.App, col *core.Collection) {
	t.Helper()
	authed := "@request.auth.id != ''"
	col.UpdateRule = &authed
	col.DeleteRule = &authed
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
}

// actionApp builds: users (fixture), label_assignments-like target collection,
// a source collection, one user, one source record, and a personal rule record
// shape (plain base collection standing in for `rules` — executor only reads
// scope/owner via GetString).
func actionApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record, *Defs) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Cleanup() })
	users, _ := app.FindCollectionByNameOrId("users")

	src := core.NewBaseCollection("things")
	src.Fields.Add(&core.TextField{Name: "title"})
	src.Fields.Add(&core.TextField{Name: "status"})
	if err := app.Save(src); err != nil {
		t.Fatal(err)
	}
	allowAuthedWrites(t, app, src)
	tgt := core.NewBaseCollection("thing_labels")
	tgt.Fields.Add(&core.TextField{Name: "label"})
	tgt.Fields.Add(&core.TextField{Name: "record_id"})
	tgt.Fields.Add(&core.TextField{Name: "collection"})
	tgt.Fields.Add(&core.RelationField{Name: "user", CollectionId: users.Id, MaxSelect: 1})
	if err := app.Save(tgt); err != nil {
		t.Fatal(err)
	}
	rulesCol := core.NewBaseCollection("fake_rules")
	rulesCol.Fields.Add(&core.TextField{Name: "scope"})
	rulesCol.Fields.Add(&core.TextField{Name: "owner"})
	if err := app.Save(rulesCol); err != nil {
		t.Fatal(err)
	}

	u, _ := app.FindFirstRecordByFilter("users", "id != ''")
	rec := core.NewRecord(src)
	rec.Set("title", "Invoice #7")
	rec.Set("status", "new")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	rule := core.NewRecord(rulesCol)
	rule.Set("scope", "org")
	rule.Set("owner", u.Id)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}

	defs := &Defs{Packages: []PackageDefs{{
		Slug: "core",
		Actions: []ActionDef{
			{
				ID: "apply-label", Kind: "record-op", Collection: "thing_labels",
				Op: RecordOp{Type: "create", Set: map[string]SetValue{
					"label":      {Param: "label"},
					"record_id":  {Context: "record-id"},
					"collection": {Context: "collection"},
					"user":       {Context: "owner"},
				}},
				Params: []ParamDef{{Key: "label", Field: "label"}},
			},
			{ID: "notify", Kind: "native", Params: []ParamDef{{Key: "title", Type: "text"}}},
		},
	}, {
		Slug: "things",
		// An update trigger IS declared here (unlike thing_labels above) so
		// TestRecordOpUpdateTriggerRecord exercises the positive sentinel-marking
		// path: a hook is actually bound for (things, update).
		Triggers: []TriggerDef{{ID: "updated", Collection: "things", On: "update"}},
		Actions: []ActionDef{{
			ID: "set-status", Kind: "record-op", Collection: "things",
			Op:     RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"status": {Param: "status"}}},
			Params: []ParamDef{{Key: "status", Field: "status"}},
		}},
	}}}
	return app, rec, rule, defs
}

var openTrigger = TriggerDef{Collection: "things", On: "create"}

func TestRecordOpCreateWithContext(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	err := ExecuteAction(app, defs, "core:apply-label", map[string]any{"label": "Finance {{title}}"}, rule, openTrigger, rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	made, err := app.FindFirstRecordByFilter("thing_labels", "record_id = {:id}", map[string]any{"id": rec.Id})
	if err != nil {
		t.Fatal(err)
	}
	if made.GetString("label") != "Finance Invoice #7" {
		t.Fatalf("template param: %q", made.GetString("label"))
	}
	if made.GetString("collection") != "things" || made.GetString("user") != rule.GetString("owner") {
		t.Fatalf("context values: %+v", made.PublicExport())
	}
	// thing_labels has no bound hook in this defs set (core:apply-label-style
	// write with nothing declared to trigger on it) — marking a sentinel here
	// would leak the sync.Map entry forever since nothing will ever consume it.
	if _, ok := takeEngineWrite(made.Id); ok {
		t.Fatal("create into a collection with no trigger must not leave a sentinel")
	}
}

func TestRecordOpUpdateTriggerRecord(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 1); err != nil {
		t.Fatal(err)
	}
	fresh, _ := app.FindRecordById("things", rec.Id)
	if fresh.GetString("status") != "filed" {
		t.Fatal("update did not apply")
	}
	w, ok := takeEngineWrite(rec.Id)
	if !ok || w.Depth != 1 || w.RuleID != rule.Id {
		t.Fatalf("provenance: %+v %v", w, ok)
	}
}

// TestTriggerRecordWriteAuthorization is the B3 regression. A record-op on the
// trigger record runs as superuser, so before the fix a rule owner who could
// merely SEE a record could mutate or delete it through automation even where
// the collection's own rules refuse that same edit — the cards case being a
// viewer moving a card. The floor is the collection's update/delete rule
// evaluated as the rule owner.
func TestTriggerRecordWriteAuthorization(t *testing.T) {
	t.Run("owner who fails the update rule is refused", func(t *testing.T) {
		app, rec, rule, defs := actionApp(t)

		// Stand in for "viewer, not editor": a rule only the record's own
		// creator satisfies, which this rule's owner is not.
		col, _ := app.FindCollectionByNameOrId("things")
		onlySomeoneElse := "@request.auth.id = 'nonexistent_user_id'"
		col.UpdateRule = &onlySomeoneElse
		if err := app.Save(col); err != nil {
			t.Fatal(err)
		}
		rec, _ = app.FindRecordById("things", rec.Id)

		err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0)
		if err == nil {
			t.Fatal("a rule owner who fails the collection's update rule must not move the trigger record")
		}
		fresh, _ := app.FindRecordById("things", rec.Id)
		if fresh.GetString("status") == "filed" {
			t.Fatal("the refused update must not have been written")
		}
		if _, ok := takeEngineWrite(rec.Id); ok {
			t.Fatal("a refused write must not strand a provenance sentinel")
		}
	})

	t.Run("superuser-only collection is refused", func(t *testing.T) {
		app, rec, rule, defs := actionApp(t)

		// nil rule = superuser-only in PocketBase. Acting as the rule owner
		// here would be a straight privilege escalation, so it fails closed.
		col, _ := app.FindCollectionByNameOrId("things")
		col.UpdateRule = nil
		if err := app.Save(col); err != nil {
			t.Fatal(err)
		}
		rec, _ = app.FindRecordById("things", rec.Id)

		if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0); err == nil {
			t.Fatal("a superuser-only collection must refuse a trigger-record write")
		}
	})

	t.Run("registered authorizer can refuse what the floor allows", func(t *testing.T) {
		app, rec, rule, defs := actionApp(t)
		RegisterTriggerRecordAuthorizer("things:set-status",
			func(_ core.App, _ ActionRequest, r *core.Record) error {
				return fmt.Errorf("package says no for %s", r.Id)
			})

		err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0)
		if err == nil {
			t.Fatal("a registered trigger-record authorizer must be able to refuse")
		}
		if !strings.Contains(err.Error(), "package says no") {
			t.Fatalf("the authorizer's reason must surface: %v", err)
		}
	})

	t.Run("delete is gated by the delete rule", func(t *testing.T) {
		app, rec, rule, defs := actionApp(t)
		defs.Packages[1].Actions = append(defs.Packages[1].Actions, ActionDef{
			ID: "drop", Kind: "record-op", Collection: "things",
			Op: RecordOp{Type: "delete", Target: "trigger-record"},
		})

		col, _ := app.FindCollectionByNameOrId("things")
		onlySomeoneElse := "@request.auth.id = 'nonexistent_user_id'"
		col.DeleteRule = &onlySomeoneElse
		if err := app.Save(col); err != nil {
			t.Fatal(err)
		}
		rec, _ = app.FindRecordById("things", rec.Id)

		if err := ExecuteAction(app, defs, "things:drop", nil, rule, openTrigger, rec, 0); err == nil {
			t.Fatal("a rule owner who fails the delete rule must not delete the trigger record")
		}
		if _, err := app.FindRecordById("things", rec.Id); err != nil {
			t.Fatal("the record must still exist after a refused delete")
		}
	})
}

// TestFailedSaveLeavesNoSentinel proves the mark-before-write ordering
// doesn't strand a sentinel on a real record id when the write itself fails:
// a stranded sentinel would mis-attribute provenance (depth/source rule) to
// whatever legitimate save of that same id happens next, wrongly suppressing
// a rule that should fire.
func TestFailedSaveLeavesNoSentinel(t *testing.T) {
	app, rec, rule, defs := actionApp(t)

	// Add a required field to "things" with no default and nothing in the
	// action's Set map — Save must fail validation.
	col, err := app.FindCollectionByNameOrId("things")
	if err != nil {
		t.Fatal(err)
	}
	col.Fields.Add(&core.TextField{Name: "must_have", Required: true})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	// Re-fetch: rec was constructed against the pre-migration collection
	// schema, so its captured Collection() wouldn't see the new required
	// field and validation would silently pass it by.
	rec, err = app.FindRecordById("things", rec.Id)
	if err != nil {
		t.Fatal(err)
	}
	// things:update already has a trigger declared in actionApp's defs, so
	// this write would be marked absent the fix under test.
	if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0); err == nil {
		t.Fatal("save must fail: required field unset")
	}
	if _, ok := takeEngineWrite(rec.Id); ok {
		t.Fatal("a failed save must not strand a sentinel on the record id")
	}
}

func TestNativeDispatchAndMissingHandler(t *testing.T) {
	app, rec, rule, defs := actionApp(t)
	var got ActionRequest
	RegisterAction("core:notify", func(a core.App, req ActionRequest) error {
		got = req
		return nil
	})
	err := ExecuteAction(app, defs, "core:notify", map[string]any{"title": "Got {{title}}"}, rule, openTrigger, rec, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Params["title"] != "Got Invoice #7" {
		t.Fatalf("substituted params must reach handler: %+v", got.Params)
	}
	ResetRegistriesForTest()
	if err := ExecuteAction(app, defs, "core:notify", nil, rule, openTrigger, rec, 0); err == nil {
		t.Fatal("missing native handler must error")
	}
}

func TestTriggerRecordOpNeedsMatchingCollection(t *testing.T) {
	app, _, rule, defs := actionApp(t)
	if err := ExecuteAction(app, defs, "things:set-status", nil, rule, openTrigger, nil, 0); err == nil {
		t.Fatal("trigger-record op with nil record must error")
	}
}

// pkgaccessApp applies core's REAL shipped migrations (rlstest), so
// checkPersonalAccess is exercised against the actual pkg_registry /
// org_pkg_access / rules schema rather than the fake_rules stand-in the other
// tests use. It creates the same "things" source collection as actionApp,
// registers it in pkg_registry as an installed package (pkgaccess derives
// ownership from the collection name matching an installed slug), and
// returns (app, trigger record, member user, personal rule, Defs) for
// things:set-status.
func pkgaccessApp(t *testing.T) (*tests.TestApp, *core.Record, *core.Record, *core.Record, *Defs) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	app := rlstest.NewApp(t)

	// Same fixture reconciliation as coreserver's RLS suites: drop the
	// bundled username index so 1820000000 (users_username_required) applies
	// the way it does against a real DB.
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	var kept types.JSONArray[string]
	for _, idx := range users.Indexes {
		if !strings.Contains(idx, "username") {
			kept = append(kept, idx)
		}
	}
	users.Indexes = kept
	users.PasswordAuth.IdentityFields = []string{"email"}
	if err := app.Save(users); err != nil {
		t.Fatalf("drop fixture username index: %v", err)
	}

	rlstest.Apply(t, app, rlstest.MigrationsDir(t, "../pb_migrations"))

	src := core.NewBaseCollection("things")
	src.Fields.Add(&core.TextField{Name: "title"})
	src.Fields.Add(&core.TextField{Name: "status"})
	if err := app.Save(src); err != nil {
		t.Fatal(err)
	}
	allowAuthedWrites(t, app, src)

	registry, err := app.FindCollectionByNameOrId("pkg_registry")
	if err != nil {
		t.Fatal(err)
	}
	reg := core.NewRecord(registry)
	reg.Set("name", "Things")
	reg.Set("slug", "things")
	reg.Set("status", "installed")
	if err := app.Save(reg); err != nil {
		t.Fatal(err)
	}

	member := pkgaccessTestUser(t, app, "member@test.local", "member")

	rulesCol, err := app.FindCollectionByNameOrId("rules")
	if err != nil {
		t.Fatal(err)
	}
	rule := core.NewRecord(rulesCol)
	rule.Set("name", "file it")
	rule.Set("scope", "personal")
	rule.Set("owner", member.Id)
	rule.Set("trigger", "things:create")
	rule.Set("actions", `[]`)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}

	rec := core.NewRecord(src)
	rec.Set("title", "Invoice #7")
	rec.Set("status", "new")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}

	defs := &Defs{Packages: []PackageDefs{{
		Slug: "things",
		Actions: []ActionDef{{
			ID: "set-status", Kind: "record-op", Collection: "things",
			Op:     RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"status": {Param: "status"}}},
			Params: []ParamDef{{Key: "status", Field: "status"}},
		}},
	}}}
	return app, rec, member, rule, defs
}

func pkgaccessTestUser(t *testing.T, app core.App, email, role string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.SetEmail(email)
	r.Set("username", strings.SplitN(email, "@", 2)[0])
	r.Set("name", "Test")
	r.Set("role", role)
	r.SetVerified(true)
	r.SetPassword("Password123!")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func denyOrgPkgAccess(t *testing.T, app core.App, userID, pkg, access string) {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("org_pkg_access")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("user", userID)
	r.Set("pkg", pkg)
	r.Set("access", access)
	if err := app.Save(r); err != nil {
		t.Fatalf("seed org_pkg_access: %v", err)
	}
}

// TestPersonalScopePkgaccess proves ExecuteAction's checkPersonalAccess is
// wired to the REAL pkgaccess package against the shipped schema: a personal
// rule owned by a member with no write access to the "things" package is
// denied; the identical action under an org-scope rule bypasses the check
// (system authority, spec-documented); and a personal rule succeeds once the
// member is granted "full" access.
func TestPersonalScopePkgaccess(t *testing.T) {
	app, rec, member, rule, defs := pkgaccessApp(t)

	// No org_pkg_access row would default to LevelFull (see LevelFor), so
	// deny explicitly to exercise the readonly branch of WriteError.
	denyOrgPkgAccess(t, app, member.Id, "things", "readonly")

	if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0); err == nil {
		t.Fatal("personal rule with readonly pkgaccess must be denied")
	}

	rule.Set("scope", "org")
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0); err != nil {
		t.Fatalf("org rule must bypass pkgaccess (system authority): %v", err)
	}

	rule.Set("scope", "personal")
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}
	existing, err := app.FindFirstRecordByFilter("org_pkg_access",
		"user = {:user} && pkg = {:pkg}",
		map[string]any{"user": member.Id, "pkg": "things"})
	if err != nil {
		t.Fatal(err)
	}
	existing.Set("access", "full")
	if err := app.Save(existing); err != nil {
		t.Fatal(err)
	}
	if err := ExecuteAction(app, defs, "things:set-status", map[string]any{"status": "filed"}, rule, openTrigger, rec, 0); err != nil {
		t.Fatalf("personal rule with full pkgaccess must succeed: %v", err)
	}
}

// relationGateApp builds the fixture the relation-param gates need: two
// users, a per-user-visible target collection (boxes, view rule
// `owner = @request.auth.id`), a LOCKED target collection (vaults, nil view
// rule), a source collection whose record-op param names a relation column,
// and native actions declaring typed relation params against both targets.
func relationGateApp(t *testing.T) (app *tests.TestApp, u1, u2, rule, crate, box1, box2, vault *core.Record, defs *Defs) {
	t.Helper()
	t.Cleanup(ResetRegistriesForTest)
	testApp, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testApp.Cleanup() })
	app = testApp
	users, _ := app.FindCollectionByNameOrId("users")

	newUser := func(email string) *core.Record {
		r := core.NewRecord(users)
		r.SetEmail(email)
		r.SetPassword("Password123!")
		r.SetVerified(true)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	u1, u2 = newUser("gate-owner@test.local"), newUser("gate-other@test.local")

	boxes := core.NewBaseCollection("boxes")
	boxes.Fields.Add(&core.TextField{Name: "name"})
	boxes.Fields.Add(&core.RelationField{Name: "owner", CollectionId: users.Id, MaxSelect: 1})
	boxes.ViewRule = types.Pointer("owner = @request.auth.id")
	if err := app.Save(boxes); err != nil {
		t.Fatal(err)
	}
	vaults := core.NewBaseCollection("vaults")
	vaults.Fields.Add(&core.TextField{Name: "name"})
	if err := app.Save(vaults); err != nil {
		t.Fatal(err)
	}
	crates := core.NewBaseCollection("crates")
	crates.Fields.Add(&core.TextField{Name: "title"})
	crates.Fields.Add(&core.RelationField{Name: "box", CollectionId: boxes.Id, MaxSelect: 1})
	if err := app.Save(crates); err != nil {
		t.Fatal(err)
	}
	allowAuthedWrites(t, app, crates)
	rulesCol := core.NewBaseCollection("fake_rules")
	rulesCol.Fields.Add(&core.TextField{Name: "scope"})
	rulesCol.Fields.Add(&core.TextField{Name: "owner"})
	if err := app.Save(rulesCol); err != nil {
		t.Fatal(err)
	}

	mkBox := func(owner *core.Record) *core.Record {
		r := core.NewRecord(boxes)
		r.Set("name", "box of "+owner.Email())
		r.Set("owner", owner.Id)
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	box1, box2 = mkBox(u1), mkBox(u2)
	vault = core.NewRecord(vaults)
	vault.Set("name", "locked")
	if err := app.Save(vault); err != nil {
		t.Fatal(err)
	}

	crate = core.NewRecord(crates)
	// The title deliberately holds a REAL box id: if a relation param were
	// template-substituted, "{{title}}" would resolve to it and succeed —
	// the no-substitution test relies on it failing verbatim instead.
	crate.Set("title", box1.Id)
	if err := app.Save(crate); err != nil {
		t.Fatal(err)
	}

	rule = core.NewRecord(rulesCol)
	rule.Set("scope", "org")
	rule.Set("owner", u1.Id)
	if err := app.Save(rule); err != nil {
		t.Fatal(err)
	}

	defs = &Defs{Packages: []PackageDefs{{
		Slug: "crates",
		Actions: []ActionDef{
			{
				ID: "file", Kind: "record-op", Collection: "crates",
				Op:     RecordOp{Type: "update", Target: "trigger-record", Set: map[string]SetValue{"box": {Param: "box"}}},
				Params: []ParamDef{{Key: "box", Field: "box"}},
			},
			{ID: "file-native", Kind: "native",
				Params: []ParamDef{{Key: "box", Type: "relation", RelationTarget: "boxes"}}},
			{ID: "vault-native", Kind: "native",
				Params: []ParamDef{{Key: "vault", Type: "relation", RelationTarget: "vaults"}}},
		},
	}}}
	return app, u1, u2, rule, crate, box1, box2, vault, defs
}

var crateTrigger = TriggerDef{Collection: "crates", On: "create"}

func allowRelation(ref, key string) {
	RegisterRelationAuthorizer(ref, key, func(core.App, ActionRequest, string) error { return nil })
}

func TestRelationParamGates(t *testing.T) {
	app, _, _, rule, crate, box1, box2, vault, defs := relationGateApp(t)

	t.Run("no authorizer refuses even the owner's own record", func(t *testing.T) {
		err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": box1.Id}, rule, crateTrigger, crate, 0)
		if err == nil || !strings.Contains(err.Error(), "no registered authorizer") {
			t.Fatalf("want the missing-authorizer refusal, got %v", err)
		}
	})

	allowRelation("crates:file", "box")
	allowRelation("crates:file-native", "box")
	allowRelation("crates:vault-native", "vault")

	t.Run("owner-visible record passes", func(t *testing.T) {
		if err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": box1.Id}, rule, crateTrigger, crate, 0); err != nil {
			t.Fatalf("owner's own box must pass: %v", err)
		}
		fresh, _ := app.FindRecordById("crates", crate.Id)
		if fresh.GetString("box") != box1.Id {
			t.Fatalf("box = %q, want %q", fresh.GetString("box"), box1.Id)
		}
	})

	t.Run("floor blocks a record the owner cannot view", func(t *testing.T) {
		err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": box2.Id}, rule, crateTrigger, crate, 0)
		if err == nil || !strings.Contains(err.Error(), "may not use") {
			t.Fatalf("want the floor denial, got %v", err)
		}
	})

	t.Run("missing record is refused", func(t *testing.T) {
		err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": "nope12345"}, rule, crateTrigger, crate, 0)
		if err == nil || !strings.Contains(err.Error(), "no boxes record") {
			t.Fatalf("want the missing-record refusal, got %v", err)
		}
	})

	t.Run("relation params are never template-substituted", func(t *testing.T) {
		// crate.title IS box1's id — substitution would resolve and succeed.
		err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": "{{title}}"}, rule, crateTrigger, crate, 0)
		if err == nil || !strings.Contains(err.Error(), "no boxes record") {
			t.Fatalf("a templated relation param must fail verbatim, got %v", err)
		}
	})

	t.Run("empty value skips the gates", func(t *testing.T) {
		if err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": ""}, rule, crateTrigger, crate, 0); err != nil {
			t.Fatalf("an empty relation value has nothing to authorize: %v", err)
		}
	})

	t.Run("locked target collection blocks via the floor", func(t *testing.T) {
		RegisterAction("crates:vault-native", func(core.App, ActionRequest) error { return nil })
		err := ExecuteAction(app, defs, "crates:vault-native", map[string]any{"vault": vault.Id}, rule, crateTrigger, crate, 0)
		if err == nil || !strings.Contains(err.Error(), "may not use") {
			t.Fatalf("a nil view rule must fail closed, got %v", err)
		}
	})

	t.Run("native handler runs only after the gates", func(t *testing.T) {
		got := ""
		RegisterAction("crates:file-native", func(_ core.App, req ActionRequest) error {
			got = req.Params["box"]
			return nil
		})
		if err := ExecuteAction(app, defs, "crates:file-native", map[string]any{"box": box1.Id}, rule, crateTrigger, crate, 0); err != nil {
			t.Fatalf("gated native must run: %v", err)
		}
		if got != box1.Id {
			t.Fatalf("handler saw %q, want %q", got, box1.Id)
		}
		if err := ExecuteAction(app, defs, "crates:file-native", map[string]any{"box": box2.Id}, rule, crateTrigger, crate, 0); err == nil {
			t.Fatal("native handler must not run for a floor-blocked id")
		}
	})

	t.Run("authorizer veto fails the action", func(t *testing.T) {
		RegisterRelationAuthorizer("crates:file", "box", func(core.App, ActionRequest, string) error {
			return fmt.Errorf("destination is read-only for this user")
		})
		err := ExecuteAction(app, defs, "crates:file", map[string]any{"box": box1.Id}, rule, crateTrigger, crate, 0)
		if err == nil || !strings.Contains(err.Error(), "read-only for this user") {
			t.Fatalf("want the authorizer's veto, got %v", err)
		}
	})
}
