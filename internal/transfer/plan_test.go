package transfer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/happydez/go-ftp/internal/ftpx"
)

// tree builds a local directory from a map of relative path to contents.
func tree(t *testing.T, files map[string]string) string {
	t.Helper()

	root := t.TempDir()

	for name, body := range files {
		full := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func summary(jobs []Job, stripLocal string) string {
	lines := make([]string, 0, len(jobs))

	for _, job := range jobs {
		local := job.Local
		if stripLocal != "" {
			local = strings.TrimPrefix(local, stripLocal+string(filepath.Separator))
		}
		lines = append(lines, fmt.Sprintf("%s<-%s", job.Remote, filepath.ToSlash(local)))
	}

	return strings.Join(lines, "\n")
}

func TestPlanUploadSendsTheContentsOfADirectory(t *testing.T) {
	root := tree(t, map[string]string{
		"a.txt":          "a",
		"sub/b.txt":      "bb",
		"sub/deep/c.txt": "ccc",
		"sub/deep/d.bin": "dddd",
	})

	jobs, err := PlanUpload("/my", root, "/")
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"/my/a.txt<-a.txt",
		"/my/sub/b.txt<-sub/b.txt",
		"/my/sub/deep/c.txt<-sub/deep/c.txt",
		"/my/sub/deep/d.bin<-sub/deep/d.bin",
	}, "\n")

	if got := summary(jobs, root); got != want {
		t.Errorf("plan:\n%s\nwant:\n%s", got, want)
	}
	if total := TotalSize(jobs); total != 10 {
		t.Errorf("total size = %d, want 10", total)
	}
}

func TestPlanUploadPutsTheTreeUnderTheNamedDirectory(t *testing.T) {
	root := tree(t, map[string]string{"a.txt": "a"})

	jobs, err := PlanUpload("/my", root, "releases/v1")
	if err != nil {
		t.Fatal(err)
	}

	if got := summary(jobs, root); got != "/my/releases/v1/a.txt<-a.txt" {
		t.Errorf("plan = %q", got)
	}
}

func TestPlanUploadOfASingleFileKeepsItsName(t *testing.T) {
	root := tree(t, map[string]string{"sub/report.txt": "hello"})

	jobs, err := PlanUpload("/my", filepath.Join(root, "sub", "report.txt"), "/archive")
	if err != nil {
		t.Fatal(err)
	}

	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Remote != "/my/archive/report.txt" {
		t.Errorf("remote = %q, want /my/archive/report.txt", jobs[0].Remote)
	}
	if jobs[0].Size != 5 {
		t.Errorf("size = %d, want 5", jobs[0].Size)
	}
}

func TestPlanUploadRefusesToLeaveTheBase(t *testing.T) {
	root := tree(t, map[string]string{"a.txt": "a"})

	if _, err := PlanUpload("/my", root, "../elsewhere"); err == nil {
		t.Error("a destination above the base should be refused")
	}
}

func TestPlanUploadReportsAMissingSource(t *testing.T) {
	_, err := PlanUpload("/my", filepath.Join(t.TempDir(), "nope"), "/")
	if err == nil {
		t.Fatal("a source that is not there should be an error")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q should name the path", err)
	}
}

func TestPlanUploadOfAnEmptyDirectoryHasNothingToDo(t *testing.T) {
	jobs, err := PlanUpload("/my", t.TempDir(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Errorf("got %d jobs, want none", len(jobs))
	}
}

type fakeLister struct {
	entries []ftpx.Entry
}

func (f *fakeLister) Stat(_ context.Context, remotePath string) (ftpx.Entry, bool, error) {
	for _, entry := range f.entries {
		if entry.Path == remotePath {
			return entry, true, nil
		}
	}

	return ftpx.Entry{}, false, nil
}

func (f *fakeLister) Walk(_ context.Context, dir string) ([]ftpx.Entry, error) {
	var under []ftpx.Entry

	for _, entry := range f.entries {
		if entry.Path != dir && strings.HasPrefix(entry.Path, strings.TrimSuffix(dir, "/")+"/") {
			under = append(under, entry)
		}
	}

	return under, nil
}

func remoteTree() *fakeLister {
	return &fakeLister{entries: []ftpx.Entry{
		{Path: "/my", Name: "my", Dir: true},
		{Path: "/my/a.txt", Name: "a.txt", Size: 1},
		{Path: "/my/sub", Name: "sub", Dir: true},
		{Path: "/my/sub/b.txt", Name: "b.txt", Size: 2},
		{Path: "/my/sub/deep", Name: "deep", Dir: true},
		{Path: "/my/sub/deep/c.txt", Name: "c.txt", Size: 3},
	}}
}

func TestPlanDownloadMirrorsTheRemoteTree(t *testing.T) {
	jobs, err := PlanDownload(t.Context(), remoteTree(), "/my", "/", "out")
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"/my/a.txt<-out/a.txt",
		"/my/sub/b.txt<-out/sub/b.txt",
		"/my/sub/deep/c.txt<-out/sub/deep/c.txt",
	}, "\n")

	if got := summary(jobs, ""); got != want {
		t.Errorf("plan:\n%s\nwant:\n%s", got, want)
	}
	if total := TotalSize(jobs); total != 6 {
		t.Errorf("total size = %d, want 6", total)
	}
}

func TestPlanDownloadOfOneFileLandsInTheLocalDirectory(t *testing.T) {
	jobs, err := PlanDownload(t.Context(), remoteTree(), "/my", "./sub/b.txt", ".")
	if err != nil {
		t.Fatal(err)
	}

	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(jobs))
	}
	if jobs[0].Remote != "/my/sub/b.txt" {
		t.Errorf("remote = %q, want /my/sub/b.txt", jobs[0].Remote)
	}
	if jobs[0].Local != filepath.FromSlash("b.txt") {
		t.Errorf("local = %q, want b.txt", jobs[0].Local)
	}
}

func TestPlanDownloadOfASubdirectoryDropsThePrefix(t *testing.T) {
	jobs, err := PlanDownload(t.Context(), remoteTree(), "/my", "sub", "out")
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"/my/sub/b.txt<-out/b.txt",
		"/my/sub/deep/c.txt<-out/deep/c.txt",
	}, "\n")

	if got := summary(jobs, ""); got != want {
		t.Errorf("plan:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanDownloadReportsAMissingRemotePath(t *testing.T) {
	_, err := PlanDownload(t.Context(), remoteTree(), "/my", "nope.txt", ".")
	if err == nil {
		t.Fatal("a path that is not on the server should be an error")
	}
	if !strings.Contains(err.Error(), "/my/nope.txt") {
		t.Errorf("error %q should name the resolved path", err)
	}
}

func TestPlanDownloadRefusesToLeaveTheBase(t *testing.T) {
	if _, err := PlanDownload(t.Context(), remoteTree(), "/my", "../etc", "."); err == nil {
		t.Error("a source above the base should be refused")
	}
}
