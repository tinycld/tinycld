package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
	"tinycld.org/cli/ui"
)

func newFormSubmissionCmd(c *client.Client) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "submission",
		Aliases: []string{"submissions"},
		Short:   "Read, export, download, retry and delete web form submissions",
	}
	cmd.AddCommand(newSubmissionListCmd(c), newSubmissionViewCmd(c), newSubmissionDownloadCmd(c),
		newSubmissionRetryCmd(c), newSubmissionRemoveCmd(c))
	return cmd
}

// runsFor reads the runs of the given destinations, grouped by submission.
func runsFor(cmd *cobra.Command, c *client.Client, dests []ruleRecord) (map[string][]ruleRun, error) {
	byRecord := map[string][]ruleRun{}
	for _, dst := range dests {
		runs, err := client.ListAll[ruleRun](cmd.Context(), c, "rule_runs",
			client.Filter("rule = {:r}", map[string]any{"r": dst.ID}), "")
		if err != nil {
			return nil, explain(err, "")
		}
		for _, r := range runs {
			byRecord[r.RecordID] = append(byRecord[r.RecordID], r)
		}
	}
	return byRecord, nil
}

func ruleIDs(dests []ruleRecord) []string {
	ids := make([]string, len(dests))
	for i, dst := range dests {
		ids[i] = dst.ID
	}
	return ids
}

type submissionRow struct {
	ID        string         `json:"id"`
	Sent      string         `json:"sent"`
	Values    map[string]any `json:"values"`
	Delivered int            `json:"delivered"`
	Total     int            `json:"total"`
}

// firstAnswers is the first two text answers, in the form's field order.
func firstAnswers(f formRecord, values map[string]any) string {
	var out []string
	for _, fd := range definitionFields(f) {
		if s, ok := values[fd.Key].(string); ok && s != "" {
			out = append(out, s)
			if len(out) == 2 {
				break
			}
		}
	}
	return strings.Join(out, " · ")
}

func newSubmissionListCmd(c *client.Client) *cobra.Command {
	var since string
	var failed bool
	cmd := &cobra.Command{
		Use:     "list <form> [--since YYYY-MM-DD] [--failed]",
		Aliases: []string{"ls"},
		Short:   "List a web form's submissions, newest first, with their delivery",
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
			filter := client.Filter("form = {:f}", map[string]any{"f": f.ID})
			if since != "" {
				filter += " && " + client.Filter("created >= {:s}", map[string]any{"s": since})
			}
			subs, err := client.ListAll[submissionRecord](cmd.Context(), c, "form_submissions", filter, "-created")
			if err != nil {
				return explain(err, "")
			}
			dests, err := destinationsOf(cmd.Context(), c, f.ID)
			if err != nil {
				return err
			}
			runs, err := runsFor(cmd, c, dests)
			if err != nil {
				return err
			}
			var out []submissionRow
			var rows [][]string
			for _, s := range subs {
				delivered, total := deliveryStatus(ruleIDs(dests), runs[s.ID])
				if failed && delivered == total {
					continue
				}
				out = append(out, submissionRow{s.ID, s.Created, s.Values, delivered, total})
				rows = append(rows, []string{s.ID, s.Created, firstAnswers(f, s.Values), fmt.Sprintf("%d of %d", delivered, total)})
			}
			return o.Write(cmd.OutOrStdout(), []string{"ID", "SENT", "ANSWERS", "DELIVERED"}, rows, out)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "only submissions sent on or after this date (YYYY-MM-DD)")
	cmd.Flags().BoolVar(&failed, "failed", false, "only submissions that a destination did not deliver")
	return cmd
}

func loadSubmission(cmd *cobra.Command, c *client.Client, id string) (submissionRecord, formRecord, []ruleRecord, []ruleRun, error) {
	s, err := client.GetRecord[submissionRecord](cmd.Context(), c, "form_submissions", id)
	if err != nil {
		return s, formRecord{}, nil, nil, explain(err, "")
	}
	f, err := client.GetRecord[formRecord](cmd.Context(), c, "forms", s.Form)
	if err != nil {
		return s, f, nil, nil, explain(err, "")
	}
	dests, err := destinationsOf(cmd.Context(), c, f.ID)
	if err != nil {
		return s, f, nil, nil, err
	}
	runs, err := client.ListAll[ruleRun](cmd.Context(), c, "rule_runs",
		client.Filter("record_id = {:r}", map[string]any{"r": s.ID}), "")
	if err != nil {
		return s, f, nil, nil, explain(err, "")
	}
	return s, f, dests, runs, nil
}

func submissionFiles(cmd *cobra.Command, c *client.Client, id string) ([]submissionFile, error) {
	files, err := client.ListAll[submissionFile](cmd.Context(), c, "form_submission_files",
		client.Filter("submission = {:s}", map[string]any{"s": id}), "")
	return files, explain(err, "")
}

