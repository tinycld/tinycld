package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

func newFormCmd(d *deps) *cobra.Command {
	c := client.NewLazy(d.resolveClientContext, d.httpClient)
	cmd := &cobra.Command{
		Use:   "form",
		Short: "Manage forms, publish web forms, read submissions and destinations",
		Long:  helpText("group.txt"),
	}
	cmd.AddCommand(
		newFormHostsCmd(c), newFormListCmd(c), newFormViewCmd(c),
		newFormCreateCmd(c), newFormUpdateCmd(c), newFormExportCmd(c),
		newFormArchiveCmd(c, true), newFormArchiveCmd(c, false), newFormRemoveCmd(c),
		newFormFieldsCmd(c), newFormExampleCmd(c),
		newFormPublishCmd(c), newFormUnpublishCmd(c), newFormEmbedCmd(c), newFormRegenerateCmd(c),
		newFormSubmissionCmd(c), newFormDestinationCmd(c),
	)
	return cmd
}

type hostsResponse struct {
	Hosts []struct {
		Key      string   `json:"key"`
		Label    string   `json:"label"`
		Kinds    []string `json:"kinds"`
		Builtins []struct {
			Key      string `json:"key"`
			Label    string `json:"label"`
			Kind     string `json:"kind"`
			Required bool   `json:"required"`
		} `json:"builtins"`
	} `json:"hosts"`
}

func fetchHosts(cmd *cobra.Command, c *client.Client) (hostsResponse, error) {
	var resp hostsResponse
	err := c.GetJSON(cmd.Context(), "/api/forms/hosts", &resp)
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 404 {
		return resp, Failed(errors.New("this server is older than `tinycld form hosts` (it has no /api/forms/hosts); update the server"))
	}
	return resp, explain(err, "")
}

func newFormHostsCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "hosts",
		Short: "List the installed form hosts, their field kinds and built-in fields",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			resp, err := fetchHosts(cmd, c)
			if err != nil {
				return err
			}
			rows := make([][]string, len(resp.Hosts))
			for i, h := range resp.Hosts {
				builtins := make([]string, len(h.Builtins))
				for j, b := range h.Builtins {
					builtins[j] = b.Key
				}
				rows[i] = []string{h.Key, h.Label, strings.Join(h.Kinds, ", "), strings.Join(builtins, ", ")}
			}
			return o.Write(cmd.OutOrStdout(), []string{"KEY", "LABEL", "KINDS", "BUILT-IN FIELDS"}, rows, resp)
		},
	}
}

type formListRow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Slug      string `json:"slug"`
	Archived  bool   `json:"archived"`
	Published bool   `json:"published"`
}

func newFormListCmd(c *client.Client) *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List forms",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			filter := ""
			if host != "" {
				filter = client.Filter("host = {:h}", map[string]any{"h": host})
			}
			forms, err := client.ListAll[formRecord](cmd.Context(), c, "forms", filter, "host,order")
			if err != nil {
				return explain(err, "")
			}
			pubs, err := client.ListAll[publicationRecord](cmd.Context(), c, "form_publications", "", "")
			if err != nil {
				return explain(err, "")
			}
			published := map[string]bool{}
			for _, p := range pubs {
				published[p.Form] = p.Enabled
			}
			out := make([]formListRow, len(forms))
			rows := make([][]string, len(forms))
			for i, f := range forms {
				out[i] = formListRow{f.ID, f.Name, f.Host, f.Slug, f.Archived, published[f.ID]}
				rows[i] = []string{f.ID, f.Name, f.Host, f.Slug, yesNo(f.Archived), yesNo(published[f.ID])}
			}
			return o.Write(cmd.OutOrStdout(), []string{"ID", "NAME", "HOST", "SLUG", "ARCHIVED", "PUBLISHED"}, rows, out)
		},
	}
	cmd.Flags().StringVar(&host, "host", "", "only forms of this host (see `tinycld form hosts`)")
	return cmd
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

var slugInvalid = regexp.MustCompile(`[^a-z0-9_-]+`)

// slugify makes a slug the forms collection accepts: ^[a-z][a-z0-9_-]*$,
// up to 60 characters.
func slugify(name string) string {
	s := strings.Trim(slugInvalid.ReplaceAllString(strings.ToLower(name), "-"), "-_")
	s = strings.TrimLeft(s, "0123456789-_")
	if s == "" {
		s = "form"
	}
	if len(s) > 60 {
		s = strings.TrimRight(s[:60], "-_")
	}
	return s
}

