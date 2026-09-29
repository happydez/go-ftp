package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeConn answers from a script and records what was asked of it.
type fakeConn struct {
	mu        sync.Mutex
	resets    int
	closed    bool
	uploaded  []string
	renamed   []string
	removed   []string
	sizes     map[string]int64
	uploadErr error
	onRemove  func(remotePath string)
	renameErr error
}

func (c *fakeConn) EnsureDir(context.Context, string) error {
	return nil
}

func (c *fakeConn) Size(_ context.Context, remotePath string) (int64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size, ok := c.sizes[remotePath]

	return size, ok
}

func (c *fakeConn) Upload(_ context.Context, remotePath string, r io.Reader) error {
	if _, err := io.Copy(io.Discard, r); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.uploadErr != nil {
		if c.sizes == nil {
			c.sizes = make(map[string]int64)
		}
		c.sizes[remotePath] = 0

		return c.uploadErr
	}

	c.uploaded = append(c.uploaded, remotePath)

	return nil
}

func (c *fakeConn) Download(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (c *fakeConn) Rename(_ context.Context, from, to string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.renameErr != nil {
		return c.renameErr
	}

	c.renamed = append(c.renamed, from+" -> "+to)

	for i, name := range c.uploaded {
		if name == from {
			c.uploaded[i] = to
		}
	}

	return nil
}

func (c *fakeConn) Remove(_ context.Context, remotePath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.removed = append(c.removed, remotePath)
	delete(c.sizes, remotePath)

	if c.onRemove != nil {
		c.onRemove(remotePath)
	}

	return nil
}

func (c *fakeConn) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.resets++
}

func (c *fakeConn) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
}

func (c *fakeConn) resetCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.resets
}

func (c *fakeConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closed
}

// jobs builds a list whose sizes are 0, 1, 2 and so on, so that the byte count
// in a summary can be checked against a known total.
func jobs(n int) []Job {
	list := make([]Job, 0, n)

	for i := range n {
		list = append(list, Job{
			Local:  fmt.Sprintf("local-%02d.txt", i),
			Remote: fmt.Sprintf("/remote/%02d.txt", i),
			Size:   int64(i),
		})
	}

	return list
}

// quick keeps the retry pauses out of the test runtime.
const quick = time.Millisecond

func plainConn() Conn {
	return &fakeConn{}
}

func movesFine(context.Context, Conn, Job) (Outcome, error) {
	return Moved, nil
}

