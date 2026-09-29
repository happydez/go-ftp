package transfer

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/happydez/go-ftp/internal/ftpx"
)

// DefaultBackoff is the first pause between attempts. Every attempt after that
// waits twice as long, up to a ceiling.
const DefaultBackoff = 500 * time.Millisecond

// Conn is what one worker holds.
type Conn interface {
	EnsureDir(ctx context.Context, dir string) error
	Size(ctx context.Context, remotePath string) (int64, bool)
	Upload(ctx context.Context, remotePath string, r io.Reader) error
	Download(ctx context.Context, remotePath string) (io.ReadCloser, error)
	Rename(ctx context.Context, from, to string) error
	Remove(ctx context.Context, remotePath string) error
	Reset()
	Close()
}

var _ Conn = (*ftpx.Client)(nil)

// Outcome is what happened to a file that did not fail.
type Outcome int

const (
	Moved Outcome = iota
	Skipped
)

// Operation moves one file over the connection its worker holds.
type Operation func(ctx context.Context, conn Conn, job Job) (Outcome, error)

// Result is the end of one file.
type Result struct {
	Job      Job
	Outcome  Outcome
	Err      error
	Attempts int
}

// Options describes a run.
type Options struct {
	// Workers is how many files travel at once. A run never opens more
	// connections than it has files.
	Workers int
	// MaxRetries is the number of attempts on top of the first one.
	MaxRetries int
	// Backoff is the first pause between attempts, doubling after that.
	Backoff time.Duration
	// Connect gives every worker its own connection, since one FTP connection
	// carries one transfer at a time.
	Connect func() Conn
	// Move is what a worker does with a file.
	Move Operation
	// OnResult is called once per file from a single goroutine, so a printer
	// needs no lock of its own.
	OnResult func(Result)
}

// Summary is the whole run in numbers, plus everything that failed.
type Summary struct {
	Total    int
	Moved    int
	Skipped  int
	Failed   int
	Bytes    int64
	Failures []Result
	Elapsed  time.Duration
}

// OK reports whether every file made it.
func (s Summary) OK() bool {
	return s.Failed == 0
}

// Run works the list and returns what happened. The error is only about the run
// as a whole, so a single file that failed shows up in the summary instead.
func Run(ctx context.Context, jobs []Job, opts Options) (Summary, error) {
	summary := Summary{Total: len(jobs)}
	if len(jobs) == 0 {
		return summary, nil
	}

	started := time.Now()

	if opts.Backoff <= 0 {
		opts.Backoff = DefaultBackoff
	}

	queue := make(chan Job)
	results := make(chan Result)

	var group sync.WaitGroup

	for range min(max(opts.Workers, 1), len(jobs)) {
		group.Add(1)

		go func() {
			defer group.Done()

			worker{
				conn:    opts.Connect(),
				move:    opts.Move,
				retries: opts.MaxRetries,
				backoff: opts.Backoff,
			}.run(ctx, queue, results)
		}()
	}

	go func() {
		defer close(queue)

		for _, job := range jobs {
			select {
			case <-ctx.Done():
				return
			case queue <- job:
			}
		}
	}()

	go func() {
		group.Wait()
		close(results)
	}()

	for result := range results {
		summary.add(result)

		if opts.OnResult != nil {
			opts.OnResult(result)
		}
	}

	summary.Elapsed = time.Since(started)

	return summary, ctx.Err()
}

func (s *Summary) add(result Result) {
	switch {
	case result.Err != nil:
		s.Failed++
		s.Failures = append(s.Failures, result)
	case result.Outcome == Skipped:
		s.Skipped++
	default:
		s.Moved++
		s.Bytes += result.Job.Size
	}
}

type worker struct {
	conn    Conn
	move    Operation
	retries int
	backoff time.Duration
}

func (w worker) run(ctx context.Context, queue <-chan Job, results chan<- Result) {
	defer w.conn.Close()
	for {
		select {
		case <-ctx.Done():
			return
		case job, ok := <-queue:
			if !ok {
				return
			}
			select {
			case <-ctx.Done():
				return
			case results <- w.process(ctx, job):
			}
		}
	}
}

func (w worker) process(ctx context.Context, job Job) Result {
	result := Result{Job: job}

	result.Err = retry(ctx, w.retries, w.backoff, func() error {
		result.Attempts++

		outcome, err := w.move(ctx, w.conn, job)
		if err != nil {
			w.conn.Reset()
			return err
		}

		result.Outcome = outcome

		return nil
	})

	return result
}

// retry runs attempt up to extra+1 times with a growing pause. A cancelled run
// stops at once, since waiting to try again is not what was asked for.
func retry(ctx context.Context, extra int, backoff time.Duration, attempt func() error) error {
	var last error
	for i := 0; i <= extra; i++ {
		if i > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff << min(i-1, 5)):
			}
		}

		err := attempt()
		if err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		last = err
	}

	return last
}
