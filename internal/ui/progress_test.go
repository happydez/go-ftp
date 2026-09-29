package ui

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func drawnBar(t *testing.T, line string) string {
	t.Helper()

	from := strings.Index(line, "[")
	to := strings.Index(line, "]")
	if from < 0 || to < from {
		t.Fatalf("the line has no bar in it: %q", line)
	}

	return line[from+1 : to]
}

func stalled(totalFiles int, totalBytes int64) *Progress {
	bar := &Progress{
		live:       true,
		started:    time.Now(),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
		stop:       make(chan struct{}),
		stopped:    make(chan struct{}),
	}
	close(bar.stopped)

	return bar
}

func TestProgressDrawsNothingWhenOutputIsNotATerminal(t *testing.T) {
	stdout, _ := capture(t)

	bar := NewProgress(10, 1000)
	bar.AddBytes(100)
	bar.Advance()
	bar.Stop()

	if stdout.Len() != 0 {
		t.Errorf("wrote %q, want nothing outside a terminal", stdout.String())
	}
}

func TestProgressIsSafeOnANilReceiver(t *testing.T) {
	var bar *Progress

	bar.AddBytes(1)
	bar.Advance()
	bar.Stop()

	printed := false
	bar.Line(func() {
		printed = true
	})

	if !printed {
		t.Error("Line must still print when there is no bar")
	}
}

func TestProgressLine(t *testing.T) {
	bar := stalled(10, 10<<20)
	bar.started = time.Now().Add(-2 * time.Second)
	bar.files.Store(5)
	bar.bytes.Store(5 << 20)

	line := bar.render()

	for _, want := range []string{"5/10 files", "5.0 MB/10.0 MB", "/s"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line is missing %q:\n%s", want, line)
		}
	}

	drawn := drawnBar(t, line)
	if got := len([]rune(drawn)); got != barWidth {
		t.Errorf("the bar is %d wide, want %d", got, barWidth)
	}

	if filled := strings.Count(drawn, "█"); filled == 0 || filled >= barWidth {
		t.Errorf("half the bytes are through, but the bar reads %q", drawn)
	}
}

func TestProgressFollowsTheBytesNotTheFiles(t *testing.T) {
	bar := stalled(1, 1000)

	bar.bytes.Store(900)

	drawn := drawnBar(t, bar.render())
	if filled := strings.Count(drawn, "█"); filled < barWidth-3 || filled > barWidth {
		t.Errorf("the bar reads %q, want it nearly full", drawn)
	}
}

func TestProgressAtBothEnds(t *testing.T) {
	bar := stalled(4, 100)

	empty := bar.render()
	if strings.Contains(empty, "█") {
		t.Errorf("nothing has moved yet, but the bar reads %q", empty)
	}
	if !strings.Contains(empty, "0/4 files") {
		t.Errorf("the line does not say nothing is done:\n%s", empty)
	}

	bar.files.Store(4)
	bar.bytes.Store(100)

	full := drawnBar(t, bar.render())
	if got := strings.Count(full, "█"); got != barWidth {
		t.Errorf("the bar is %d full, want all %d", got, barWidth)
	}
}

func TestProgressDoesNotRunPastTheEnd(t *testing.T) {
	bar := stalled(1, 100)

	bar.bytes.Store(250)

	line := bar.render()
	if got := strings.Count(drawnBar(t, line), "█"); got != barWidth {
		t.Errorf("the bar is %d full, want it stopped at %d", got, barWidth)
	}
	if !strings.Contains(line, "100 B/100 B") {
		t.Errorf("the count ran past the total:\n%s", line)
	}
}

func TestProgressRateIsHeldBackUntilThereAreBytes(t *testing.T) {
	bar := stalled(2, 10)

	if !strings.Contains(bar.render(), "--") {
		t.Errorf("a rate from no bytes and no time is a guess, want -- instead:\n%s", bar.render())
	}
}

func TestProgressRewritesOneLineAndCleansUp(t *testing.T) {
	stdout, _ := capture(t)

	bar := stalled(4, 400)

	bar.AddBytes(100)
	bar.redraw()

	first := stdout.String()
	if strings.Contains(first, "\n") {
		t.Errorf("the bar must stay on one line, got %q", first)
	}
	if !strings.Contains(first, "100 B/400 B") {
		t.Errorf("the bar does not show the progress: %q", first)
	}

	stdout.Reset()
	bar.AddBytes(100)
	bar.redraw()

	second := stdout.String()

	if !strings.HasPrefix(second, "\r") {
		t.Errorf("a redraw should start by going back to the start of the line: %q", second)
	}
	if !strings.Contains(second, "200 B/400 B") {
		t.Errorf("the bar did not move on: %q", second)
	}

	stdout.Reset()
	bar.Stop()

	if last := stdout.String(); !strings.HasSuffix(last, "\r") {
		t.Errorf("stopping should leave the line empty and the cursor at its start: %q", last)
	}
}

func TestLineTakesTheBarDownBeforePrinting(t *testing.T) {
	stdout, _ := capture(t)

	bar := stalled(2, 20)
	bar.AddBytes(10)
	bar.redraw()

	stdout.Reset()
	bar.Line(func() {
		OK("a.txt")
	})

	printed := stdout.String()
	if !strings.HasPrefix(printed, "\r") {
		t.Errorf("the bar was not taken down first: %q", printed)
	}
	if !strings.Contains(printed, "a.txt") {
		t.Errorf("the message never appeared: %q", printed)
	}
	if strings.Contains(printed, "█") {
		t.Errorf("the bar was drawn into the message: %q", printed)
	}
}

func TestProgressTakesEveryWorkerAtOnce(t *testing.T) {
	bar := stalled(100, 10000)
	bar.live = false

	var group sync.WaitGroup

	for range 8 {
		group.Add(1)

		go func() {
			defer group.Done()

			for range 100 {
				bar.AddBytes(10)
				bar.Advance()
			}
		}()
	}

	group.Wait()

	if got := bar.bytes.Load(); got != 8000 {
		t.Errorf("bytes = %d, want 8000", got)
	}
	if got := bar.files.Load(); got != 800 {
		t.Errorf("files = %d, want 800", got)
	}
}

func TestTheTimerRedrawsOnItsOwn(t *testing.T) {
	stdout, _ := capture(t)

	bar := &Progress{
		live:       true,
		started:    time.Now(),
		totalFiles: 1,
		totalBytes: 1000,
		stop:       make(chan struct{}),
		stopped:    make(chan struct{}),
	}

	go bar.run()

	for range 4 {
		bar.AddBytes(100)
		time.Sleep(redrawEvery)
	}

	bar.Stop()

	drawn := stdout.String()
	if !strings.Contains(drawn, "█") {
		t.Fatalf("the timer never drew anything: %q", drawn)
	}
	if !strings.Contains(drawn, "0/1 files") {
		t.Errorf("the bar moved without a file finishing, but does not say so: %q", drawn)
	}
	if got := strings.Count(drawn, "["); got < 2 {
		t.Errorf("drew %d times, want the bar to keep up on its own", got)
	}
}
