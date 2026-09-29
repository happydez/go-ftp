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
	contents     bool
}

func (m *moveOptions) bind(cmd *cobra.Command, localHelp, remoteHelp string) {
	f := cmd.Flags()
	f.StringVar(&m.local, "local", "", localHelp)
	f.StringVar(&m.remote, "ftp", "", remoteHelp)
	f.IntVar(&m.workers, "workers", 0, "files in flight at once, overriding the config")
	f.BoolVar(&m.skipExisting, "skip-existing", false, "leave files whose size already matches")
	f.BoolVar(&m.dryRun, "dry-run", false, "list what would move and stop")
	f.BoolVar(&m.contents, "contents", false, "send what is inside the directory instead of the directory itself")
}

func newUploadCmd(g *globalOptions) *cobra.Command {
	opts := &moveOptions{}

	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Send files to the server",
		Long: "upload sends a local file or the contents of a local directory to a\n" +
			"directory on the server. Remote paths are relative to the base_dir of the\n" +
			"active profile.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.local == "" || opts.remote == "" {
				return newUsageError("both --local and --ftp are required")
			}

			s, err := g.session()
			if err != nil {
				return err
			}

			remoteDir, err := s.remotePath(opts.remote)
			if err != nil {
				return err
			}

			plan, err := transfer.PlanUpload(remoteDir, opts.local, opts.contents)
			if err != nil {
				return usageError{err}
			}

			reportIgnored(plan)

			if plan.Empty() {
				ui.Info("nothing to upload from %s", ui.Path(opts.local))

				return nil
			}

			if opts.dryRun {
				return printPlan(cmd.OutOrStdout(), plan, "upload", localSide, remoteSide)
			}

			connect, err := s.connect()
			if err != nil {
				return err
			}

			ui.Header("uploading %d file(s), %s, from %s to %s on %s",
				len(plan.Jobs), ui.Bytes(plan.TotalSize()),
				ui.Path(opts.local), ui.Path(remoteDir), s.profile.Addr())

			ctx, cancel := s.withTimeout(cmd.Context())
			defer cancel()

			// A directory with no files of its own would never be created by a
			// transfer, so it is made before the pool starts.
			if err := makeRemoteDirs(ctx, connect, plan.Dirs); err != nil {
				return err
			}

			progress := ui.NewProgress(len(plan.Jobs), plan.TotalSize())

			summary, runErr := transfer.Run(ctx, plan.Jobs, transfer.Options{
				Workers:    s.workers(opts.workers),
				MaxRetries: s.cfg.Transfer.MaxRetries,
				Connect: func() transfer.Conn {
					return connect()
				},
				Move:     transfer.Uploader(opts.skipExisting, opts.inPlace, progress.AddBytes),
				OnResult: reporter{name: remoteSide, progress: progress}.report,
			})

			progress.Stop()
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
