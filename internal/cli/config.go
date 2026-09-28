package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/happydez/go-ftp/internal/config"
	"github.com/happydez/go-ftp/internal/ui"
)

func newConfigCmd(g *globalOptions) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect and change the config file",
	}

	cmd.AddCommand(
		newConfigViewCmd(g),
		newConfigGetProfilesCmd(g),
		newConfigCurrentProfileCmd(g),
		newConfigUseProfileCmd(g),
	)

	return cmd
}

func newConfigViewCmd(g *globalOptions) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:   "view",
		Short: "Print the config as it was loaded, with the defaults filled in",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}

			return writeConfig(cmd.OutOrStdout(), cfg, output)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "yaml", "output format: yaml or json")

	return cmd
}

func newConfigGetProfilesCmd(g *globalOptions) *cobra.Command {
	var output string

	cmd := &cobra.Command{
		Use:     "get-profiles",
		Short:   "List the profiles, marking the active one",
		Aliases: []string{"profiles"},
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := g.config()
			if err != nil {
				return err
			}
			return writeProfiles(cmd.OutOrStdout(), cfg, g.profile, output)
		},
	}

	cmd.Flags().StringVarP(&output, "output", "o", "table", "output format: table or name")

	return cmd
}

func newConfigCurrentProfileCmd(g *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "current-profile",
		Short: "Print the name of the profile the commands would use",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, profile, err := g.target()
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), profile.Name())

			return err
		},
	}
}

func newConfigUseProfileCmd(g *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "use-profile NAME",
		Short: "Write NAME into current_profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := args[0]

			cfg, err := g.config()
			if err != nil {
				return err
			}
			if _, err := cfg.Profile(name); err != nil {
				return usageError{err}
			}
			if err := config.SetCurrentProfile(cfg.Path(), name); err != nil {
				return err
			}

			ui.Info("switched to profile %s in %s", ui.Bold(name), ui.Path(cfg.Path()))

			return nil
		},
	}
}

func writeConfig(w io.Writer, cfg *config.Config, output string) error {
	switch output {
	case "yaml":
		if _, err := fmt.Fprintf(w, "# loaded from %s\n", cfg.Path()); err != nil {
			return err
		}
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(cfg); err != nil {
			return err
		}
		return enc.Close()
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(cfg)
	default:
		return newUsageError("unknown output format %q, want yaml or json", output)
	}
}

func writeProfiles(w io.Writer, cfg *config.Config, wanted, output string) error {
	active, err := cfg.Profile(wanted)
	if err != nil {
		active = config.Profile{}
	}

	switch output {
	case "name":
		for _, name := range cfg.ProfileNames() {
			if _, err := fmt.Fprintln(w, name); err != nil {
				return err
			}
		}
		return nil
	case "table":
		tw := tabwriter.NewWriter(w, 0, 0, 3, ' ', 0)
		if _, err := fmt.Fprintln(tw, "\tNAME\tHOST\tUSER\tBASE DIR\tTLS"); err != nil {
			return err
		}
		for _, name := range cfg.ProfileNames() {
			p := cfg.Profiles[name]
			marker := " "
			if name == active.Name() {
				marker = "*"
			}
			_, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
				marker, name, p.Addr(), p.User, p.BaseDir, yesNo(p.TLS))
			if err != nil {
				return err
			}
		}
		return tw.Flush()
	default:
		return newUsageError("unknown output format %q, want table or name", output)
	}
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
