package main

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"tinycld.org/cli/internal/config"
)

type contextRow struct {
	Name    string `json:"name"`
	Origin  string `json:"origin"`
	User    string `json:"user,omitempty"`
	Current bool   `json:"current"`
}

func newContextCmd(d *deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context",
		Short: "Manage saved servers (contexts)",
	}
	cmd.AddCommand(
		newContextListCmd(d),
		newContextUseCmd(d),
		newContextAddCmd(d),
		newContextRemoveCmd(d),
	)
	return cmd
}

func newContextListCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List saved contexts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := d.loadConfig()
			if err != nil {
				return err
			}
			names := make([]string, 0, len(cfg.Contexts))
			for name := range cfg.Contexts {
				names = append(names, name)
			}
			sort.Strings(names)

			list := make([]contextRow, 0, len(names))
			rows := make([][]string, 0, len(names))
			for _, name := range names {
				ctx := cfg.Contexts[name]
				current := name == cfg.Current
				list = append(list, contextRow{Name: name, Origin: ctx.Origin, User: ctx.User, Current: current})
				marker := ""
				if current {
					marker = "*"
				}
				rows = append(rows, []string{marker, name, ctx.Origin, ctx.User})
			}
			return d.out.Write(cmd.OutOrStdout(), []string{"", "NAME", "ORIGIN", "USER"}, rows, list)
		},
	}
}

func newContextUseCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Switch the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := d.loadConfig()
			if err != nil {
				return err
			}
			name := args[0]
			if _, ok := cfg.Contexts[name]; !ok {
				return fmt.Errorf("unknown context %q — run `tinycld context list`", name)
			}
			cfg.Current = name
			if err := cfg.Save(d.configDir); err != nil {
				return err
			}
			d.out.Info(cmd.OutOrStdout(), "✓ Using context %s", name)
			return nil
		},
	}
}

func newContextAddCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "add <name> <origin>",
		Short: "Save a context without logging in",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := d.loadConfig()
			if err != nil {
				return err
			}
			name := args[0]
			if _, ok := cfg.Contexts[name]; ok {
				return fmt.Errorf("context %q already exists — `tinycld context remove %s` first", name, name)
			}
			origin, err := config.NormalizeOrigin(args[1])
			if err != nil {
				return err
			}
			cfg.Contexts[name] = config.Context{Origin: origin}
			if cfg.Current == "" {
				cfg.Current = name
			}
			if err := cfg.Save(d.configDir); err != nil {
				return err
			}
			d.out.Info(cmd.OutOrStdout(), "✓ Added context %s (%s)", name, origin)
			return nil
		},
	}
}

func newContextRemoveCmd(d *deps) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a context and its stored credentials",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := d.loadConfig()
			if err != nil {
				return err
			}
			name := args[0]
			if _, ok := cfg.Contexts[name]; !ok {
				return fmt.Errorf("unknown context %q — run `tinycld context list`", name)
			}
			delete(cfg.Contexts, name)
			if cfg.Current == name {
				cfg.Current = ""
			}
			if err := cfg.Save(d.configDir); err != nil {
				return err
			}
			if err := d.store().Delete(name); err != nil {
				return fmt.Errorf("context removed, but deleting its credential failed: %w", err)
			}
			d.out.Info(cmd.OutOrStdout(), "✓ Removed context %s", name)
			return nil
		},
	}
}
