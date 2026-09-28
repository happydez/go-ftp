package cli

import (
	"fmt"
	"io"
	"time"

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

// reporter prints one line per finished file. Run calls it from a single
// goroutine, so it needs no lock.
type reporter struct {
	name target
}

func (r reporter) report(result transfer.Result) {
	path := ui.Path(r.name(result.Job))
	switch {
	case result.Err != nil:
		ui.Fail("%s: %v", path, result.Err)
	case result.Outcome == transfer.Skipped:
		ui.Skip("%s is already there", path)
	default:
		ui.OK("%s (%s)", path, ui.Bytes(result.Job.Size))
	}
}

// printPlan is what --dry-run answers with. The arrow points the way the files
// would actually travel.
func printPlan(w io.Writer, jobs []transfer.Job, verb string, from, to target) error {
	_, err := fmt.Fprintf(w, "would %s %d file(s), %s\n\n", verb, len(jobs), ui.Bytes(transfer.TotalSize(jobs)))
	if err != nil {
		return err
	}

	for _, job := range jobs {
		if _, err := fmt.Fprintf(w, "  %s -> %s\n", from(job), to(job)); err != nil {
			return err
		}
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
