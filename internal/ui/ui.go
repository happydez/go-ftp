// Package ui prints everything the user sees.
package ui

import (
	"fmt"
	"io"
	"os"

	"github.com/fatih/color"
)

// Verbosity decides which of the printers below write anything. Failures and
// errors ignore it, because hiding them is never what the user meant.
type Verbosity int

const (
	Quiet Verbosity = iota - 1
	Normal
	Verbose
)

var (
	out       io.Writer = os.Stdout
	errOut    io.Writer = os.Stderr
	verbosity           = Normal
)

var (
	okTag   = color.New(color.FgGreen, color.Bold)
	skipTag = color.New(color.FgYellow, color.Bold)
	failTag = color.New(color.FgRed, color.Bold)
	bold    = color.New(color.Bold)
	faint   = color.New(color.Faint)
	accent  = color.New(color.FgCyan)
)

// SetColor can only ever turn colour off. fatih/color already disables itself
// for a non-terminal and for NO_COLOR.
func SetColor(enabled bool) {
	if !enabled {
		color.NoColor = true
	}
}

func SetVerbosity(v Verbosity) {
	verbosity = v
}

// SetOutput redirects both streams, for tests.
func SetOutput(stdout, stderr io.Writer) {
	out, errOut = stdout, stderr
}

func OK(format string, a ...any) {
	if verbosity >= Normal {
		tagged(out, okTag, "OK", format, a...)
	}
}

func Skip(format string, a ...any) {
	if verbosity >= Normal {
		tagged(out, skipTag, "SKIP", format, a...)
	}
}

// Fail is one file that did not make it. The run carries on.
func Fail(format string, a ...any) {
	tagged(errOut, failTag, "FAIL", format, a...)
}

// Error is the thing that stopped the whole command.
func Error(format string, a ...any) {
	_, _ = failTag.Fprint(errOut, "error: ")
	_, _ = fmt.Fprintf(errOut, format+"\n", a...)
}

func Warn(format string, a ...any) {
	_, _ = skipTag.Fprint(errOut, "warning: ")
	_, _ = fmt.Fprintf(errOut, format+"\n", a...)
}

func Header(format string, a ...any) {
	if verbosity >= Normal {
		_, _ = bold.Fprintf(out, format+"\n", a...)
	}
}

func Info(format string, a ...any) {
	if verbosity >= Normal {
		_, _ = fmt.Fprintf(out, format+"\n", a...)
	}
}

func Debug(format string, a ...any) {
	if verbosity >= Verbose {
		_, _ = faint.Fprintf(errOut, format+"\n", a...)
	}
}

// Path marks a file name inside a sentence.
func Path(s string) string {
	return accent.Sprint(s)
}

// Bold marks a word inside a sentence.
func Bold(s string) string {
	return bold.Sprint(s)
}

// tagged prints the status column padded to the longest tag, so that a long run
// stays aligned.
func tagged(w io.Writer, c *color.Color, tag, format string, a ...any) {
	_, _ = c.Fprintf(w, "%-4s ", tag)
	_, _ = fmt.Fprintf(w, format+"\n", a...)
}

// Bytes renders a size the way a person reads it.
func Bytes(n int64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%d B", n)
	}

	value := float64(n)

	for _, name := range []string{"KB", "MB", "GB", "TB", "PB"} {
		value /= unit
		if value < unit {
			return fmt.Sprintf("%.1f %s", value, name)
		}
	}

	return fmt.Sprintf("%.1f EB", value/unit)
}
