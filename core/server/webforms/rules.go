package webforms

import (
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"
	"tinycld.org/core/automation"
	"tinycld.org/core/forms"
	"tinycld.org/core/notify"
	"tinycld.org/core/ratelimit"
)

const FormSubmittedTrigger = "core:form-submitted"

func configForm(config map[string]any) string {
	s, _ := config["form"].(string)
	return s
}

// formRuleWants: a rule names its form in trigger_config; it wants only that
// form's submissions. A rule with no form wants none.
func formRuleWants(_ core.App, config map[string]any, record *core.Record) bool {
	form := configForm(config)
	return form != "" && form == record.GetString("form")
}

var kindTypes = map[string]string{
	"text": "text", "textarea": "text", "number": "number", "computed": "number",
	"toggle": "boolean", "date": "date", "select": "select", "file": "text",
}

// formFields exposes each answer of the rule's form as fields.<key>. A file
// field gives its file names; checklists and tables give no field.
func formFields(app core.App, config map[string]any, record *core.Record) []automation.DynamicField {
	form, err := app.FindRecordById("forms", configForm(config))
	if err != nil {
		return nil
	}
	def, err := forms.ParseDefinition([]byte(form.GetString("definition")))
	if err != nil {
		return nil
	}
	values := map[string]any{}
	_ = record.UnmarshalJSONField("values", &values)
	names := fileNames(app, record)
	out := []automation.DynamicField{}
	for _, f := range def.Fields {
		typ, ok := kindTypes[f.Kind]
		if !ok {
			continue
		}
		d := automation.DynamicField{Key: f.Key, Label: f.Label, Type: typ, Value: values[f.Key]}
		if f.Kind == "file" {
			d.Value = strings.Join(names[f.Key], ", ")
		}
		for _, o := range f.Options {
			d.Options = append(d.Options, o.Value)
		}
		out = append(out, d)
	}
	return out
}

func fileNames(app core.App, record *core.Record) map[string][]string {
	files, err := automation.TriggerFiles(app, record)
	if err != nil {
		return nil
	}
	out := map[string][]string{}
	for _, f := range files {
		out[f.Field] = append(out[f.Field], f.Name)
	}
	return out
}

func registerRules() {
	automation.RegisterRuleFilter(FormSubmittedTrigger, formRuleWants)
	automation.RegisterFieldsResolver(FormSubmittedTrigger, formFields)
	automation.RegisterRunObserver(FormSubmittedTrigger, onFormRun)
}

var failureNotices = ratelimit.NewPerApp("webforms.failureNotice", 1, 24*time.Hour)

// onFormRun tells the admins, at most once a day for each rule, that a
// destination failed; the Submissions tab shows which submissions.
func onFormRun(app core.App, rule, record *core.Record, outcome automation.RunOutcome) {
	if !outcome.Failed() || record == nil || !failureNotices.For(app).Allow(rule.Id) {
		return
	}
	formName := record.GetString("form")
	if form, err := app.FindRecordById("forms", record.GetString("form")); err == nil {
		formName = form.GetString("name")
	}
	_, err := notify.Administrators(app, notify.NotifyParams{
		Type:    "core.web_form.delivery_failed",
		Package: "core",
		Title:   "A web form destination failed",
		Body:    rule.GetString("name") + " could not deliver a submission of " + formName + ".",
		URL:     "/settings/forms/" + record.GetString("form"),
	})
	if err != nil {
		log.Warn("notify admins of a failed web form destination", "rule", rule.Id, "err", err)
	}
}
