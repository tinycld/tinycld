package main

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/client"
	"tinycld.org/cli/output"
)

func newBackupListCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List backup and restore runs",
		Long: "Shows the ledger: every backup and restore this deployment has run,\n" +
			"newest first, with its outcome.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := d.apiClient()
			if err != nil {
				return err
			}
			rows, err := client.ListAll[ledgerRow](cmd.Context(), c, "backups", "", "-started")
			if err != nil {
				return Failed(err)
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{
					r.ID, r.Kind, r.Status, r.Started, output.FormatBytes(r.Bytes), r.TargetHost, r.Error,
				})
			}
			return d.out.Write(d.stdout,
				[]string{"ID", "KIND", "STATUS", "STARTED", "SIZE", "TARGET", "ERROR"}, table, rows)
		},
	}
}
