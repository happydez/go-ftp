package cli

import (
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/happydez/go-ftp/internal/config"
	"github.com/happydez/go-ftp/internal/creds"
	"github.com/happydez/go-ftp/internal/ui"
)

func newLoginCmd(g *globalOptions) *cobra.Command {
	var fromStdin bool

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store the password for a profile",
		Long: "login asks for the password of the active profile and writes it to the\n" +
			"credentials file, which lives outside the config so that the config stays\n" +
			"safe to commit.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, profile, err := g.target()
			if err != nil {
				return err
			}

			store, err := g.credentials()
			if err != nil {
				return err
			}

			ask := prompter()
			if !fromStdin && os.Getenv(creds.EnvPassword) == "" && ask == nil {
				return newUsageError("there is no terminal to ask on, use --password-stdin or set %s",
					creds.EnvPassword)
			}

			password, source, err := creds.Resolve(creds.Options{
				Profile:  profile.Name(),
				Stdin:    cmd.InOrStdin(),
				UseStdin: fromStdin,
				Prompt:   ask,
			})
			if err != nil {
				return usageError{err}
			}

			if err := store.Set(profile.Name(), password); err != nil {
				return err
			}

			ui.Info("stored the password for %s (%s@%s), read from %s", ui.Bold(profile.Name()), creds.User(profile.User), profile.Addr(), source)
			ui.Info("credentials file: %s", ui.Path(store.Path()))

			return nil
		},
	}

	cmd.Flags().BoolVar(&fromStdin, "password-stdin", false, "read the password from stdin instead of asking")

	return cmd
}

func newLogoutCmd(g *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Forget the stored password for a profile",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			_, profile, err := g.target()
			if err != nil {
				return err
			}

			store, err := g.credentials()
			if err != nil {
				return err
			}

			removed, err := store.Delete(profile.Name())
			if err != nil {
				return err
			}
			if !removed {
				ui.Info("no password was stored for %s", ui.Bold(profile.Name()))
				return nil
			}

			ui.Info("forgot the password for %s", ui.Bold(profile.Name()))

			return nil
		},
	}
}

func newWhoamiCmd(g *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the active profile and where its password comes from",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, profile, err := g.target()
			if err != nil {
				return err
			}

			store, err := g.credentials()
			if err != nil {
				return err
			}

			_, source, err := creds.Resolve(creds.Options{
				Profile: profile.Name(),
				Store:   store,
			})
			if err != nil {
				source = creds.SourceNone
			}

			return printTarget(cmd.OutOrStdout(), cfg, profile, store, source)
		},
	}
}

func printTarget(w io.Writer, cfg *config.Config, profile config.Profile, store *creds.Store, source creds.Source) error {
	rows := [][2]string{
		{"profile", profile.Name()},
		{"server", profile.Addr()},
		{"user", creds.User(profile.User)},
		{"base dir", profile.BaseDir},
		{"tls", yesNo(profile.TLS)},
		{"config", cfg.Path()},
		{"password", source.String()},
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, row := range rows {
		if _, err := fmt.Fprintf(tw, "%s:\t%s\n", row[0], row[1]); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if source == creds.SourceNone {
		ui.Warn("no password available, run `go-ftp login` or set %s", creds.EnvPassword)
	}
	if store.TooOpen() {
		ui.Warn("%s is readable by other users, run: chmod 600 %s", store.Path(), store.Path())
	}

	return nil
}

// prompter returns a way to ask for a password, or nil when there is no terminal
// to ask on. A script gets an error instead of hanging on a prompt nobody sees.
func prompter() creds.Prompter {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return nil
	}

	return func(prompt string) (string, error) {
		fmt.Fprint(os.Stderr, prompt)

		password, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", fmt.Errorf("read the password: %w", err)
		}

		return string(password), nil
	}
}
