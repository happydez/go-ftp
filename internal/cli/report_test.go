package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/happydez/go-ftp/internal/transfer"
)

func planned() transfer.Plan {
	return transfer.Plan{Jobs: []transfer.Job{
		{Local: "src/a.txt", Remote: "/my/a.txt", Size: 10},
		{Local: "src/sub/b.txt", Remote: "/my/sub/b.txt", Size: 2038},
	}}
}

func TestPlanPointsTheArrowTheWayTheFilesTravel(t *testing.T) {
	var out bytes.Buffer
	if err := printPlan(&out, planned(), "upload", localSide, remoteSide); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if !strings.Contains(got, "would upload 2 file(s), 2.0 KB") {
		t.Errorf("the header is wrong:\n%s", got)
	}
	if !strings.Contains(got, "src/a.txt -> /my/a.txt") {
		t.Errorf("an upload should read local to remote:\n%s", got)
	}

	out.Reset()
	if err := printPlan(&out, planned(), "download", remoteSide, localSide); err != nil {
		t.Fatal(err)
	}

	if got := out.String(); !strings.Contains(got, "/my/a.txt -> src/a.txt") {
		t.Errorf("a download should read remote to local:\n%s", got)
	}
}

func TestSummaryErrorOnlyFiresWhenSomethingFailed(t *testing.T) {
	clean := transfer.Summary{Total: 5, Moved: 5}
	if err := summaryError(clean); err != nil {
		t.Errorf("error = %v, want none for a clean run", err)
	}

	broken := transfer.Summary{Total: 5, Moved: 3, Failed: 2}

	err := summaryError(broken)
	if err == nil {
		t.Fatal("a run that lost files should give an error")
	}
	if !strings.Contains(err.Error(), "2 of 5") {
		t.Errorf("error = %q, want the counts in it", err)
	}

	if _, ok := errors.AsType[usageError](err); ok {
		t.Error("a failed transfer is not a usage error")
	}
}