func TestRunMovesEverything(t *testing.T) {
	var moved atomic.Int64

	summary, err := Run(t.Context(), jobs(20), Options{
		Workers: 4,
		Backoff: quick,
		Connect: plainConn,
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			moved.Add(1)

			return Moved, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if summary.Total != 20 || summary.Moved != 20 {
		t.Errorf("summary = %+v, want 20 of 20 moved", summary)
	}
	if summary.Failed != 0 || !summary.OK() {
		t.Errorf("summary reports failures: %+v", summary)
	}
	if moved.Load() != 20 {
		t.Errorf("the operation ran %d times, want 20", moved.Load())
	}
	if summary.Bytes != 190 {
		t.Errorf("bytes = %d, want 190 for the sizes 0 to 19", summary.Bytes)
	}
}

func TestRunNeverOpensMoreConnectionsThanFiles(t *testing.T) {
	var opened atomic.Int64

	_, err := Run(t.Context(), jobs(2), Options{
		Workers: 16,
		Backoff: quick,
		Connect: func() Conn {
			opened.Add(1)

			return &fakeConn{}
		},
		Move: movesFine,
	})
	if err != nil {
		t.Fatal(err)
	}

	if opened.Load() != 2 {
		t.Errorf("opened %d connections for 2 files, want 2", opened.Load())
	}
}

func TestRunGivesEveryWorkerItsOwnConnection(t *testing.T) {
	const workers = 4

	var (
		mu      sync.Mutex
		seen    = make(map[Conn]struct{})
		ready   = make(chan struct{})
		arrived atomic.Int64
	)

	// Every worker waits until all of them have arrived, which only finishes if
	// they really do run at the same time.
	_, err := Run(t.Context(), jobs(workers), Options{
		Workers: workers,
		Backoff: quick,
		Connect: plainConn,
		Move: func(_ context.Context, conn Conn, _ Job) (Outcome, error) {
			mu.Lock()
			seen[conn] = struct{}{}
			mu.Unlock()

			if arrived.Add(1) == workers {
				close(ready)
			}
			<-ready

			return Moved, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(seen) != workers {
		t.Errorf("%d distinct connections were used, want %d", len(seen), workers)
	}
}

func TestRunClosesEveryConnection(t *testing.T) {
	var (
		mu    sync.Mutex
		conns []*fakeConn
	)

	_, err := Run(t.Context(), jobs(6), Options{
		Workers: 3,
		Backoff: quick,
		Connect: func() Conn {
			conn := &fakeConn{}

			mu.Lock()
			conns = append(conns, conn)
			mu.Unlock()

			return conn
		},
		Move: movesFine,
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, conn := range conns {
		if !conn.isClosed() {
			t.Errorf("connection %d was left open", i)
		}
	}
}

func TestRunRetriesAndThenGivesUp(t *testing.T) {
	var attempts atomic.Int64

	boom := errors.New("connection reset")

	summary, err := Run(t.Context(), jobs(1), Options{
		Workers:    1,
		MaxRetries: 3,
		Backoff:    quick,
		Connect:    plainConn,
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			attempts.Add(1)

			return Moved, boom
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if attempts.Load() != 4 {
		t.Errorf("tried %d times, want one attempt plus three retries", attempts.Load())
	}
	if summary.Failed != 1 || len(summary.Failures) != 1 {
		t.Fatalf("summary = %+v, want one failure", summary)
	}
	if !errors.Is(summary.Failures[0].Err, boom) {
		t.Errorf("error = %v, want the one the operation returned", summary.Failures[0].Err)
	}
	if summary.Failures[0].Attempts != 4 {
		t.Errorf("attempts = %d, want 4", summary.Failures[0].Attempts)
	}
	if summary.OK() {
		t.Error("a run with a failure is not OK")
	}
}

func TestRunStopsRetryingOnceItWorks(t *testing.T) {
	var attempts atomic.Int64

	summary, err := Run(t.Context(), jobs(1), Options{
		Workers:    1,
		MaxRetries: 5,
		Backoff:    quick,
		Connect:    plainConn,
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			if attempts.Add(1) < 3 {
				return Moved, errors.New("not yet")
			}

			return Moved, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if attempts.Load() != 3 {
		t.Errorf("tried %d times, want to stop at the third", attempts.Load())
	}
	if summary.Moved != 1 || summary.Failed != 0 {
		t.Errorf("summary = %+v, want one moved file", summary)
	}
}

func TestAFailedAttemptResetsTheConnection(t *testing.T) {
	conn := &fakeConn{}

	_, err := Run(t.Context(), jobs(1), Options{
		Workers:    1,
		MaxRetries: 2,
		Backoff:    quick,
		Connect: func() Conn {
			return conn
		},
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			return Moved, errors.New("broken pipe")
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := conn.resetCount(); got != 3 {
		t.Errorf("reset %d times, want 3", got)
	}
}

func TestRunDoesNotRetryACancelledRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var attempts atomic.Int64

	summary, err := Run(ctx, jobs(1), Options{
		Workers:    1,
		MaxRetries: 10,
		Backoff:    time.Second,
		Connect:    plainConn,
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			attempts.Add(1)
			cancel()

			return Moved, context.Canceled
		},
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if attempts.Load() != 1 {
		t.Errorf("tried %d times, want to stop after the first", attempts.Load())
	}
	if summary.Total != 1 {
		t.Errorf("total = %d, want 1", summary.Total)
	}
}

func TestRunStopsHandingOutWorkWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	var done atomic.Int64

	summary, err := Run(ctx, jobs(200), Options{
		Workers: 2,
		Backoff: quick,
		Connect: plainConn,
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			if done.Add(1) == 5 {
				cancel()
			}

			return Moved, nil
		},
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
	if summary.Moved >= 200 {
		t.Errorf("moved %d of 200, the run should have stopped early", summary.Moved)
	}
}

func TestRunCountsSkippedFilesSeparately(t *testing.T) {
	summary, err := Run(t.Context(), jobs(10), Options{
		Workers: 2,
		Backoff: quick,
		Connect: plainConn,
		Move: func(_ context.Context, _ Conn, job Job) (Outcome, error) {
			if job.Size%2 == 0 {
				return Skipped, nil
			}
			return Moved, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if summary.Skipped != 5 || summary.Moved != 5 {
		t.Errorf("summary = %+v, want 5 moved and 5 skipped", summary)
	}
	if summary.Bytes != 25 {
		t.Errorf("bytes = %d, want 25", summary.Bytes)
	}
}

func TestOnResultSeesEveryFileOnce(t *testing.T) {
	var seen []Result

	summary, err := Run(t.Context(), jobs(30), Options{
		Workers: 5,
		Backoff: quick,
		Connect: plainConn,
		Move:    movesFine,
		OnResult: func(result Result) {
			seen = append(seen, result)
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(seen) != 30 {
		t.Errorf("the callback saw %d results, want 30", len(seen))
	}
	if summary.Moved != 30 {
		t.Errorf("summary = %+v, want 30 moved", summary)
	}
}

func TestRunWithNothingToDo(t *testing.T) {
	summary, err := Run(t.Context(), nil, Options{
		Workers: 4,
		Connect: func() Conn {
			t.Error("no connection should be opened for an empty list")
			return &fakeConn{}
		},
		Move: func(context.Context, Conn, Job) (Outcome, error) {
			t.Error("nothing should be moved")
			return Moved, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if summary.Total != 0 || !summary.OK() {
		t.Errorf("summary = %+v, want an empty successful run", summary)
	}
}
