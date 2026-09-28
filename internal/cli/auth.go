package cli

import (
	"context"
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
	var (
		fromStdin bool
		noVerify  bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store the password for a profile",
		Long: "login asks for the password of the active profile, checks it against the\n" +
			"server and writes it to the credentials file, which lives outside the config.",
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

			// The store is deliberately left out of the lookup. Logging in again
			// has to ask for a password, not quietly reuse the old one.
			password, source, err := creds.Resolve(creds.Options{
				Profile:  profile.Name(),
				Stdin:    cmd.InOrStdin(),
				UseStdin: fromStdin,
				Prompt:   ask,
			})
			if err != nil {
				return usageError{err}
			}

			if !noVerify {
				if err := checkLogin(cmd.Context(), profile, password); err != nil {
					return err
				}
				ui.OK("logged in to %s as %s", profile.Addr(), creds.User(profile.User))
			}

			if err := store.Set(profile.Name(), password); err != nil {
				return err
			}

			ui.Info("stored the password for %s, read from %s", ui.Bold(profile.Name()), source)
			ui.Info("credentials file: %s", ui.Path(store.Path()))

			return nil
		},
	}

	cmd.Flags().BoolVar(&fromStdin, "password-stdin", false, "read the password from stdin instead of asking")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "store the password without checking it against the server")

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
	var offline bool

	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the active profile and check that it can log in",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s, err := g.session()
			if err != nil {
				return err
			}

			if err := printTarget(cmd.OutOrStdout(), s); err != nil {
				return err
			}

			if offline || s.source == creds.SourceNone {
				return nil
			}

			client, err := s.client()
			if err != nil {
				return err
			}
			defer client.Close()

			if err := client.Connect(cmd.Context()); err != nil {
				return err
			}
			ui.OK("logged in to %s as %s", s.profile.Addr(), creds.User(s.profile.User))

			return nil
		},
	}

	cmd.Flags().BoolVar(&offline, "offline", false, "report the settings without contacting the server")

	return cmd
}

// checkLogin opens a connection, logs in and hangs up, which is the cheapest
// proof that a password works.
func checkLogin(ctx context.Context, profile config.Profile, password string) error {
	client := newClient(profile, password)
	defer client.Close()

	return client.Connect(ctx)
}

func printTarget(w io.Writer, s *session) error {
	rows := [][2]string{
		{"profile", s.profile.Name()},
		{"server", s.profile.Addr()},
		{"user", creds.User(s.profile.User)},
		{"base dir", s.profile.BaseDir},
		{"tls", yesNo(s.profile.TLS)},
		{"config", s.cfg.Path()},
		{"password", s.source.String()},
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

	if s.source == creds.SourceNone {
		ui.Warn("no password available, run `go-ftp login` or set %s", creds.EnvPassword)
	}
	if s.store.TooOpen() {
		ui.Warn("%s is readable by other users, run: chmod 600 %s", s.store.Path(), s.store.Path())
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
