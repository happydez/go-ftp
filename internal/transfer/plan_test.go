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

func summary(plan Plan, stripLocal string) string {
	lines := make([]string, 0, len(plan.Jobs))

	for _, job := range plan.Jobs {
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

	plan, err := PlanUpload(resolve("/my", "/"), root, true)
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"/my/a.txt<-a.txt",
		"/my/sub/b.txt<-sub/b.txt",
		"/my/sub/deep/c.txt<-sub/deep/c.txt",
		"/my/sub/deep/d.bin<-sub/deep/d.bin",
	}, "\n")

	if got := summary(plan, root); got != want {
		t.Errorf("plan:\n%s\nwant:\n%s", got, want)
	}
	if total := plan.TotalSize(); total != 10 {
		t.Errorf("total size = %d, want 10", total)
	}
}

func TestPlanUploadPutsTheTreeUnderTheNamedDirectory(t *testing.T) {
	root := tree(t, map[string]string{"a.txt": "a"})

	plan, err := PlanUpload(resolve("/my", "releases/v1"), root, true)
	if err != nil {
		t.Fatal(err)
	}

	if got := summary(plan, root); got != "/my/releases/v1/a.txt<-a.txt" {
		t.Errorf("plan = %q", got)
	}
}

func TestPlanUploadOfASingleFileKeepsItsName(t *testing.T) {
	root := tree(t, map[string]string{"sub/report.txt": "hello"})

	plan, err := PlanUpload(resolve("/my", "/archive"), filepath.Join(root, "sub", "report.txt"), true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(plan.Jobs))
	}
	if plan.Jobs[0].Remote != "/my/archive/report.txt" {
		t.Errorf("remote = %q, want /my/archive/report.txt", plan.Jobs[0].Remote)
	}
	if plan.Jobs[0].Size != 5 {
		t.Errorf("size = %d, want 5", plan.Jobs[0].Size)
	}
}

func TestPlanUploadReportsAMissingSource(t *testing.T) {
	_, err := PlanUpload(resolve("/my", "/"), filepath.Join(t.TempDir(), "nope"), true)
	if err == nil {
		t.Fatal("a source that is not there should be an error")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q should name the path", err)
	}
}

func TestPlanUploadOfAnEmptyDirectoryHasNothingToDo(t *testing.T) {
	plan, err := PlanUpload(resolve("/my", "/"), t.TempDir(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Jobs) != 0 {
		t.Errorf("got %d jobs, want none", len(plan.Jobs))
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
	plan, err := PlanDownload(t.Context(), remoteTree(), resolve("/my", "/"), "out", true)
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"/my/a.txt<-out/a.txt",
		"/my/sub/b.txt<-out/sub/b.txt",
		"/my/sub/deep/c.txt<-out/sub/deep/c.txt",
	}, "\n")

	if got := summary(plan, ""); got != want {
		t.Errorf("plan:\n%s\nwant:\n%s", got, want)
	}
	if total := plan.TotalSize(); total != 6 {
		t.Errorf("total size = %d, want 6", total)
	}
}

func TestPlanDownloadOfOneFileLandsInTheLocalDirectory(t *testing.T) {
	plan, err := PlanDownload(t.Context(), remoteTree(), resolve("/my", "./sub/b.txt"), ".", true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Jobs) != 1 {
		t.Fatalf("got %d jobs, want 1", len(plan.Jobs))
	}
	if plan.Jobs[0].Remote != "/my/sub/b.txt" {
		t.Errorf("remote = %q, want /my/sub/b.txt", plan.Jobs[0].Remote)
	}
	if plan.Jobs[0].Local != filepath.FromSlash("b.txt") {
		t.Errorf("local = %q, want b.txt", plan.Jobs[0].Local)
	}
}

