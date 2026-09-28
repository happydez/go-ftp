package ftpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConnectRejectsAWrongPassword(t *testing.T) {
	addr, _ := startServer(t)

	client := New(Options{
		Addr:     addr,
		Host:     "127.0.0.1",
		User:     testUser,
		Password: "not the password",
	})
	defer client.Close()

	if err := client.Connect(t.Context()); err == nil {
		t.Fatal("a wrong password should not log in")
	}
}

func TestConnectIsIdempotent(t *testing.T) {
	client, _ := dial(t)

	for range 3 {
		if err := client.Connect(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestUploadThenDownloadRoundTrip(t *testing.T) {
	client, root := dial(t)
	ctx := t.Context()

	const body = "the quick brown fox\r\njumps over\x00the lazy dog"

	if err := client.Upload(ctx, "/hello.txt", strings.NewReader(body)); err != nil {
		t.Fatal(err)
	}

	onDisk, err := os.ReadFile(filepath.Join(root, "hello.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != body {
		t.Errorf("the server stored %q, want %q", onDisk, body)
	}

	reader, err := client.Download(ctx, "/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = reader.Close()
	}()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Errorf("downloaded %q, want %q", got, body)
	}
}

func TestDownloadingSomethingMissingFails(t *testing.T) {
	client, _ := dial(t)
	if _, err := client.Download(t.Context(), "/nope.txt"); err == nil {
		t.Error("downloading a file that is not there should fail")
	}
}

func TestEnsureDirCreatesEveryLevel(t *testing.T) {
	client, root := dial(t)
	ctx := t.Context()

	if err := client.EnsureDir(ctx, "/one/two/three"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filepath.Join(root, "one", "two", "three"))
	if err != nil {
		t.Fatal(err)
	}
	if !info.IsDir() {
		t.Error("the deepest level is not a directory")
	}

	if err := client.EnsureDir(ctx, "/one/two/three"); err != nil {
		t.Errorf("the second call failed: %v", err)
	}
	if err := client.EnsureDir(ctx, "/one/two/four"); err != nil {
		t.Errorf("a sibling failed: %v", err)
	}
}

func TestSizeSaysWhenThereIsNoFile(t *testing.T) {
	client, _ := dial(t)
	ctx := t.Context()

	if _, ok := client.Size(ctx, "/nope.txt"); ok {
		t.Error("a file that is not there should not report a size")
	}

	if err := client.Upload(ctx, "/small.txt", strings.NewReader("12345")); err != nil {
		t.Fatal(err)
	}

	size, ok := client.Size(ctx, "/small.txt")
	if !ok {
		t.Fatal("the uploaded file should report a size")
	}
	if size != 5 {
		t.Errorf("size = %d, want 5", size)
	}
}

func TestListReturnsFullPathsAndNoDotEntries(t *testing.T) {
	client, _ := dial(t)
	ctx := t.Context()

	if err := client.EnsureDir(ctx, "/dir/sub"); err != nil {
		t.Fatal(err)
	}
	if err := client.Upload(ctx, "/dir/a.txt", strings.NewReader("a")); err != nil {
		t.Fatal(err)
	}

	entries, err := client.List(ctx, "/dir")
	if err != nil {
		t.Fatal(err)
	}

	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Name == "." || entry.Name == ".." {
			t.Errorf("%q should have been filtered out", entry.Name)
		}
		paths = append(paths, entry.Path)
	}

	if got := strings.Join(paths, ","); got != "/dir/a.txt,/dir/sub" {
		t.Errorf("paths = %q, want /dir/a.txt,/dir/sub", got)
	}

	for _, entry := range entries {
		if entry.Name == "sub" && !entry.Dir {
			t.Error("sub should be reported as a directory")
		}
		if entry.Name == "a.txt" && entry.Dir {
			t.Error("a.txt should not be reported as a directory")
		}
	}
}

func TestWalkGoesAllTheWayDown(t *testing.T) {
	client, _ := dial(t)
	ctx := t.Context()

	if err := client.EnsureDir(ctx, "/tree/deep/deeper"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/tree/top.txt", "/tree/deep/middle.txt", "/tree/deep/deeper/bottom.txt"} {
		if err := client.Upload(ctx, path, strings.NewReader(path)); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := client.Walk(ctx, "/tree")
	if err != nil {
		t.Fatal(err)
	}

	var files []string
	for _, entry := range entries {
		if !entry.Dir {
			files = append(files, entry.Path)
		}
	}

	want := "/tree/deep/deeper/bottom.txt,/tree/deep/middle.txt,/tree/top.txt"
	if got := strings.Join(files, ","); got != want {
		t.Errorf("files = %q, want %q", got, want)
	}
}

func TestStatDescribesFilesAndDirectories(t *testing.T) {
	client, _ := dial(t)
	ctx := t.Context()

	if err := client.EnsureDir(ctx, "/things"); err != nil {
		t.Fatal(err)
	}
	if err := client.Upload(ctx, "/things/file.txt", strings.NewReader("12345")); err != nil {
		t.Fatal(err)
	}

	file, ok, err := client.Stat(ctx, "/things/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("the uploaded file was not found")
	}
	if file.Dir {
		t.Error("a file was reported as a directory")
	}
	if file.Size != 5 {
		t.Errorf("size = %d, want 5", file.Size)
	}

	dir, ok, err := client.Stat(ctx, "/things")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !dir.Dir {
		t.Errorf("/things = %+v, want an existing directory", dir)
	}

	if _, ok, err = client.Stat(ctx, "/things/nope.txt"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("a missing file should not be found")
	}
}

func TestStatKnowsTheRoot(t *testing.T) {
	client, _ := dial(t)

	entry, ok, err := client.Stat(t.Context(), "/")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !entry.Dir {
		t.Errorf("/ = %+v, want an existing directory", entry)
	}
}

func TestUploadStopsOnACancelledContext(t *testing.T) {
	client, _ := dial(t)

	if err := client.Connect(t.Context()); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := client.Upload(ctx, "/big.bin", bytes.NewReader(make([]byte, 1<<20)))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("error = %v, want context.Canceled", err)
	}
}

func TestResetForgetsTheDirectoryCache(t *testing.T) {
	client, root := dial(t)
	ctx := t.Context()

	if err := client.EnsureDir(ctx, "/cached"); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(root, "cached")); err != nil {
		t.Fatal(err)
	}

	client.Reset()

	if err := client.EnsureDir(ctx, "/cached"); err != nil {
		t.Fatalf("the directory was not created again: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cached")); err != nil {
		t.Error(err)
	}
}
