package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"tinycld.org/cli/client"
)

const (
	webFormHost          = "core:web_form"
	formSubmittedTrigger = "core:form-submitted"
)

type formRecord struct {
	ID         string          `json:"id"`
	Host       string          `json:"host"`
	Name       string          `json:"name"`
	Slug       string          `json:"slug"`
	Definition json.RawMessage `json:"definition"`
	IsDefault  bool            `json:"is_default"`
	Archived   bool            `json:"archived"`
	Order      int             `json:"order"`
}

type publicationRecord struct {
	ID             string `json:"id"`
	Form           string `json:"form"`
	Token          string `json:"token"`
	Enabled        bool   `json:"enabled"`
	AllowedDomains string `json:"allowed_domains"`
	RequirePow     bool   `json:"require_pow"`
	Confirmation   string `json:"confirmation"`
	RetentionDays  int    `json:"retention_days"`
	MaxUploadMb    int    `json:"max_upload_mb"`
}

type submissionRecord struct {
	ID          string         `json:"id"`
	Form        string         `json:"form"`
	Publication string         `json:"publication"`
	Values      map[string]any `json:"values"`
	Created     string         `json:"created"`
}

type submissionFile struct {
	ID         string `json:"id"`
	Submission string `json:"submission"`
	Field      string `json:"field"`
	File       string `json:"file"`
	Size       int64  `json:"size"`
	Name       string `json:"name"`
}

type ruleAction struct {
	Ref    string         `json:"ref"`
	Params map[string]any `json:"params"`
}

type ruleRecord struct {
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Scope         string          `json:"scope"`
	Owner         string          `json:"owner"`
	Trigger       string          `json:"trigger"`
	TriggerConfig map[string]any  `json:"trigger_config"`
	Conditions    json.RawMessage `json:"conditions"`
	Actions       []ruleAction    `json:"actions"`
	Enabled       bool            `json:"enabled"`
}

type ruleRun struct {
	ID       string          `json:"id"`
	Rule     string          `json:"rule"`
	FiredAt  string          `json:"fired_at"`
	Error    string          `json:"error"`
	Results  json.RawMessage `json:"results"`
	RecordID string          `json:"record_id"`
}

type definitionField struct {
	Key   string `json:"key"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

func definitionFields(f formRecord) []definitionField {
	var def struct {
		Fields []definitionField `json:"fields"`
	}
	_ = json.Unmarshal(f.Definition, &def)
	return def.Fields
}

// resolveForm finds a form by id, slug or name. An id match wins; two forms
// that share a slug (on two hosts) or a name are refused with their ids.
func resolveForm(ctx context.Context, c *client.Client, arg string) (formRecord, error) {
	rows, err := client.ListAll[formRecord](ctx, c, "forms",
		client.Filter("id = {:v} || slug = {:v} || name = {:v}", map[string]any{"v": arg}), "")
	if err != nil {
		return formRecord{}, explain(err, "")
	}
	for _, r := range rows {
		if r.ID == arg {
			return r, nil
		}
	}
	switch len(rows) {
	case 0:
		return formRecord{}, Failed(fmt.Errorf("no form %q. List forms with: tinycld form list", arg))
	case 1:
		return rows[0], nil
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID + " (" + r.Host + ")"
	}
	return formRecord{}, Usage(fmt.Errorf("%d forms match %q; use the id: %s", len(rows), arg, strings.Join(ids, ", ")))
}

func requireWebForm(f formRecord) error {
	if f.Host != webFormHost {
		return Usage(fmt.Errorf("only web forms can be published; %q has host %s", f.Name, f.Host))
	}
	return nil
}

// publicationFor is the form's publication, or nil when it was never
// published.
func publicationFor(ctx context.Context, c *client.Client, formID string) (*publicationRecord, error) {
	rows, err := client.ListAll[publicationRecord](ctx, c, "form_publications",
		client.Filter("form = {:f}", map[string]any{"f": formID}), "")
	if err != nil {
		return nil, explain(err, "")
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// destinationsOf lists the organization rules that act on a web form's
// submissions. trigger_config is JSON, so the form is matched here.
func destinationsOf(ctx context.Context, c *client.Client, formID string) ([]ruleRecord, error) {
	rows, err := client.ListAll[ruleRecord](ctx, c, "rules",
		client.Filter("scope = {:s} && trigger = {:t}", map[string]any{"s": "org", "t": formSubmittedTrigger}), "")
	if err != nil {
		return nil, explain(err, "")
	}
	out := rows[:0]
	for _, r := range rows {
		if formID == "" || fmt.Sprint(r.TriggerConfig["form"]) == formID {
			out = append(out, r)
		}
	}
	return out, nil
}
