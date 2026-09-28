// Package cli wires the command tree.
package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/happydez/go-ftp/internal/ui"
)

// Exit codes the binary answers with.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// globalOptions are the flags every command shares.
type globalOptions struct {
	configPath string
	credsPath  string
	profile    string
	noColor    bool
	verbose    bool
	quiet      bool
}

// usageError marks a mistake in the command line or in the config, as opposed
// to a transfer that went wrong. Only these leave with exit code 2.
type usageError struct {
	err error
}

func (e usageError) Error() string {
	return e.err.Error()
}

func (e usageError) Unwrap() error {
	return e.err
}

func newUsageError(format string, a ...any) error {
	return usageError{fmt.Errorf(format, a...)}
}

// Execute runs the command tree and returns the process exit code. This is the
// only place an error is printed.
func Execute(ctx context.Context) int {
	err := newRootCmd().ExecuteContext(ctx)
	if err == nil {
		return exitOK
	}

	var ue usageError
	switch {
	case errors.As(err, &ue):
		ui.Error("%v", err)
		return exitUsage
	case errors.Is(err, context.Canceled):
		ui.Warn("interrupted")
		return exitFailure
	case errors.Is(err, context.DeadlineExceeded):
		ui.Error("the run ran out of time, see transfer.timeout in the config")
		return exitFailure
	default:
		ui.Error("%v", err)
		return exitFailure
	}
}

func newRootCmd() *cobra.Command {
	g := &globalOptions{}

	cmd := &cobra.Command{
		Use:   "go-ftp",
		Short: "Upload and download files over FTP",
		Long: "go-ftp moves files between this machine and an FTP server.\n\n" +
			"Remote paths are resolved relative to the base_dir of the active profile,\n" +
			"so nothing outside it can be touched by accident.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(*cobra.Command, []string) {
			ui.SetColor(!g.noColor)
			switch {
			case g.verbose:
				ui.SetVerbosity(ui.Verbose)
			case g.quiet:
				ui.SetVerbosity(ui.Quiet)
			}
		},
	}

	f := cmd.PersistentFlags()
	f.StringVar(&g.configPath, "config", "", "config file to use instead of the searched ones")
	f.StringVar(&g.credsPath, "creds", "", "credentials file to use instead of the default one")
	f.StringVarP(&g.profile, "profile", "p", "", "profile to use instead of current_profile")
	f.BoolVar(&g.noColor, "no-color", false, "disable coloured output")
	f.BoolVarP(&g.verbose, "verbose", "v", false, "print debug output")
	f.BoolVarP(&g.quiet, "quiet", "q", false, "print warnings and errors only")

	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return usageError{err}
	})

	cmd.AddCommand(
		newConfigCmd(g),
		newDownloadCmd(g),
		newLoginCmd(g),
		newLsCmd(g),
		newUploadCmd(g),
		newLogoutCmd(g),
		newWhoamiCmd(g),
		newVersionCmd(),
	)

	return cmd
}
