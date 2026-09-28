package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/fatih/color"
)

// capture points both streams at buffers and puts everything back afterwards.
func capture(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()

	oldOut, oldErr, oldVerbosity, oldNoColor := out, errOut, verbosity, color.NoColor
	t.Cleanup(func() {
		out, errOut, verbosity, color.NoColor = oldOut, oldErr, oldVerbosity, oldNoColor
	})

	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	SetOutput(stdout, stderr)

	return stdout, stderr
}

func TestTagsAreColouredUnlessDisabled(t *testing.T) {
	stdout, _ := capture(t)

	color.NoColor = false
	OK("uploaded %s", "a.txt")
	if !strings.Contains(stdout.String(), "\x1b[") {
		t.Fatalf("expected ANSI codes, got %q", stdout.String())
	}

	stdout.Reset()
	SetColor(false)
	OK("uploaded %s", "a.txt")
	if got := stdout.String(); got != "OK   uploaded a.txt\n" {
		t.Fatalf("expected a plain line, got %q", got)
	}
}

func TestQuietHidesProgressButNotFailures(t *testing.T) {
	stdout, stderr := capture(t)
	SetVerbosity(Quiet)

	OK("uploaded a.txt")
	Fail("b.txt: connection reset")

	if stdout.Len() != 0 {
		t.Fatalf("quiet should print no progress, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "b.txt") {
		t.Fatalf("quiet must still report failures, got %q", stderr.String())
	}
}

func TestDebugOnlyWhenVerbose(t *testing.T) {
	_, stderr := capture(t)

	Debug("connecting to %s", "127.0.0.1:21")
	if stderr.Len() != 0 {
		t.Fatalf("debug should be silent at normal verbosity, got %q", stderr.String())
	}

	SetVerbosity(Verbose)
	Debug("connecting to %s", "127.0.0.1:21")
	if !strings.Contains(stderr.String(), "127.0.0.1:21") {
		t.Fatalf("debug should print when verbose, got %q", stderr.String())
	}
}
