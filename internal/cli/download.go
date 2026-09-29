package cli

import (
	"github.com/spf13/cobra"

	"github.com/happydez/go-ftp/internal/transfer"
	"github.com/happydez/go-ftp/internal/ui"
)

func newDownloadCmd(g *globalOptions) *cobra.Command {
	opts := &moveOptions{}

	cmd := &cobra.Command{
		Use:   "download",
		Short: "Fetch files from the server",
		Long: "download brings a remote file, or the contents of a remote directory, into\n" +
			"a local directory. Remote paths are relative to the base_dir of the active\n" +
			"profile.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.local == "" || opts.remote == "" {
				return newUsageError("both --local and --ftp are required")
			}

			s, err := g.session()
			if err != nil {
				return err
			}

			throttle, err := s.throttle(opts.limit, s.cfg.Transfer.DownloadLimit)
			if err != nil {
				return err
			}

			connect, err := s.connect()
			if err != nil {
				return err
			}

			ctx, cancel := s.withTimeout(cmd.Context())
			defer cancel()

			lister := connect()
			defer lister.Close()

			remoteSource, err := s.remotePath(opts.remote)
			if err != nil {
				return err
			}

			contents := opts.contents || remoteSource == s.profile.BaseDir

			plan, err := transfer.PlanDownload(ctx, lister, remoteSource, opts.local, contents)
			if err != nil {
				return err
			}
			if plan.Empty() {
				ui.Info("nothing to download from %s", ui.Path(opts.remote))
				return nil
			}

			if opts.dryRun {
				return printPlan(cmd.OutOrStdout(), plan, "download", remoteSide, localSide)
			}

			ui.Header("downloading %d file(s), %s, from %s on %s to %s",
				len(plan.Jobs), ui.Bytes(plan.TotalSize()),
				ui.Path(remoteSource), s.profile.Addr(), ui.Path(opts.local))

			// A remote directory holding no files would never be created by a
			// transfer, so it is made before the pool starts.
			if err := makeLocalDirs(plan.Dirs); err != nil {
				return err
			}

			progress := ui.NewProgress(len(plan.Jobs), plan.TotalSize())

			summary, runErr := transfer.Run(ctx, plan.Jobs, transfer.Options{
				Workers:    s.workers(opts.workers),
				MaxRetries: s.cfg.Transfer.MaxRetries,
				Connect: func() transfer.Conn {
					return connect()
				},
				Move: transfer.Downloader(transfer.StreamOptions{
					SkipExisting: opts.skipExisting,
					Count:        progress.AddBytes,
					Throttle:     throttle,
				}),
				OnResult: reporter{name: localSide, progress: progress}.report,
			})

			progress.Stop()
			printSummary(summary, localSide)

			if runErr != nil {
				return runErr
			}

			return summaryError(summary)
		},
	}

	opts.bind(cmd, "local directory to write into", "file or directory on the server, relative to base_dir")

	return cmd
}
