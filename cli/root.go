package main

import (
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"tinycld.org/cli/internal/config"
	"tinycld.org/cli/internal/keychain"
	"tinycld.org/cli/output"
)

// deps is the wiring seam: production values come from defaultDeps, tests
// substitute buffers, a temp config dir, and a memory keychain.
type deps struct {
	configDir  string
	stdout     io.Writer
	stderr     io.Writer
	httpClient *http.Client
	isTTY      bool

	openStore func(configDir string, warn io.Writer) keychain.Store
	// sleep is nil in production (real time.Sleep); tests inject a no-op so
	// device-flow polling doesn't stall the suite.
	sleep func(time.Duration)
	// slugs overrides the generated searchable-package list. Nil in
	// production, where searchSlugs() falls back to the generated var; tests
	// set it so they do not depend on which packages this checkout assembled.
	slugs []string

	// resolved in the root PersistentPreRunE
	out     output.Options
	ctxFlag string
	yes     bool

	storeOnce sync.Once
	memoStore keychain.Store
}

func defaultDeps() *deps {
	return &deps{
		configDir:  config.Dir(),
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		isTTY:      term.IsTerminal(int(os.Stdout.Fd())),
		openStore:  keychain.Open,
	}
}

// store opens the keychain lazily so `--help`, `version`, and `context list`
// never touch (or prompt for) the OS keychain.
func (d *deps) store() keychain.Store {
	d.storeOnce.Do(func() { d.memoStore = d.openStore(d.configDir, d.stderr) })
	return d.memoStore
}

func (d *deps) loadConfig() (*config.Config, error) {
	return config.Load(d.configDir)
}

// searchSlugs returns the packages this build can search. The generated list
// (cli/search_slugs.go) reflects the assembly the binary was built from and is
// empty in a bare checkout, so tests override it rather than depending on
// which packages happen to be present.
func (d *deps) searchSlugs() []string {
	if d.slugs != nil {
		return d.slugs
	}
	return searchSlugs
}

func newRootCmd(d *deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "tinycld",
		Short:         "Command-line access to your TinyCld server",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(d.stdout)
	root.SetErr(d.stderr)

	pf := root.PersistentFlags()
	pf.String("output", string(output.Table), "output format: table|json|csv")
	pf.Bool("json", false, "shorthand for --output json")
	pf.StringVar(&d.ctxFlag, "context", "", "context (server) to use")
	pf.Bool("quiet", false, "suppress informational messages")
	pf.Bool("no-color", false, "disable colored output")
	pf.BoolVar(&d.yes, "yes", false, "answer yes to prompts and skip interactive input")

	root.PersistentPreRunE = func(cmd *cobra.Command, _ []string) error {
		formatFlag, _ := cmd.Flags().GetString("output")
		format, err := output.ParseFormat(formatFlag)
		if err != nil {
			return err
		}
		if jsonFlag, _ := cmd.Flags().GetBool("json"); jsonFlag {
			format = output.JSON
		}
		quiet, _ := cmd.Flags().GetBool("quiet")
		noColor, _ := cmd.Flags().GetBool("no-color")
		d.out = output.Options{
			Format:  format,
			Quiet:   quiet,
			NoColor: noColor || !d.isTTY,
			TTY:     d.isTTY,
		}
		return nil
	}

	root.AddCommand(
		newVersionCmd(d),
		newContextCmd(d),
		newAuthCmd(d),
		newSearchCmd(d),
	)
	return root
}