type destinationRun struct {
	Rule   string `json:"rule"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

func newSubmissionViewCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:   "view <submission id>",
		Short: "Show a submission's answers, files and each destination's last run",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			s, f, dests, runs, err := loadSubmission(cmd, c, args[0])
			if err != nil {
				return err
			}
			files, err := submissionFiles(cmd, c, s.ID)
			if err != nil {
				return err
			}
			rows := [][]string{{"id", s.ID}, {"form", f.Name}, {"sent", s.Created}}
			for _, fd := range definitionFields(f) {
				if v, ok := s.Values[fd.Key]; ok {
					rows = append(rows, []string{fd.Label, fmt.Sprint(v)})
				}
			}
			for _, file := range files {
				rows = append(rows, []string{"file", fmt.Sprintf("%s (%d bytes)", file.Name, file.Size)})
			}
			var delivery []destinationRun
			for _, dst := range dests {
				dr := destinationRun{Rule: dst.ID, Name: dst.Name, Status: "no run"}
				if run := newestRun(dst.ID, runs); run != nil {
					dr.Status, dr.Error = "delivered", run.Error
					if !runSucceeded(*run) {
						dr.Status = "failed"
					}
				}
				delivery = append(delivery, dr)
				rows = append(rows, []string{"destination " + dst.Name, strings.TrimSpace(dr.Status + " " + dr.Error)})
			}
			raw := struct {
				Submission   submissionRecord `json:"submission"`
				Files        []submissionFile `json:"files"`
				Destinations []destinationRun `json:"destinations"`
			}{s, files, delivery}
			return o.Write(cmd.OutOrStdout(), []string{"FIELD", "VALUE"}, rows, raw)
		},
	}
}

// uniquePath gives name a " (2)", " (3)" … suffix before its extension when
// the directory already holds a file of that name.
func uniquePath(dir, name string) string {
	path := filepath.Join(dir, name)
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 2; ; n++ {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return path
		}
		path = filepath.Join(dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
	}
}

func newSubmissionDownloadCmd(c *client.Client) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "download <submission id> [--dir <dir>]",
		Short: "Download a submission's files",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			files, err := submissionFiles(cmd, c, args[0])
			if err != nil {
				return err
			}
			if len(files) == 0 {
				o.Info(cmd.ErrOrStderr(), "submission %s has no files", args[0])
				return nil
			}
			var written []string
			for _, file := range files {
				dest := uniquePath(dir, client.LocalFileName(file.Name))
				if err := client.DownloadToFile(cmd.Context(), c, client.FileURL("form_submission_files", file.ID, file.File), dest, nil); err != nil {
					return explain(err, "")
				}
				written = append(written, dest)
			}
			rows := make([][]string, len(written))
			for i, w := range written {
				rows[i] = []string{w}
			}
			return o.Write(cmd.OutOrStdout(), []string{"FILE"}, rows, written)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "the directory to write the files to")
	return cmd
}

func newSubmissionRetryCmd(c *client.Client) *cobra.Command {
	var only string
	cmd := &cobra.Command{
		Use:   "retry <submission id> [--destination <rule>]",
		Short: "Run the failed destinations again for a submission, or one destination",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, _, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			s, _, dests, runs, err := loadSubmission(cmd, c, args[0])
			if err != nil {
				return err
			}
			var targets []ruleRecord
			for _, dst := range dests {
				if only != "" {
					if dst.ID == only || dst.Name == only {
						targets = append(targets, dst)
					}
					continue
				}
				if run := newestRun(dst.ID, runs); run != nil && !runSucceeded(*run) {
					targets = append(targets, dst)
				}
			}
			if only != "" && len(targets) == 0 {
				return Failed(fmt.Errorf("no destination %q for this form; list them with: tinycld form destination list <form>", only))
			}
			sort.Slice(targets, func(i, j int) bool { return targets[i].Name < targets[j].Name })
			var rows [][]string
			for _, dst := range targets {
				path := "/api/automation/rules/" + dst.ID + "/records/" + s.ID + "/run"
				if err := c.PostJSON(cmd.Context(), path, map[string]any{}, nil); err != nil {
					return explain(err, "")
				}
				rows = append(rows, []string{dst.ID, dst.Name, "retried"})
			}
			if len(rows) == 0 {
				o.Info(cmd.ErrOrStderr(), "no failed destination to retry")
			}
			return o.Write(cmd.OutOrStdout(), []string{"RULE", "NAME", "RESULT"}, rows, rows)
		},
	}
	cmd.Flags().StringVar(&only, "destination", "", "retry this destination (id or name) even if it did not fail")
	return cmd
}

func newSubmissionRemoveCmd(c *client.Client) *cobra.Command {
	return &cobra.Command{
		Use:     "remove <submission id>",
		Aliases: []string{"rm"},
		Short:   "Delete a submission and its files",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(cmd *cobra.Command, args []string) error {
			o, yes, err := output.FromCommand(cmd)
			if err != nil {
				return err
			}
			ok, err := ui.Confirm(o, yes, cmd.InOrStdin(), cmd.ErrOrStderr(), "Delete submission "+args[0]+" and its files? This cannot be undone.")
			if err != nil {
				return Usage(err)
			}
			if !ok {
				return Usage(errors.New("cancelled"))
			}
			if err := client.DeleteRecord(cmd.Context(), c, "form_submissions", args[0]); err != nil {
				return explain(err, "")
			}
			o.Info(cmd.ErrOrStderr(), "deleted submission %s", args[0])
			return nil
		},
	}
}
