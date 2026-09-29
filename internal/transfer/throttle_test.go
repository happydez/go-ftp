package transfer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNoThrottleWhenThereIsNoLimit(t *testing.T) {
	for _, limit := range []int64{0, -1} {
		if got := NewThrottle(limit); got != nil {
			t.Errorf("NewThrottle(%d) = %v, want nothing at all", limit, got)
		}
	}
}

func TestANilThrottleHandsTheStreamBack(t *testing.T) {
	var none *Throttle

	source := strings.NewReader("straight through")

	if got := none.reader(t.Context(), source); got != io.Reader(source) {
		t.Error("an unlimited run should not pay for a wrapper it does not need")
	}
}

func TestThrottleHoldsTheSpeedDown(t *testing.T) {
	const (
		limit = 20 << 10
		total = 40 << 10
	)

	throttle := NewThrottle(limit)

	started := time.Now()

	read, err := io.Copy(io.Discard, throttle.reader(t.Context(), bytes.NewReader(make([]byte, total))))
	if err != nil {
		t.Fatal(err)
	}

	elapsed := time.Since(started)

	if read != total {
		t.Errorf("read %d bytes, want %d", read, total)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("%d bytes at %d a second took %s, which is faster than the limit allows", total, limit, elapsed)
	}
	if elapsed > 4*time.Second {
		t.Errorf("took %s, far longer than the limit asks for", elapsed)
	}
}

func TestThrottleLetsTheFirstReadStraightThrough(t *testing.T) {
	throttle := NewThrottle(1 << 10)

	started := time.Now()

	buffer := make([]byte, 512)
	if _, err := throttle.reader(t.Context(), bytes.NewReader(buffer)).Read(buffer); err != nil {
		t.Fatal(err)
	}

	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Errorf("the first read waited %s before doing anything", elapsed)
	}
}

func TestThrottleStopsOnACancelledRun(t *testing.T) {
	throttle := NewThrottle(1 << 10)

	ctx, cancel := context.WithCancel(t.Context())

	reader := throttle.reader(ctx, bytes.NewReader(make([]byte, 1<<20)))

	cancel()

	_, err := io.Copy(io.Discard, reader)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want the wait to give up when the run does", err)
	}
}

func TestOneThrottleIsSharedByEveryWorker(t *testing.T) {
	const (
		limit = 32 << 10
		each  = 8 << 10
		users = 8
	)

	throttle := NewThrottle(limit)

	var (
		group sync.WaitGroup
		read  atomic.Int64
	)

	started := time.Now()

	for range users {
		group.Add(1)

		go func() {
			defer group.Done()

			n, err := io.Copy(io.Discard, throttle.reader(t.Context(), bytes.NewReader(make([]byte, each))))
			if err != nil {
				t.Error(err)
			}
			read.Add(n)
		}()
	}

	group.Wait()

	elapsed := time.Since(started)

	if got := read.Load(); got != users*each {
		t.Errorf("read %d bytes altogether, want %d", got, users*each)
	}
	if elapsed < 900*time.Millisecond {
		t.Errorf("eight readers got through %d bytes in %s, so the limit is per reader, not per run", users*each, elapsed)
	}
}

func TestASmallLimitStillMakesProgress(t *testing.T) {
	throttle := NewThrottle(64)

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	n, err := io.CopyN(io.Discard, throttle.reader(ctx, bytes.NewReader(make([]byte, 1<<20))), 64)
	if err != nil {
		t.Fatal(err)
	}
	if n != 64 {
		t.Errorf("read %d bytes, want 64", n)
	}
}
