package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

func newFormDestinationCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "destination",
		Aliases: []string{"destinations"},
		Short:   "Manage what happens to each submission of a web form",
		Long:    helpText("destination.txt"),
	}
	cmd.AddCommand(newDestinationListCmd(c), newDestinationActionsCmd(c), newDestinationAddCmd(c),
		newDestinationUpdateCmd(c), newDestinationToggleCmd(c, true), newDestinationToggleCmd(c, false),
		newDestinationRemoveCmd(c))
	return cmd
}

// resolveDestination finds a web form destination by id or name.
func resolveDestination(cmd *cobra.Command, c *client.Client, arg string) (ruleRecord, error) {
	all, err := destinationsOf(cmd.Context(), c, "")
	if err != nil {
		return ruleRecord{}, err
	}
	var found []ruleRecord
	for _, r := range all {
		if r.ID == arg {
			return r, nil
		}
		if r.Name == arg {
			found = append(found, r)
		}
	}
	switch len(found) {
	case 0:
		return ruleRecord{}, Failed(fmt.Errorf("no web form destination %q; list them with: tinycld form destination list <form>", arg))
	case 1:
		return found[0], nil
	}
	ids := make([]string, len(found))
	for i, r := range found {
		ids[i] = r.ID
	}
	return ruleRecord{}, Usage(fmt.Errorf("%d destinations are named %q; use the id: %s", len(found), arg, strings.Join(ids, ", ")))
}

func actionRefs(r ruleRecord) string {
	refs := make([]string, len(r.Actions))
	for i, a := range r.Actions {
		refs[i] = a.Ref
	}
	return strings.Join(refs, ", ")
}

func newDestinationListCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:     "list <form>",
		Aliases: []string{"ls"},
		Short:   "List a web form's destinations and each one's last run",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := requireWebForm(f); err != nil {
				return err
			}
			dests, err := destinationsOf(cmd.Context(), c, f.ID)
			if err != nil {
				return err
			}
			rows := make([][]string, len(dests))
			for i, dst := range dests {
				last := "never"
				res, err := client.ListRecords[ruleRun](cmd.Context(), c, "rule_runs", client.ListOptions{
					Filter: client.Filter("rule = {:r}", map[string]any{"r": dst.ID}), Sort: "-fired_at", PerPage: 1, SkipTotal: true})
				if err != nil {
					return explain(err, "")
				}
				if len(res.Items) > 0 {
					last = res.Items[0].FiredAt + " ok"
					if !runSucceeded(res.Items[0]) {
						last = res.Items[0].FiredAt + " failed: " + res.Items[0].Error
					}
				}
				rows[i] = []string{dst.ID, dst.Name, yesNo(dst.Enabled), actionRefs(dst), last}
			}
			return o.Write(cmd.OutOrStdout(), []string{"ID", "NAME", "ON", "ACTIONS", "LAST RUN"}, rows, dests)
		},
	}
}

type catalogParam struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Template bool   `json:"template"`
	Field    struct {
		Type           string   `json:"type"`
		Options        []string `json:"options,omitempty"`
		RelationTarget string   `json:"relationTarget,omitempty"`
	} `json:"field"`
}

type catalogRow struct {
	Ref        string `json:"ref"`
	Label      string `json:"label"`
	Definition struct {
		Params []catalogParam `json:"params"`
	} `json:"definition"`
}

func newDestinationActionsCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "actions",
		Short: "List the actions a destination can run, with their params (* = required)",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			rows, err := client.ListAll[catalogRow](cmd.Context(), c, "automation_catalog",
				client.Filter("kind = {:k} && available = true", map[string]any{"k": "action"}), "ref")
			if err != nil {
				return explain(err, "")
			}
			table := make([][]string, len(rows))
			for i, r := range rows {
				params := make([]string, len(r.Definition.Params))
				for j, p := range r.Definition.Params {
					s := p.Key + ":" + p.Field.Type
					if p.Field.RelationTarget != "" {
						s += "(" + p.Field.RelationTarget + ")"
					}
					if len(p.Field.Options) > 0 {
						s += "[" + strings.Join(p.Field.Options, "|") + "]"
					}
					if p.Required {
						s += "*"
					}
					params[j] = s
				}
				table[i] = []string{r.Ref, r.Label, strings.Join(params, " ")}
			}
			return o.Write(cmd.OutOrStdout(), []string{"REF", "LABEL", "PARAMS"}, table, rows)
		},
	}
}

type destinationFile struct {
	Actions    json.RawMessage `json:"actions"`
	Conditions json.RawMessage `json:"conditions,omitempty"`
}

