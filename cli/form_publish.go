package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

func newFormPublishCmd(c *client.Client) *cobra.Command {
	var domains []string
	var pow, noPow bool
	var confirmationFile string
	var retention, maxUpload int
	cmd := &cobra.Command{
		Use:     "publish <form>",
		Short:   "Turn a web form on for visitors, and set how it is published",
		Long:    helpText("publish.txt"),
		Example: "  tinycld form publish jobs --domain https://example.com --pow",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			if pow && noPow {
				return Usage(errors.New("give --pow or --no-pow, not both"))
			}
			body := map[string]any{"enabled": true}
			fl := cmd.Flags()
			if fl.Changed("domain") {
				body["allowed_domains"] = joinOrigins(domains)
			}
			if pow {
				body["require_pow"] = true
			}
			if noPow {
				body["require_pow"] = false
			}
			if confirmationFile != "" {
				b, err := os.ReadFile(confirmationFile)
				if err != nil {
					return Usage(err)
				}
				body["confirmation"] = string(b)
			}
			if fl.Changed("retention-days") {
				body["retention_days"] = retention
			}
			if fl.Changed("max-upload-mb") {
				body["max_upload_mb"] = maxUpload
			}
			f, err := resolveForm(cmd.Context(), c, args[0])
			if err != nil {
				return err
			}
			if err := requireWebForm(f); err != nil {
				return err
			}
			pub, err := publicationFor(cmd.Context(), c, f.ID)
			if err != nil {
				return err
			}
			var saved publicationRecord
			if pub == nil {
				body["form"] = f.ID
				saved, err = client.CreateRecord[publicationRecord](cmd.Context(), c, "form_publications", body)
			} else {
				saved, err = client.UpdateRecord[publicationRecord](cmd.Context(), c, "form_publications", pub.ID, body)
			}
			if err != nil {
				return explain(err, "tinycld form publish --help")
			}
			o.Info(cmd.ErrOrStderr(), "%s is published. Embed code: tinycld form embed %s", f.Name, f.Slug)
			url := publicFormURL(c.Origin(), saved.Token)
			return o.Write(cmd.OutOrStdout(), []string{"URL"}, [][]string{{url}}, map[string]string{"url": url})
		},
	}
	fl := cmd.Flags()
	fl.StringSliceVar(&domains, "domain", nil, "a website that may embed the form (repeat or comma-separate)")
	fl.BoolVar(&pow, "pow", false, "ask the browser to prove it is not a bot")
	fl.BoolVar(&noPow, "no-pow", false, "do not ask for proof of work")
	fl.StringVar(&confirmationFile, "confirmation-file", "", "markdown file with the message shown after a submission")
	fl.IntVar(&retention, "retention-days", 0, "keep submissions this many days (1 to 3650)")
	fl.IntVar(&maxUpload, "max-upload-mb", 0, "largest upload per submission in MB (1 to 100)")
	return cmd
}

// joinOrigins turns the --domain values into the space-separated list the
// server parses (embedpolicy.ParseOrigins).
func joinOrigins(values []string) string {
	var out []string
	for _, v := range values {
		for _, part := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' }) {
			out = append(out, part)
		}
	}
	return strings.Join(out, " ")
}

func newFormUnpublishCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "unpublish <form>",
		Short: "Stop a web form taking submissions. The link and embed code stay the same",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, pub, err := webPublication(cmd, c, args[0])
			if err != nil {
				return err
			}
			if _, err := client.UpdateRecord[publicationRecord](cmd.Context(), c, "form_publications", pub.ID, map[string]any{"enabled": false}); err != nil {
				return explain(err, "")
			}
			o.Info(cmd.ErrOrStderr(), "%s no longer takes submissions", f.Name)
			return nil
		},
	}
}

// webPublication resolves a web form and its publication, refusing a form
// that was never published.
func webPublication(cmd *cobra.Command, c *client.Client, arg string) (formRecord, *publicationRecord, error) {
	f, err := resolveForm(cmd.Context(), c, arg)
	if err != nil {
		return f, nil, err
	}
	if err := requireWebForm(f); err != nil {
		return f, nil, err
	}
	pub, err := publicationFor(cmd.Context(), c, f.ID)
	if err != nil {
		return f, nil, err
	}
	if pub == nil {
		return f, nil, Failed(fmt.Errorf("%s was never published; run: tinycld form publish %s", f.Name, f.Slug))
	}
	return f, pub, nil
}

func newFormEmbedCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "embed <form>",
		Short: "Print the code to paste into a website",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, pub, err := webPublication(cmd, c, args[0])
			if err != nil {
				return err
			}
			if !pub.Enabled {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s does not take submissions now; run: tinycld form publish %s\n", f.Name, f.Slug)
			}
			code := embedCode(c.Origin(), pub.Token, f.Name)
			if o.Format == output.JSON {
				return o.Write(cmd.OutOrStdout(), nil, nil, map[string]string{"url": publicFormURL(c.Origin(), pub.Token), "code": code})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), code)
			return err
		},
	}
}

func newFormRegenerateCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "regenerate-link <form>",
		Short: "Give a web form a new link. The OLD LINK AND EVERY EMBED OF IT STOP WORKING",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, yes, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			f, pub, err := webPublication(cmd, c, args[0])
			if err != nil {
				return err
			}
			ok, err := ui.Confirm(o, yes, cmd.InOrStdin(), cmd.ErrOrStderr(),
				fmt.Sprintf("Give %s a new link? Every website that embeds the old one stops working.", f.Name))
			if err != nil {
				return Usage(err)
			}
			if !ok {
				return Usage(errors.New("cancelled"))
			}
			saved, err := client.UpdateRecord[publicationRecord](cmd.Context(), c, "form_publications", pub.ID, map[string]any{"token": ""})
			if err != nil {
				return explain(err, "")
			}
			url := publicFormURL(c.Origin(), saved.Token)
			return o.Write(cmd.OutOrStdout(), []string{"URL"}, [][]string{{url}}, map[string]string{"url": url})
		},
	}
}
