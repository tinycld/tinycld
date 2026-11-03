package automation

import (
	"context"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// A fictional trigger whose rules pick a "channel" in trigger_config and
// whose records carry their answers in a json column, as a web form does.
func dynamicApp(t *testing.T) (core.App, *Engine, *core.Record, func(channel, answer string) *core.Record) {
	t.Helper()
	app, _, u := engineApp(t)
	col := core.NewBaseCollection("posts")
	col.Fields.Add(&core.TextField{Name: "channel"})
	col.Fields.Add(&core.JSONField{Name: "answers"})
	col.Fields.Add(&core.AutodateField{Name: "created", OnCreate: true})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	defs := &Defs{Packages: []PackageDefs{{
		Slug:     "posts",
		Triggers: []TriggerDef{{ID: "post-created", Label: "post", Collection: "posts", On: "create", Fields: []FieldRef{{Key: "created"}}}},
		Actions:  []ActionDef{{ID: "capture", Label: "capture", Kind: "native", Params: []ParamDef{{Key: "note", Type: "text"}}}},
	}}}
	eng := NewEngine(app, defs)
	RegisterRuleFilter("posts:post-created", func(_ core.App, cfg map[string]any, rec *core.Record) bool {
		return cfg["channel"] == rec.GetString("channel")
	})
	RegisterFieldsResolver("posts:post-created", func(_ core.App, _ map[string]any, rec *core.Record) []DynamicField {
		var answers map[string]any
		_ = rec.UnmarshalJSONField("answers", &answers)
		return []DynamicField{{Key: "answer", Label: "Answer", Type: "text", Value: answers["answer"]}}
	})
	save := func(channel, answer string) *core.Record {
		r := core.NewRecord(col)
		r.Set("channel", channel)
		r.Set("answers", map[string]any{"answer": answer})
		if err := app.Save(r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	return app, eng, u, save
}

func dynamicRule(t *testing.T, app core.App, ownerID, channel, conditions, note string) *core.Record {
	t.Helper()
	col, _ := app.FindCollectionByNameOrId("rules")
	r := core.NewRecord(col)
	r.Set("name", "d")
	r.Set("scope", "org")
	r.Set("owner", ownerID)
	r.Set("trigger", "posts:post-created")
	r.Set("trigger_config", map[string]any{"channel": channel})
	r.Set("conditions", conditions)
	r.Set("actions", `[{"ref":"posts:capture","params":{"note":"`+note+`"}}]`)
	r.Set("enabled", true)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDynamicFieldsReachTemplatesConditionsAndHandlers(t *testing.T) {
	app, eng, u, save := dynamicApp(t)
	var got []ActionRequest
	RegisterAction("posts:capture", func(_ core.App, req ActionRequest) error {
		got = append(got, req)
		return nil
	})
	trigger, _, _ := eng.defs.Trigger("posts:post-created")
	wanted := `{"match":"all","groups":[{"match":"all","conditions":[{"field":"fields.answer","op":"contains","value":"yes"}]}]}`
	dynamicRule(t, app, u.Id, "a", wanted, "Answer: {{fields.answer}} / {{fields.gone}}")
	dynamicRule(t, app, u.Id, "b", `{"match":"all","groups":[]}`, "other channel")

	rec := save("a", "yes please")
	eng.DispatchForTest(event{TriggerRef: "posts:post-created", Trigger: trigger, Record: rec})

	if len(got) != 1 {
		t.Fatalf("only the channel-a rule may run, got %d", len(got))
	}
	if got[0].Params["note"] != "Answer: yes please / " {
		t.Fatalf("template must read fields.answer and blank an unknown key, got %q", got[0].Params["note"])
	}
	if len(got[0].Fields) != 1 || got[0].Fields[0].Key != "answer" {
		t.Fatalf("the handler gets the dynamic fields, got %#v", got[0].Fields)
	}

	got = nil
	eng.DispatchForTest(event{TriggerRef: "posts:post-created", Trigger: trigger, Record: save("a", "no")})
	if len(got) != 0 {
		t.Fatal("a condition on fields.answer must not match \"no\"")
	}
}

func TestConditionOnUnknownDynamicFieldFailsClosed(t *testing.T) {
	ast, err := DecodeConditions(types.JSONRaw(`{"match":"all","groups":[{"match":"all","conditions":[{"field":"fields.gone","op":"is_empty"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, save := dynamicApp(t)
	rec := save("a", "x")
	if EvaluateConditionsWith(ast, rec, TriggerDef{Fields: []FieldRef{{Key: "created"}}}, map[string]any{"fields.answer": "x"}) {
		t.Fatal("a condition on a key no resolver gave must fail closed")
	}
}

func TestPlaceholderAcceptsOnlyTheFieldsPrefix(t *testing.T) {
	if !placeholderRe.MatchString("{{fields.email}}") || !placeholderRe.MatchString("{{ title }}") {
		t.Fatal("plain and fields. placeholders must match")
	}
	if placeholderRe.MatchString("{{other.email}}") || strings.Contains(placeholderRe.String(), `\.\*`) {
		t.Fatal("only the fields. prefix is allowed")
	}
}

func TestMarkdownParamEscapesSubstitutedValues(t *testing.T) {
	app, _, u := engineApp(t)
	col := core.NewBaseCollection("notes")
	col.Fields.Add(&core.TextField{Name: "body"})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("body", "![x](https://tracker.example/p.png)")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	defs := &Defs{Packages: []PackageDefs{{
		Slug: "notes",
		Actions: []ActionDef{{ID: "write", Kind: "native", Params: []ParamDef{
			{Key: "md", Type: "text", Markdown: true},
			{Key: "plain", Type: "text"},
		}}},
	}}}
	var got ActionRequest
	RegisterAction("notes:write", func(_ core.App, req ActionRequest) error { got = req; return nil })
	rule := makeRule(t, app, u.Id, "org", nil, nil, 0, false)
	trigger := TriggerDef{ID: "note", Collection: "notes", On: "create"}
	raw := map[string]any{"md": "Note: {{body}} {{fields.answer}}", "plain": "{{body}}"}
	fields := []DynamicField{{Key: "answer", Value: "*b*"}}
	if err := ExecuteActionContext(context.Background(), app, defs, "notes:write", raw, rule, trigger, rec, 0, fields); err != nil {
		t.Fatal(err)
	}
	if want := `Note: \!\[x\]\(https\://tracker\.example/p\.png\) \*b\*`; got.Params["md"] != want {
		t.Fatalf("markdown param = %q, want %q", got.Params["md"], want)
	}
	if got.Params["plain"] != "![x](https://tracker.example/p.png)" {
		t.Fatalf("a plain param is not escaped: %q", got.Params["plain"])
	}
}

func TestDryRunReadsOnlyTheScopedRecords(t *testing.T) {
	_, eng, u, save := dynamicApp(t)
	RegisterRuleScope("posts:post-created", func(cfg map[string]any) (string, map[string]any) {
		return "channel = {:scopeChannel}", map[string]any{"scopeChannel": cfg["channel"]}
	})
	save("a", "the one")
	for i := 0; i < 60; i++ {
		save("b", "noise")
	}
	u.Set("role", "admin")
	ast, _ := DecodeConditions(nil)
	res, err := eng.dryRun(u, "posts:post-created", map[string]any{"channel": "a"}, ast)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 1 || len(res.Matches) != 1 {
		t.Fatalf("the dry run must read channel a only, got total %d matches %d", res.Total, len(res.Matches))
	}
}

// A trigger column that already holds markdown (a card description) is
// copied as it is: escaping it would show its formatting as raw characters.
func TestMarkdownTriggerFieldIsNotEscapedAgain(t *testing.T) {
	app, _, u := engineApp(t)
	col := core.NewBaseCollection("notes")
	col.Fields.Add(&core.TextField{Name: "body"})
	if err := app.Save(col); err != nil {
		t.Fatal(err)
	}
	rec := core.NewRecord(col)
	rec.Set("body", "**Steps:**\n- one")
	if err := app.Save(rec); err != nil {
		t.Fatal(err)
	}
	defs := &Defs{Packages: []PackageDefs{{
		Slug:    "notes",
		Actions: []ActionDef{{ID: "write", Kind: "native", Params: []ParamDef{{Key: "md", Type: "text", Markdown: true}}}},
	}}}
	var got ActionRequest
	RegisterAction("notes:write", func(_ core.App, req ActionRequest) error { got = req; return nil })
	rule := makeRule(t, app, u.Id, "org", nil, nil, 0, false)
	trigger := TriggerDef{ID: "note", Collection: "notes", On: "create", Fields: []FieldRef{{Key: "body", Markdown: true}}}
	raw := map[string]any{"md": "{{body}} / {{fields.answer}}"}
	fields := []DynamicField{{Key: "answer", Value: "*b*"}}
	if err := ExecuteActionContext(context.Background(), app, defs, "notes:write", raw, rule, trigger, rec, 0, fields); err != nil {
		t.Fatal(err)
	}
	if want := "**Steps:**\n- one / \\*b\\*"; got.Params["md"] != want {
		t.Fatalf("markdown param = %q, want %q", got.Params["md"], want)
	}
}
