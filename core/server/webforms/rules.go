package webforms

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"

	validation "github.com/pocketbase/ozzo-validation/v4"
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

// formScope limits a dry run to the rule's form; a rule with no form reads
// nothing.
func formScope(config map[string]any) (string, map[string]any) {
	form := configForm(config)
	if form == "" {
		return "id = ''", nil
	}
	return "form = {:scopeForm}", map[string]any{"scopeForm": form}
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
	automation.RegisterRuleScope(FormSubmittedTrigger, formScope)
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

var formFieldInTo = regexp.MustCompile(`\{\{\s*fields\.`)

// sendsToFormAddress reports an action list with a recipient param (an
// email's "to", or any package action that declares one) filled from the
// form: without proof of work, a bot could make the server mail any address.
func sendsToFormAddress(app core.App, rule *core.Record) bool {
	var actions []struct {
		Ref    string         `json:"ref"`
		Params map[string]any `json:"params"`
	}
	b, _ := json.Marshal(rule.Get("actions"))
	if json.Unmarshal(b, &actions) != nil {
		return false
	}
	for _, a := range actions {
		for _, key := range automation.RecipientParams(app, a.Ref) {
			to, _ := a.Params[key].(string)
			if formFieldInTo.MatchString(to) {
				return true
			}
		}
	}
	return false
}

func formRequiresPow(app core.App, formID string) bool {
	pub, err := app.FindFirstRecordByData(publicationsName, "form", formID)
	return err == nil && pub.GetBool("require_pow")
}

func checkAutoReply(app core.App, rule *core.Record) error {
	if rule.GetString("trigger") != FormSubmittedTrigger || !rule.GetBool("enabled") || !sendsToFormAddress(app, rule) {
		return nil
	}
	if formRequiresPow(app, configFormOf(rule)) {
		return nil
	}
	return validation.Errors{"actions": validation.NewError("validation_unsafe_auto_reply",
		`An email to an address from the form needs "Ask the browser to prove it is not a bot" on the form's Publish tab.`)}
}

func configFormOf(rule *core.Record) string {
	var cfg map[string]any
	b, _ := json.Marshal(rule.Get("trigger_config"))
	_ = json.Unmarshal(b, &cfg)
	return configForm(cfg)
}

func checkPowStillNeeded(app core.App, pub *core.Record) error {
	if pub.GetBool("require_pow") {
		return nil
	}
	rules, err := app.FindRecordsByFilter("rules", "trigger = {:t} && enabled = true", "", 0, 0, map[string]any{"t": FormSubmittedTrigger})
	if err != nil {
		return nil
	}
	for _, r := range rules {
		if configFormOf(r) == pub.GetString("form") && sendsToFormAddress(app, r) {
			return validation.Errors{"require_pow": validation.NewError("validation_auto_reply_needs_pow",
				"A rule sends email to an address from this form. Turn that rule off first.")}
		}
	}
	return nil
}

func bindSafetyHooks(app core.App) {
	app.OnRecordCreate("rules").BindFunc(func(e *core.RecordEvent) error {
		if err := checkAutoReply(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordUpdate("rules").BindFunc(func(e *core.RecordEvent) error {
		if err := checkAutoReply(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
	app.OnRecordUpdate(publicationsName).BindFunc(func(e *core.RecordEvent) error {
		if err := checkPowStillNeeded(e.App, e.Record); err != nil {
			return err
		}
		return e.Next()
	})
}