func readDestinationFile(path string) (destinationFile, error) {
	raw, err := readJSONFile(path)
	if err != nil {
		return destinationFile{}, err
	}
	var file destinationFile
	if err := json.Unmarshal(raw, &file); err != nil || len(file.Actions) == 0 {
		return file, Usage(fmt.Errorf(`%s: needs {"actions": [...]}; see: tinycld form destination add --help`, path))
	}
	return file, nil
}

const destinationHint = "tinycld form destination actions, and tinycld form destination add --help"

func newDestinationAddCmd(c *client.Client) *cobra.Command {
	var from, name string
	cmd := &cobra.Command{
		Use:     "add <form> --from <rule.json> [--name <name>]",
		Short:   "Add a destination: what to do with each submission",
		Long:    helpText("destination.txt"),
		Example: "  tinycld form example destination > rule.json\n  tinycld form destination add jobs --from rule.json",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			if from == "" {
				return Usage(errors.New("--from is required; see `tinycld form destination add --help`"))
			}
			file, err := readDestinationFile(from)
			if err != nil {
				return err
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := requireWebForm(f); err != nil {
				return err
			}
			var me struct {
				Sub string `json:"sub"`
			}
			if err := c.GetJSON(cmd.Context(), "/oauth/userinfo", &me); err != nil {
				return explain(err, "")
			}
			if name == "" {
				name = f.Name + " destination"
			}
			body := map[string]any{
				"name": name, "scope": "org", "owner": me.Sub,
				"trigger": formSubmittedTrigger, "trigger_config": map[string]any{"form": f.ID},
				"actions": file.Actions, "enabled": true,
			}
			if len(file.Conditions) > 0 {
				body["conditions"] = file.Conditions
			}
			created, err := client.CreateRecord[ruleRecord](cmd.Context(), c, "rules", body)
			if err != nil {
				return explain(err, destinationHint)
			}
			o.Info(cmd.ErrOrStderr(), "added destination %s to %s", created.ID, f.Name)
			return o.Write(cmd.OutOrStdout(), []string{"ID", "NAME"}, [][]string{{created.ID, created.Name}}, created)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "the destination JSON file")
	cmd.Flags().StringVar(&name, "name", "", "the destination's name (default: <form name> destination)")
	return cmd
}

func newDestinationUpdateCmd(c *client.Client) *cobra.Command {
	var from string
	cmd := &cobra.Command{
		Use:   "update <rule> --from <rule.json>",
		Short: "Replace a destination's actions and conditions",
		Long:  helpText("destination.txt"),
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			if from == "" {
				return Usage(errors.New("--from is required"))
			}
			file, err := readDestinationFile(from)
			if err != nil {
				return err
			}
			r, err := resolveDestination(cmd, c, args[0])
			if err != nil {
				return err
			}
			body := map[string]any{"actions": file.Actions, "conditions": nil}
			if len(file.Conditions) > 0 {
				body["conditions"] = file.Conditions
			}
			if _, err := client.UpdateRecord[ruleRecord](cmd.Context(), c, "rules", r.ID, body); err != nil {
				return explain(err, destinationHint)
			}
			o.Info(cmd.ErrOrStderr(), "updated destination %s", r.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "the destination JSON file")
	return cmd
}

func newDestinationToggleCmd(c *client.Client, on bool) *cobra.Command {
	use, short := "enable <rule>", "Turn a destination on"
	if !on {
		use, short = "disable <rule>", "Turn a destination off"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			r, err := resolveDestination(cmd, c, args[0])
			if err != nil {
				return err
			}
			if _, err := client.UpdateRecord[ruleRecord](cmd.Context(), c, "rules", r.ID, map[string]any{"enabled": on}); err != nil {
				return explain(err, "")
			}
			o.Info(cmd.ErrOrStderr(), "%s: enabled=%v", r.Name, on)
			return nil
		},
	}
}

func newDestinationRemoveCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <rule>",
		Aliases: []string{"rm"},
		Short:   "Delete a destination",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, yes, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			r, err := resolveDestination(cmd, c, args[0])
			if err != nil {
				return err
			}
			ok, err := ui.Confirm(o, yes, cmd.InOrStdin(), cmd.ErrOrStderr(), fmt.Sprintf("Delete destination %q?", r.Name))
			if err != nil {
				return Usage(err)
			}
			if !ok {
				return Usage(errors.New("cancelled"))
			}
			if err := client.DeleteRecord(cmd.Context(), c, "rules", r.ID); err != nil {
				return explain(err, "")
			}
			o.Info(cmd.ErrOrStderr(), "deleted destination %s", r.ID)
			return nil
		},
	}
}
