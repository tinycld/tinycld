package main

import (
	"github.com/spf13/cobra"

	"tinycld.org/cli/output"
)

// snapshotRow mirrors one entry of GET /api/org-backups/snapshots — a
// snapshot held in the configured backup repository, keyed by its ref rather
// than a ledger id.
type snapshotRow struct {
	Ref     string `json:"ref"`
	Created string `json:"created"`
	Bytes   int64  `json:"bytes"`
}

func newBackupSnapshotsCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "snapshots",
		Short: "List the snapshots in the configured backup repository",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, _, err := d.apiClient()
			if err != nil {
				return err
			}
			var rows []snapshotRow
			if err := c.GetJSON(cmd.Context(), backupsPath+"/snapshots", &rows); err != nil {
				return Failed(err)
			}
			table := make([][]string, 0, len(rows))
			for _, r := range rows {
				table = append(table, []string{r.Ref, r.Created, output.FormatBytes(r.Bytes)})
			}
			return d.out.Write(d.stdout, []string{"REF", "CREATED", "SIZE"}, table, rows)
		},
	}
}