func TestPlanDownloadOfASubdirectoryDropsThePrefix(t *testing.T) {
	plan, err := PlanDownload(t.Context(), remoteTree(), resolve("/my", "sub"), "out", true)
	if err != nil {
		t.Fatal(err)
	}

	want := strings.Join([]string{
		"/my/sub/b.txt<-out/b.txt",
		"/my/sub/deep/c.txt<-out/deep/c.txt",
	}, "\n")

	if got := summary(plan, ""); got != want {
		t.Errorf("plan:\n%s\nwant:\n%s", got, want)
	}
}

func TestPlanDownloadReportsAMissingRemotePath(t *testing.T) {
	_, err := PlanDownload(t.Context(), remoteTree(), resolve("/my", "nope.txt"), ".", true)
	if err == nil {
		t.Fatal("a path that is not on the server should be an error")
	}
	if !strings.Contains(err.Error(), "/my/nope.txt") {
		t.Errorf("error %q should name the resolved path", err)
	}
}

func TestPlanUploadKeepsEmptyDirectories(t *testing.T) {
	root := tree(t, map[string]string{"full/a.txt": "a"})

	if err := os.MkdirAll(filepath.Join(root, "empty", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanUpload(resolve("/my", "/"), root, true)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(plan.Dirs, ","); got != "/my/empty,/my/empty/nested" {
		t.Errorf("dirs = %q, want both empty levels", got)
	}

	for _, dir := range plan.Dirs {
		if dir == "/my/full" {
			t.Error("/my/full holds a file and does not need creating on its own")
		}
	}
}

func TestPlanUploadWithNothingButEmptyDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanUpload(resolve("/my", "/"), root, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Jobs) != 0 {
		t.Errorf("jobs = %v, want none", plan.Jobs)
	}
	if plan.Empty() {
		t.Error("a plan that creates a directory is not empty")
	}
}

func TestPlanDownloadKeepsEmptyDirectories(t *testing.T) {
	lister := &fakeLister{entries: []ftpx.Entry{
		{Path: "/my", Name: "my", Dir: true},
		{Path: "/my/full", Name: "full", Dir: true},
		{Path: "/my/full/a.txt", Name: "a.txt", Size: 1},
		{Path: "/my/empty", Name: "empty", Dir: true},
		{Path: "/my/empty/nested", Name: "nested", Dir: true},
	}}

	plan, err := PlanDownload(t.Context(), lister, resolve("/my", "/"), "out", true)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join("out", "empty") + "," + filepath.Join("out", "empty", "nested")
	if got := strings.Join(plan.Dirs, ","); got != want {
		t.Errorf("dirs = %q, want %q", got, want)
	}
}

func TestPlanUploadReportsWhatItCannotSend(t *testing.T) {
	root := tree(t, map[string]string{"a.txt": "a"})

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(root, "a.txt"), link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	plan, err := PlanUpload(resolve("/my", "/"), root, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Jobs) != 1 {
		t.Errorf("jobs = %v, want only the plain file", plan.Jobs)
	}
	if got := strings.Join(plan.Ignored, ","); got != link {
		t.Errorf("ignored = %q, want %q", got, link)
	}
}

func TestPlanUploadOfASymlinkNamedDirectly(t *testing.T) {
	root := tree(t, map[string]string{"a.txt": "a"})

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(filepath.Join(root, "a.txt"), link); err != nil {
		t.Skipf("symlinks are not available here: %v", err)
	}

	plan, err := PlanUpload(resolve("/my", "/"), link, true)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Jobs) != 0 {
		t.Errorf("jobs = %v, want nothing to send", plan.Jobs)
	}
	if len(plan.Ignored) != 1 {
		t.Errorf("ignored = %v, want the symlink named", plan.Ignored)
	}
}

func resolve(base, input string) string {
	resolved, err := Resolve(base, input)
	if err != nil {
		panic(err)
	}

	return resolved
}

func TestPlanDownloadCarriesTheDirectoryNameOver(t *testing.T) {
	plan, err := PlanDownload(t.Context(), remoteTree(), "/my/sub", "out", false)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join("out", "sub", "b.txt")
	if plan.Jobs[0].Local != want {
		t.Errorf("local = %q, want %q", plan.Jobs[0].Local, want)
	}
}
