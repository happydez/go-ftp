package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/happydez/go-ftp/internal/ui"
)

func newMvCmd(g *globalOptions) *cobra.Command {
	return &cobra.Command{
		Use:     "mv SOURCE DEST",
		Aliases: []string{"rename"},
		Short:   "Move or rename something on the server",
		Long: "mv renames a remote file or directory. Both paths are relative to the\n" +
			"base_dir of the active profile, so nothing outside it can be touched.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := g.session()
			if err != nil {
				return err
			}

			from, err := s.remotePath(args[0])
			if err != nil {
				return err
			}

			to, err := s.remotePath(args[1])
			if err != nil {
				return err
			}

			if from == to {
				return newUsageError("%s and %s are the same path", args[0], args[1])
			}

			client, err := s.client()
			if err != nil {
				return err
			}
			defer client.Close()

			if err := client.Rename(cmd.Context(), from, to); err != nil {
				return err
			}

			ui.OK("%s -> %s", ui.Path(from), ui.Path(to))

			return nil
		},
	}
}

func newRmCmd(g *globalOptions) *cobra.Command {
	var (
		recursive bool
		assumeYes bool
	)

	cmd := &cobra.Command{
		Use:     "rm PATH",
		Aliases: []string{"remove"},
		Short:   "Delete a file, or a whole directory with -r",
		Long: "rm deletes a remote file. A directory needs -r, and that asks first\n" +
			"unless --yes is given. The path is relative to the base_dir of the active\n" +
			"profile.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := g.session()
			if err != nil {
				return err
			}

			remote, err := s.remotePath(args[0])
			if err != nil {
				return err
			}
			if remote == s.profile.BaseDir {
				return newUsageError("refusing to delete the base directory %s itself", remote)
			}

			client, err := s.client()
			if err != nil {
				return err
			}
			defer client.Close()

			ctx := cmd.Context()

			entry, found, err := client.Stat(ctx, remote)
			if err != nil {
				return err
			}
			if !found {
				return fmt.Errorf("%s is not on the server", remote)
			}

			if !entry.Dir {
				if err := client.Remove(ctx, remote); err != nil {
					return err
				}
				ui.OK("deleted %s", ui.Path(remote))

				return nil
			}

			if !recursive {
				return newUsageError("%s is a directory, pass -r to delete it and everything in it", remote)
			}

			// Deleting a tree is the one thing here that cannot be undone, so it
			// says what it is about to do and waits for a yes.
			if !assumeYes {
				confirmed, err := confirm(fmt.Sprintf("delete %s and everything under it?", remote))
				if err != nil {
					return err
				}
				if !confirmed {
					ui.Info("left alone")
					return nil
				}
			}

			if err := client.RemoveDir(ctx, remote); err != nil {
				return err
			}

			ui.OK("deleted %s and everything under it", ui.Path(remote))

			return nil
		},
	}

	f := cmd.Flags()
	f.BoolVarP(&recursive, "recursive", "r", false, "delete a directory and everything in it")
	f.BoolVar(&assumeYes, "yes", false, "do not ask before deleting a directory")

	return cmd
}

// confirm asks a yes or no question. Without a terminal there is nobody to ask,
// and guessing yes on a delete is not an option.
func confirm(question string) (bool, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return false, newUsageError("nothing to ask on, pass --yes to go ahead without a question")
	}

	fmt.Fprintf(os.Stderr, "%s [y/N] ", question)

	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read the answer: %w", err)
	}

	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}
