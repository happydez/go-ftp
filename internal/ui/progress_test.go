package ui

import (
	"strings"
	"testing"
	"time"
)

func TestProgressDrawsNothingWhenOutputIsNotATerminal(t *testing.T) {
	stdout, _ := capture(t)

	bar := NewProgress(10, 1000)
	bar.Advance(100)
	bar.Advance(100)
	bar.Stop()

	if stdout.Len() != 0 {
		t.Errorf("wrote %q, want nothing outside a terminal", stdout.String())
	}
}

func TestProgressIsSafeOnANilReceiver(t *testing.T) {
	var bar *Progress
	bar.Advance(1)
	bar.Clear()
	bar.Stop()
}

func TestProgressLine(t *testing.T) {
	bar := &Progress{
		live:       true,
		started:    time.Now().Add(-2 * time.Second),
		totalFiles: 10,
		totalBytes: 10 << 20,
		files:      5,
		bytes:      2 << 20,
	}

	line := bar.render()

	for _, want := range []string{"5/10 files", "2.0 MB/10.0 MB", "/s"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line is missing %q:\n%s", want, line)
		}
	}

	inside := line[strings.Index(line, "[")+1 : strings.Index(line, "]")]
	if len(inside) != barWidth {
		t.Errorf("the bar is %d wide, want %d", len(inside), barWidth)
	}
	if got := strings.Count(inside, "="); got == 0 || got >= barWidth {
		t.Errorf("the bar reads %q, want it part filled", inside)
	}
	if !strings.Contains(inside, ">") {
		t.Errorf("the bar reads %q, want a leading edge", inside)
	}
}

func TestProgressLineAtBothEnds(t *testing.T) {
	bar := &Progress{live: true, started: time.Now(), totalFiles: 4, totalBytes: 100}

	empty := bar.render()
	if strings.Contains(empty, "=") || strings.Contains(empty, ">") {
		t.Errorf("nothing done yet, but the bar reads %q", empty)
	}
	if !strings.Contains(empty, "0/4 files") {
		t.Errorf("the line does not say nothing is done:\n%s", empty)
	}

	bar.files, bar.bytes = 4, 100

	full := bar.render()
	if strings.Contains(full, ">") {
		t.Errorf("a finished bar should have no leading edge: %q", full)
	}
	if !strings.Contains(full, "4/4 files") {
		t.Errorf("the line does not say everything is done:\n%s", full)
	}
}

func TestProgressRateIsHeldBackUntilThereAreBytes(t *testing.T) {
	bar := &Progress{live: true, started: time.Now(), totalFiles: 2, totalBytes: 10}

	if !strings.Contains(bar.render(), "--") {
		t.Errorf("a rate from no bytes and no time is a guess, want -- instead:\n%s", bar.render())
	}
}

func TestProgressRewritesOneLineAndCleansUp(t *testing.T) {
	stdout, _ := capture(t)

	bar := NewProgress(4, 400)
	bar.live = true

	bar.Advance(100)
	first := stdout.String()

	if strings.Contains(first, "\n") {
		t.Errorf("the bar must stay on one line, got %q", first)
	}
	if !strings.Contains(first, "1/4 files") {
		t.Errorf("the bar does not show the progress: %q", first)
	}

	stdout.Reset()
	bar.Advance(100)

	second := stdout.String()
	if !strings.HasPrefix(second, "\r") {
		t.Errorf("a redraw should start by going back to the start of the line: %q", second)
	}
	if !strings.Contains(second, "2/4 files") {
		t.Errorf("the bar did not move on: %q", second)
	}

	stdout.Reset()
	bar.Stop()

	if last := stdout.String(); !strings.HasSuffix(last, "\r") {
		t.Errorf("stopping should leave the line empty and the cursor at its start: %q", last)
	}
}

func TestProgressClearIsIdempotent(t *testing.T) {
	stdout, _ := capture(t)

	bar := NewProgress(2, 20)
	bar.live = true
	bar.Advance(10)

	stdout.Reset()
	bar.Clear()
	first := stdout.Len()

	bar.Clear()
	if stdout.Len() != first {
		t.Error("clearing twice should write nothing the second time")
	}
}
