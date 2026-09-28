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
			"profile.\n\n" +
			"A directory contributes its contents rather than itself, so\n" +
			"--ftp /my --local . puts <base_dir>/my/a.txt at ./a.txt.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.local == "" || opts.remote == "" {
				return newUsageError("both --local and --ftp are required")
			}

			s, err := g.session()
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

			remoteSource, err := transfer.Resolve(s.profile.BaseDir, opts.remote)
			if err != nil {
				return usageError{err}
			}

			jobs, err := transfer.PlanDownload(ctx, lister, s.profile.BaseDir, opts.remote, opts.local)
			if err != nil {
				return err
			}
			if len(jobs) == 0 {
				ui.Info("nothing to download from %s", ui.Path(opts.remote))
				return nil
			}

			if opts.dryRun {
				return printPlan(cmd.OutOrStdout(), jobs, "download", remoteSide, localSide)
			}

			ui.Header("downloading %d file(s), %s, from %s on %s to %s",
				len(jobs), ui.Bytes(transfer.TotalSize(jobs)),
				ui.Path(remoteSource), s.profile.Addr(), ui.Path(opts.local))

			summary, runErr := transfer.Run(ctx, jobs, transfer.Options{
				Workers:    s.workers(opts.workers),
				MaxRetries: s.cfg.Transfer.MaxRetries,
				Connect: func() transfer.Conn {
					return connect()
				},
				Move:     transfer.Downloader(opts.skipExisting),
				OnResult: reporter{name: localSide}.report,
			})

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
