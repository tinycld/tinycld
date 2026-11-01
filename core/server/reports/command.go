package reports

import (
	"fmt"
	"io"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/spf13/cobra"
)

// NewCommand is `reports` with one subcommand, `reports validate`. It
// checks every saved report against the sources the installed packages
// register, stores the outcome (the hub marks a broken report), and lists
// each broken report with its owner. It exits with an error when a report
// is broken, so a deploy script can stop on it.
//
// PocketBase bootstraps the app before it runs a subcommand. The command
// then applies pending migrations, as `create-owner` does: a source whose
// collection a pending migration adds would otherwise be refused, and every
// report on it listed as broken.
func NewCommand(app *pocketbase.PocketBase) *cobra.Command {
	cmd := &cobra.Command{Use: "reports", Short: "Saved reports"}
	cmd.AddCommand(&cobra.Command{
		Use:          "validate",
		Short:        "List saved reports that name a missing source, link or field, with their owners",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := app.Bootstrap(); err != nil {
				return fmt.Errorf("reports validate: bootstrap: %w", err)
			}
			if err := app.RunAppMigrations(); err != nil {
				return fmt.Errorf("reports validate: run migrations: %w", err)
			}
			return validateSaved(app, cmd.OutOrStdout())
		},
	})
	return cmd
}

// validateSaved checks the sources, then every saved report, and writes
// what it found to out.
func validateSaved(app core.App, out io.Writer) error {
	refusedCount := 0
	if err := CheckSources(app); err != nil {
		refused := Refused(app)
		refusedCount = len(refused)
		for _, key := range sortedKeys(refused) {
			fmt.Fprintf(out, "source %s is refused: %s\n", key, refused[key])
		}
	}
	res, err := CheckSaved(app)
	if err != nil {
		return err
	}
	writeBrokenReports(out, res)
	switch {
	case refusedCount > 0 && len(res.Broken) > 0:
		return fmt.Errorf("%d report sources are refused and %d of %d saved reports are broken", refusedCount, len(res.Broken), res.Checked)
	case refusedCount > 0:
		return fmt.Errorf("%d report sources are refused", refusedCount)
	case len(res.Broken) > 0:
		return fmt.Errorf("%d of %d saved reports are broken", len(res.Broken), res.Checked)
	}
	return nil
}
