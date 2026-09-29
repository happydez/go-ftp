package ui

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

const barWidth = 24

// redrawEvery is often enough to look alive and rare enough that the drawing
// costs nothing next to the transfer itself.
const redrawEvery = 100 * time.Millisecond

// Progress draws one line and rewrites it in place. The bytes are counted as
// they travel rather than when a file finishes, so a single large file shows
// something moving instead of nothing at all.
type Progress struct {
	live       bool
	started    time.Time
	totalFiles int
	totalBytes int64

	files atomic.Int64
	bytes atomic.Int64

	mu    sync.Mutex
	drawn int

	stopOnce sync.Once
	stop     chan struct{}
	stopped  chan struct{}
}

// NewProgress returns a bar for a run of this size and starts drawing it.
func NewProgress(totalFiles int, totalBytes int64) *Progress {
	p := &Progress{
		live:       onTerminal() && verbosity >= Normal && totalFiles > 0,
		started:    time.Now(),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
		stop:       make(chan struct{}),
		stopped:    make(chan struct{}),
	}

	if !p.live {
		close(p.stopped)

		return p
	}

	p.redraw()

	go p.run()

	return p
}

// AddBytes records bytes that just went over the wire. It only counts, since
// drawing on every read would cost more than the transfer.
func (p *Progress) AddBytes(n int64) {
	if p == nil {
		return
	}

	p.bytes.Add(n)
}

// Advance records one finished file.
func (p *Progress) Advance() {
	if p == nil {
		return
	}

	p.files.Add(1)
}

// Line prints a message above the bar.
func (p *Progress) Line(print func()) {
	if p == nil || !p.live {
		print()

		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.clearLocked()
	print()
}

// Stop takes the bar down for good, at the end of a run. Calling it twice is
// safe.
func (p *Progress) Stop() {
	if p == nil || !p.live {
		return
	}

	p.stopOnce.Do(func() {
		close(p.stop)
	})
	<-p.stopped

	p.mu.Lock()
	p.clearLocked()
	p.mu.Unlock()
}

func (p *Progress) run() {
	ticker := time.NewTicker(redrawEvery)

	defer ticker.Stop()
	defer close(p.stopped)

	for {
		select {
		case <-p.stop:
			return
		case <-ticker.C:
			p.redraw()
		}
	}
}

func (p *Progress) redraw() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.clearLocked()

	line := p.render()
	p.drawn = len([]rune(line))

	_, _ = fmt.Fprint(out, color.New(color.FgCyan).Sprint(line))
}

// clearLocked wipes the drawn line. The caller holds the mutex.
func (p *Progress) clearLocked() {
	if p.drawn == 0 {
		return
	}

	_, _ = fmt.Fprintf(out, "\r%s\r", strings.Repeat(" ", p.drawn))
	p.drawn = 0
}

func (p *Progress) render() string {
	files := int(p.files.Load())
	bytes := p.bytes.Load()

	shown := min(bytes, p.totalBytes)

	done := float64(files) / float64(p.totalFiles)
	if p.totalBytes > 0 {
		done = float64(shown) / float64(p.totalBytes)
	}
	done = min(max(done, 0), 1)

	bar := strings.Repeat("█", int(done*barWidth))

	elapsed := time.Since(p.started).Seconds()

	rate := "--"
	if elapsed > 0 && bytes > 0 {
		rate = Bytes(int64(float64(bytes)/elapsed)) + "/s"
	}

	return fmt.Sprintf("[%-*s] %d/%d files  %s/%s  %s",
		barWidth, bar, files, p.totalFiles, Bytes(shown), Bytes(p.totalBytes), rate)
}

// onTerminal reports whether the normal output stream is a terminal that can
// have a line rewritten on it.
func onTerminal() bool {
	stdout, ok := out.(*os.File)
	if !ok {
		return false
	}

	return term.IsTerminal(int(stdout.Fd()))
}
