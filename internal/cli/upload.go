package cli

import (
	"github.com/spf13/cobra"

	"github.com/happydez/go-ftp/internal/transfer"
	"github.com/happydez/go-ftp/internal/ui"
)

// moveOptions are the flags upload and download share.
type moveOptions struct {
	local        string
	remote       string
	workers      int
	skipExisting bool
	inPlace      bool
	dryRun       bool
}

func (m *moveOptions) bind(cmd *cobra.Command, localHelp, remoteHelp string) {
	f := cmd.Flags()
	f.StringVar(&m.local, "local", "", localHelp)
	f.StringVar(&m.remote, "ftp", "", remoteHelp)
	f.IntVar(&m.workers, "workers", 0, "files in flight at once, overriding the config")
	f.BoolVar(&m.skipExisting, "skip-existing", false, "leave files whose size already matches")
	f.BoolVar(&m.dryRun, "dry-run", false, "list what would move and stop")
}

func newUploadCmd(g *globalOptions) *cobra.Command {
	opts := &moveOptions{}

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Send files to the server",
		Long: "upload sends a local file or the contents of a local directory to a\n" +
			"directory on the server. Remote paths are relative to the base_dir of the\n" +
			"active profile.\n\n" +
			"A directory contributes its contents rather than itself, so\n" +
			"--local ./my --ftp / puts ./my/a.txt at <base_dir>/a.txt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.local == "" || opts.remote == "" {
				return newUsageError("both --local and --ftp are required")
			}

			s, err := g.session()
			if err != nil {
				return err
			}

			remoteDir, err := transfer.Resolve(s.profile.BaseDir, opts.remote)
			if err != nil {
				return usageError{err}
			}

			jobs, err := transfer.PlanUpload(s.profile.BaseDir, opts.local, opts.remote)
			if err != nil {
				return usageError{err}
			}
			if len(jobs) == 0 {
				ui.Info("nothing to upload from %s", ui.Path(opts.local))

				return nil
			}

			if opts.dryRun {
				return printPlan(cmd.OutOrStdout(), jobs, "upload", localSide, remoteSide)
			}

			connect, err := s.connect()
			if err != nil {
				return err
			}

			ui.Header("uploading %d file(s), %s, from %s to %s on %s",
				len(jobs), ui.Bytes(transfer.TotalSize(jobs)),
				ui.Path(opts.local), ui.Path(remoteDir), s.profile.Addr())

			ctx, cancel := s.withTimeout(cmd.Context())
			defer cancel()

			summary, runErr := transfer.Run(ctx, jobs, transfer.Options{
				Workers:    s.workers(opts.workers),
				MaxRetries: s.cfg.Transfer.MaxRetries,
				Connect: func() transfer.Conn {
					return connect()
				},
				Move:     transfer.Uploader(opts.skipExisting, opts.inPlace),
				OnResult: reporter{name: remoteSide}.report,
			})

			printSummary(summary, remoteSide)

			if runErr != nil {
				return runErr
			}

			return summaryError(summary)
		},
	}

	opts.bind(cmd, "local file or directory to send", "destination directory on the server, relative to base_dir")

	cmd.Flags().BoolVar(&opts.inPlace, "inplace", false, "write straight to the target name instead of uploading and renaming")

	return cmd
}