func newFormCreateCmd(c *client.Client) *cobra.Command {
	var from, host, slug string
	cmd := &cobra.Command{
		Use:     "create <name> --from <definition.json>",
		Short:   "Create a form from a definition file",
		Long:    "Create a form of a host (default core:web_form).\n\n" + helpText("definition.txt"),
		Example: "  tinycld form example definition > def.json\n  tinycld form create \"Job applications\" --from def.json",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			if from == "" {
				return Usage(errors.New("--from is required; see `tinycld form create --help`"))
			}
			def, err := readJSONFile(from)
			if err != nil {
				return err
			}
			if slug == "" {
				slug = slugify(args[0])
			}
			siblings, err := client.ListAll[formRecord](cmd.Context(), c, "forms",
				client.Filter("host = {:h}", map[string]any{"h": host}), "")
			if err != nil {
				return explain(err, "")
			}
			created, err := client.CreateRecord[formRecord](cmd.Context(), c, "forms", map[string]any{
				"host": host, "name": args[0], "slug": slug, "definition": def,
				"is_default": false, "archived": false, "order": len(siblings) + 1,
			})
			if err != nil {
				return explain(err, "tinycld form create --help")
			}
			o.Info(cmd.ErrOrStderr(), "created form %s (%s)", created.ID, created.Slug)
			return o.Write(cmd.OutOrStdout(), []string{"ID", "SLUG", "HOST"}, [][]string{{created.ID, created.Slug, created.Host}}, created)
		},
	}
	fl := cmd.Flags()
	fl.StringVar(&from, "from", "", "the definition JSON file")
	fl.StringVar(&host, "host", webFormHost, "the form host (see `tinycld form hosts`)")
	fl.StringVar(&slug, "slug", "", "the form slug (default: made from the name)")
	return cmd
}

func newFormUpdateCmd(c *client.Client) *cobra.Command {
	var from, name string
	cmd := &cobra.Command{
		Use:     "update <form> [--from <definition.json>] [--name <name>]",
		Short:   "Replace a form's definition, or rename it",
		Long:    helpText("definition.txt"),
		Example: "  tinycld form export jobs > def.json\n  tinycld form update jobs --from def.json",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			body := map[string]any{}
			if from != "" {
				def, err := readJSONFile(from)
				if err != nil {
					return err
				}
				body["definition"] = def
			}
			if name != "" {
				body["name"] = name
			}
			if len(body) == 0 {
				return Usage(errors.New("give --from, --name or both"))
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if _, err := client.UpdateRecord[formRecord](cmd.Context(), c, "forms", f.ID, body); err != nil {
				return explain(err, "tinycld form update --help")
			}
			o.Info(cmd.ErrOrStderr(), "updated form %s", f.ID)
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "the definition JSON file")
	cmd.Flags().StringVar(&name, "name", "", "a new name")
	return cmd
}

func newFormExportCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "export <form>",
		Short: "Print a form's definition as JSON",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			return writeIndented(cmd, f.Definition)
		},
	}
}

func writeIndented(cmd *cobra.Command, raw []byte) error {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "    ")
	return enc.Encode(v)
}

func newFormArchiveCmd(c *client.Client, archive bool) *cobra.Command {
	use, short := "archive <form>", "Archive a form (it stays, but is not offered for new records)"
	if !archive {
		use, short = "unarchive <form>", "Make an archived form available again"
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
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if _, err := client.UpdateRecord[formRecord](cmd.Context(), c, "forms", f.ID, map[string]any{"archived": archive}); err != nil {
				return explain(err, "")
			}
			o.Info(cmd.ErrOrStderr(), "%s: archived=%v", f.Name, archive)
			return nil
		},
	}
}

func newFormRemoveCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <form>",
		Aliases: []string{"rm"},
		Short:   "Delete a form. The server refuses a form that records use; archive it instead",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, yes, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			ok, err := ui.Confirm(o, yes, cmd.InOrStdin(), cmd.ErrOrStderr(), fmt.Sprintf("Delete form %q? This cannot be undone.", f.Name))
			if err != nil {
				return Usage(err)
			}
			if !ok {
				return Usage(errors.New("cancelled"))
			}
			if err := client.DeleteRecord(cmd.Context(), c, "forms", f.ID); err != nil {
				return explain(err, "tinycld form archive --help")
			}
			o.Info(cmd.ErrOrStderr(), "deleted form %s", f.ID)
			return nil
		},
	}
}

func newFormFieldsCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "fields <form>",
		Short: "List a form's fields, with the {{fields.<key>}} placeholder of each",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			fields := definitionFields(f)
			rows := make([][]string, len(fields))
			for i, fd := range fields {
				placeholder := ""
				if f.Host == webFormHost {
					placeholder = "{{fields." + fd.Key + "}}"
				}
				rows[i] = []string{fd.Key, fd.Kind, fd.Label, placeholder}
			}
			return o.Write(cmd.OutOrStdout(), []string{"KEY", "KIND", "LABEL", "PLACEHOLDER"}, rows, fields)
		},
	}
}

func newFormExampleCmd(c *client.Client) *cobra.Command {
	var host string
	cmd := &cobra.Command{
		Use:       "example definition|destination",
		Short:     "Print a valid starting file for create or destination add",
		ValidArgs: []string{"definition", "destination"},
		Args:      usageArgs(cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs)),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] == "destination" {
				_, err := cmd.OutOrStdout().Write(exampleFile("destination.json"))
				return err
			}
			if host == webFormHost {
				_, err := cmd.OutOrStdout().Write(exampleFile("definition.json"))
				return err
			}
			// A package host's default form holds every built-in field in
			// its place, and the server has already validated it.
			rows, err := client.ListAll[formRecord](cmd.Context(), c, "forms",
				client.Filter("host = {:h} && is_default = true", map[string]any{"h": host}), "")
			if err != nil {
				return explain(err, "")
			}
			if len(rows) == 0 {
				return Failed(fmt.Errorf("host %q has no default form; see `tinycld form hosts`", host))
			}
			return writeIndented(cmd, rows[0].Definition)
		},
	}
	cmd.Flags().StringVar(&host, "host", webFormHost, "the host to start from (see `tinycld form hosts`)")
	return cmd
}

func newFormViewCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "view <form>",
		Short: "Show a form, its publication and its destinations",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			v := formView{Form: f, Fields: len(definitionFields(f))}
			rows := [][]string{
				{"id", f.ID}, {"name", f.Name}, {"host", f.Host}, {"slug", f.Slug},
				{"archived", yesNo(f.Archived)}, {"fields", strconv.Itoa(v.Fields)},
			}
			if f.Host == webFormHost {
				pub, err := publicationFor(cmd.Context(), c, f.ID)
				if err != nil {
					return err
				}
				dests, err := destinationsOf(cmd.Context(), c, f.ID)
				if err != nil {
					return err
				}
				v.Publication = pub
				if pub != nil {
					v.URL = publicFormURL(c.Origin(), pub.Token)
				}
				for _, dst := range dests {
					v.Destinations = append(v.Destinations, dst.Name)
				}
				rows = append(rows, publicationRows(c, pub)...)
				rows = append(rows, []string{"destinations", strings.Join(v.Destinations, ", ")})
			}
			return o.Write(cmd.OutOrStdout(), []string{"FIELD", "VALUE"}, rows, v)
		},
	}
}

type formView struct {
	Form         formRecord         `json:"form"`
	Fields       int                `json:"fields"`
	URL          string             `json:"url,omitempty"`
	Publication  *publicationRecord `json:"publication,omitempty"`
	Destinations []string           `json:"destinations,omitempty"`
}

func publicationRows(c *client.Client, pub *publicationRecord) [][]string {
	if pub == nil {
		return [][]string{{"published", "no (run: tinycld form publish <form>)"}}
	}
	return [][]string{
		{"published", yesNo(pub.Enabled)},
		{"url", publicFormURL(c.Origin(), pub.Token)},
		{"domains", pub.AllowedDomains},
		{"proof of work", yesNo(pub.RequirePow)},
		{"retention days", strconv.Itoa(pub.RetentionDays)},
		{"max upload MB", strconv.Itoa(pub.MaxUploadMb)},
	}
}
