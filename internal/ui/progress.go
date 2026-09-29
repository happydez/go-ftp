package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"golang.org/x/term"
)

const barWidth = 24

// Progress draws one line and rewrites it in place.
type Progress struct {
	live       bool
	started    time.Time
	totalFiles int
	totalBytes int64
	files      int
	bytes      int64
	drawn      int
}

// NewProgress returns a bar for a run of this size.
func NewProgress(totalFiles int, totalBytes int64) *Progress {
	return &Progress{
		live:       onTerminal() && verbosity >= Normal && totalFiles > 0,
		started:    time.Now(),
		totalFiles: totalFiles,
		totalBytes: totalBytes,
	}
}

// Advance records one finished file and redraws.
func (p *Progress) Advance(bytes int64) {
	if p == nil {
		return
	}

	p.files++
	p.bytes += bytes
	p.draw()
}

// Clear wipes the line so that an ordinary message can be printed over it. The
// next Advance puts the bar back.
func (p *Progress) Clear() {
	if p == nil || !p.live || p.drawn == 0 {
		return
	}

	_, _ = fmt.Fprintf(out, "\r%s\r", strings.Repeat(" ", p.drawn))
	p.drawn = 0
}

// Stop takes the bar down for good, at the end of a run.
func (p *Progress) Stop() {
	if p == nil {
		return
	}

	p.Clear()
	p.live = false
}

func (p *Progress) draw() {
	if !p.live {
		return
	}

	p.Clear()

	line := p.render()
	p.drawn = len([]rune(line))

	_, _ = fmt.Fprint(out, color.New(color.FgCyan).Sprint(line))
}

func (p *Progress) render() string {
	done := float64(p.files) / float64(p.totalFiles)
	filled := int(done * barWidth)

	bar := strings.Repeat("█", max(filled-1, 0)) // =
	if filled > 0 && filled < barWidth {
		bar += "█" // >
	} else if filled >= barWidth {
		bar += "█" // =
	}

	elapsed := time.Since(p.started).Seconds()

	rate := "--"
	if elapsed > 0 && p.bytes > 0 {
		rate = Bytes(int64(float64(p.bytes)/elapsed)) + "/s"
	}

	return fmt.Sprintf("[%-*s] %d/%d files  %s/%s  %s",
		barWidth, bar, p.files, p.totalFiles, Bytes(p.bytes), Bytes(p.totalBytes), rate)
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
