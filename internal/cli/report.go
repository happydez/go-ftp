package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/happydez/go-ftp/internal/ftpx"
	"github.com/happydez/go-ftp/internal/transfer"
	"github.com/happydez/go-ftp/internal/ui"
)

// elapsedPrecision is how much of the run time is worth reading.
const elapsedPrecision = 10 * time.Millisecond

// target names the side of a job the user cares about. For an upload that is
// where the file landed, for a download where it came to rest here.
type target func(transfer.Job) string

func remoteSide(job transfer.Job) string {
	return job.Remote
}

func localSide(job transfer.Job) string {
	return job.Local
}

type reporter struct {
	name     target
	progress *ui.Progress
}

func (r reporter) report(result transfer.Result) {
	r.progress.Clear()

	path := ui.Path(r.name(result.Job))

	switch {
	case result.Err != nil:
		ui.Fail("%s: %v", path, result.Err)
	case result.Outcome == transfer.Skipped:
		ui.Skip("%s is already there", path)
	default:
		ui.OK("%s (%s)", path, ui.Bytes(result.Job.Size))
	}

	moved := int64(0)
	if result.Err == nil && result.Outcome == transfer.Moved {
		moved = result.Job.Size
	}

	r.progress.Advance(moved)
}

// printPlan is what --dry-run answers with. The arrow points the way the files
// would actually travel.
func printPlan(w io.Writer, plan transfer.Plan, verb string, from, to target) error {
	_, err := fmt.Fprintf(w, "would %s %d file(s), %s\n\n", verb, len(plan.Jobs), ui.Bytes(plan.TotalSize()))
	if err != nil {
		return err
	}

	for _, job := range plan.Jobs {
		if _, err := fmt.Fprintf(w, "  %s -> %s\n", from(job), to(job)); err != nil {
			return err
		}
	}

	if len(plan.Dirs) == 0 {
		return nil
	}

	if _, err := fmt.Fprintf(w, "\nand create %d empty director(ies):\n", len(plan.Dirs)); err != nil {
		return err
	}

	for _, dir := range plan.Dirs {
		if _, err := fmt.Fprintf(w, "  %s\n", dir); err != nil {
			return err
		}
	}

	return nil
}

// reportIgnored says which source paths were passed over, since a file that is
// not a plain file cannot be sent and silence about it looks like success.
func reportIgnored(plan transfer.Plan) {
	for _, path := range plan.Ignored {
		ui.Warn("%s is not a plain file, skipping it", path)
	}
}

// makeRemoteDirs creates the directories no file would create on its own. One
// connection is enough and it is opened only when there is something to do.
func makeRemoteDirs(ctx context.Context, connect func() *ftpx.Client, dirs []string) error {
	if len(dirs) == 0 {
		return nil
	}

	client := connect()
	defer client.Close()

	for _, dir := range dirs {
		if err := client.EnsureDir(ctx, dir); err != nil {
			return err
		}
		ui.Debug("created %s", dir)
	}

	return nil
}

func makeLocalDirs(dirs []string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
		ui.Debug("created %s", dir)
	}

	return nil
}

// printSummary closes a run with the counts and, when there is anything to say,
// every file that did not make it.
func printSummary(summary transfer.Summary, name target) {
	ui.Info("")
	ui.Header("%d moved, %d skipped, %d failed of %d in %s, %s transferred",
		summary.Moved, summary.Skipped, summary.Failed, summary.Total,
		summary.Elapsed.Round(elapsedPrecision), ui.Bytes(summary.Bytes))

	if len(summary.Failures) == 0 {
		return
	}

	ui.Info("")
	ui.Info("did not make it:")

	for _, failure := range summary.Failures {
		ui.Fail("%s after %d attempt(s): %v", ui.Path(name(failure.Job)), failure.Attempts, failure.Err)
	}
}

// summaryError turns a run with failures into a non-zero exit code. The files
// themselves have already been listed, so this only carries the count.
func summaryError(summary transfer.Summary) error {
	if summary.OK() {
		return nil
	}

	return fmt.Errorf("%d of %d file(s) failed", summary.Failed, summary.Total)
}
