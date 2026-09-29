package transfer

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// opsConn serves file contents from a map and records the uploads.
type opsConn struct {
	fakeConn
	contents map[string]string
	// readErr interrupts a download part way through, the way a dropped
	// connection would.
	readErr error
}

func (c *opsConn) Download(_ context.Context, remotePath string) (io.ReadCloser, error) {
	body, ok := c.contents[remotePath]
	if !ok {
		return nil, errors.New("no such file")
	}

	if c.readErr != nil {
		return io.NopCloser(io.MultiReader(
			strings.NewReader(body),
			brokenReader{err: c.readErr},
		)), nil
	}

	return io.NopCloser(strings.NewReader(body)), nil
}

type brokenReader struct {
	err error
}

func (r brokenReader) Read([]byte) (int, error) {
	return 0, r.err
}

func TestUploaderSendsTheFile(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := &fakeConn{}

	outcome, err := Uploader(StreamOptions{InPlace: true})(t.Context(), conn, Job{
		Local:  local,
		Remote: "/my/a.txt",
		Size:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != Moved {
		t.Errorf("outcome = %v, want Moved", outcome)
	}
	if got := strings.Join(conn.uploaded, ","); got != "/my/a.txt" {
		t.Errorf("uploaded %q, want /my/a.txt", got)
	}
}

func TestUploaderReportsAMissingLocalFile(t *testing.T) {
	_, err := Uploader(StreamOptions{InPlace: true})(t.Context(), &fakeConn{}, Job{
		Local:  filepath.Join(t.TempDir(), "gone.txt"),
		Remote: "/my/gone.txt",
	})
	if err == nil {
		t.Fatal("a file that is not there should be an error")
	}
}

func TestUploaderSkipsAFileOfTheSameSize(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := &fakeConn{sizes: map[string]int64{"/my/a.txt": 5}}

	outcome, err := Uploader(StreamOptions{SkipExisting: true, InPlace: true})(t.Context(), conn, Job{
		Local:  local,
		Remote: "/my/a.txt",
		Size:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != Skipped {
		t.Errorf("outcome = %v, want Skipped", outcome)
	}
	if len(conn.uploaded) != 0 {
		t.Errorf("uploaded %v, want nothing", conn.uploaded)
	}
}

func TestUploaderResendsAFileOfADifferentSize(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(local, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	// A half uploaded file from an earlier run.
	conn := &fakeConn{sizes: map[string]int64{"/my/a.txt": 2}}

	outcome, err := Uploader(StreamOptions{SkipExisting: true, InPlace: true})(t.Context(), conn, Job{
		Local:  local,
		Remote: "/my/a.txt",
		Size:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != Moved {
		t.Errorf("outcome = %v, want the file to be sent again", outcome)
	}
}

func TestDownloaderWritesTheFileAndItsDirectories(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "sub", "deep", "b.txt")

	conn := &opsConn{contents: map[string]string{"/my/sub/deep/b.txt": "contents"}}

	outcome, err := Downloader(StreamOptions{})(t.Context(), conn, Job{
		Local:  local,
		Remote: "/my/sub/deep/b.txt",
		Size:   8,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != Moved {
		t.Errorf("outcome = %v, want Moved", outcome)
	}

	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "contents" {
		t.Errorf("file holds %q, want contents", got)
	}
}

func TestDownloaderLeavesNothingBehindWhenItFails(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "b.txt")

	conn := &opsConn{
		contents: map[string]string{"/my/b.txt": "the first half"},
		readErr:  errors.New("connection reset"),
	}

	if _, err := Downloader(StreamOptions{})(t.Context(), conn, Job{Local: local, Remote: "/my/b.txt"}); err == nil {
		t.Fatal("a broken transfer should be an error")
	}

	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Error("a half file was left where a finished one should be")
	}

	left, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 0 {
		t.Errorf("the directory holds %d leftovers, want none", len(left))
	}
}

func TestDownloaderReplacesAnExistingFile(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(local, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := &opsConn{contents: map[string]string{"/my/b.txt": "new contents"}}

	if _, err := Downloader(StreamOptions{})(t.Context(), conn, Job{Local: local, Remote: "/my/b.txt"}); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new contents" {
		t.Errorf("file holds %q, want the new contents", got)
	}
}

func TestDownloaderSkipsAFileOfTheSameSize(t *testing.T) {
	dir := t.TempDir()
	local := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(local, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}

	conn := &opsConn{contents: map[string]string{"/my/b.txt": "different"}}

	outcome, err := Downloader(StreamOptions{SkipExisting: true})(t.Context(), conn, Job{
		Local:  local,
		Remote: "/my/b.txt",
		Size:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != Skipped {
		t.Errorf("outcome = %v, want Skipped", outcome)
	}

	got, err := os.ReadFile(local)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "12345" {
		t.Errorf("the file was rewritten to %q", got)
	}
}

func TestDownloaderReportsAMissingRemoteFile(t *testing.T) {
	conn := &opsConn{contents: map[string]string{}}

	_, err := Downloader(StreamOptions{})(t.Context(), conn, Job{
		Local:  filepath.Join(t.TempDir(), "b.txt"),
		Remote: "/my/nope.txt",
	})
	if err == nil {
		t.Fatal("a file that is not on the server should be an error")
	}
}

func TestScratchNameSitsNextToTheTarget(t *testing.T) {
	cases := map[string]string{
		"/my/a.txt":         "/my/.go-ftp-a.txt.part",
		"/my/sub/deep/c.md": "/my/sub/deep/.go-ftp-c.md.part",
		"/a.txt":            "/.go-ftp-a.txt.part",
	}

	for remote, want := range cases {
		if got := scratchName(remote); got != want {
			t.Errorf("scratchName(%q) = %q, want %q", remote, got, want)
		}
	}
}

func localFile(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestUploaderGoesThroughAScratchNameAndRenames(t *testing.T) {
	conn := &fakeConn{}

	outcome, err := Uploader(StreamOptions{})(t.Context(), conn, Job{
		Local:  localFile(t, "hello"),
		Remote: "/my/a.txt",
		Size:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome != Moved {
		t.Errorf("outcome = %v, want Moved", outcome)
	}

	if got := strings.Join(conn.renamed, ","); got != "/my/.go-ftp-a.txt.part -> /my/a.txt" {
		t.Errorf("renames = %q, want the scratch file moved into place", got)
	}
	if got := strings.Join(conn.uploaded, ","); got != "/my/a.txt" {
		t.Errorf("the file ended up at %q, want /my/a.txt", got)
	}
}

func TestUploaderLeavesTheTargetAloneWhenTheUploadFails(t *testing.T) {
	conn := &fakeConn{
		sizes:     map[string]int64{"/my/a.txt": 5},
		uploadErr: errors.New("connection reset"),
	}

	_, err := Uploader(StreamOptions{})(t.Context(), conn, Job{
		Local:  localFile(t, "newer"),
		Remote: "/my/a.txt",
		Size:   5,
	})
	if err == nil {
		t.Fatal("a broken upload should be an error")
	}

	if size, ok := conn.sizes["/my/a.txt"]; !ok || size != 5 {
		t.Errorf("the file on the server is now %d bytes, want the original 5", size)
	}
	if got := strings.Join(conn.removed, ","); got != "/my/.go-ftp-a.txt.part" {
		t.Errorf("removed %q, want the scratch file cleaned up", got)
	}
}

func TestUploaderInPlaceTruncatesTheTargetWhenItFails(t *testing.T) {
	conn := &fakeConn{
		sizes:     map[string]int64{"/my/a.txt": 5},
		uploadErr: errors.New("connection reset"),
	}

	_, err := Uploader(StreamOptions{InPlace: true})(t.Context(), conn, Job{
		Local:  localFile(t, "newer"),
		Remote: "/my/a.txt",
		Size:   5,
	})
	if err == nil {
		t.Fatal("a broken upload should be an error")
	}

	if size := conn.sizes["/my/a.txt"]; size != 0 {
		t.Errorf("the file is %d bytes, and --inplace is expected to have emptied it", size)
	}
}

func TestUploaderClearsTheWayWhenRenameIsRefused(t *testing.T) {
	conn := &fakeConn{
		sizes:     map[string]int64{"/my/a.txt": 5},
		renameErr: errors.New("550 file exists"),
	}

	conn.onRemove = func(remotePath string) {
		if remotePath == "/my/a.txt" {
			conn.renameErr = nil
		}
	}

	if _, err := Uploader(StreamOptions{})(t.Context(), conn, Job{
		Local:  localFile(t, "hello"),
		Remote: "/my/a.txt",
		Size:   5,
	}); err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(conn.removed, ","); got != "/my/a.txt" {
		t.Errorf("removed %q, want the target cleared out of the way", got)
	}
	if got := strings.Join(conn.renamed, ","); got != "/my/.go-ftp-a.txt.part -> /my/a.txt" {
		t.Errorf("renames = %q, want the second attempt to have worked", got)
	}
}

func TestUploaderGivesUpWhenRenameKeepsFailing(t *testing.T) {
	refused := errors.New("550 not allowed")

	conn := &fakeConn{
		sizes:     map[string]int64{"/my/a.txt": 5},
		renameErr: refused,
	}

	_, err := Uploader(StreamOptions{})(t.Context(), conn, Job{
		Local:  localFile(t, "hello"),
		Remote: "/my/a.txt",
		Size:   5,
	})
	if !errors.Is(err, refused) {
		t.Errorf("error = %v, want the refusal from the server", err)
	}
}

func counted() (ByteCounter, *atomic.Int64) {
	var total atomic.Int64
	return func(n int64) {
		total.Add(n)
	}, &total
}

func TestUploaderCountsBytesAsTheyTravel(t *testing.T) {
	const body = "the quick brown fox"

	count, total := counted()

	if _, err := Uploader(StreamOptions{InPlace: true, Count: count})(t.Context(), &fakeConn{}, Job{
		Local:  localFile(t, body),
		Remote: "/my/a.txt",
		Size:   int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}

	if got := total.Load(); got != int64(len(body)) {
		t.Errorf("counted %d bytes, want %d", got, len(body))
	}
}

func TestDownloaderCountsBytesAsTheyTravel(t *testing.T) {
	const body = "jumps over the lazy dog"

	count, total := counted()

	conn := &opsConn{contents: map[string]string{"/my/b.txt": body}}

	if _, err := Downloader(StreamOptions{Count: count})(t.Context(), conn, Job{
		Local:  filepath.Join(t.TempDir(), "b.txt"),
		Remote: "/my/b.txt",
		Size:   int64(len(body)),
	}); err != nil {
		t.Fatal(err)
	}

	if got := total.Load(); got != int64(len(body)) {
		t.Errorf("counted %d bytes, want %d", got, len(body))
	}
}

func TestCountingHappensDuringTheTransferNotAfterIt(t *testing.T) {
	const body = "half of this never arrives"

	count, total := counted()

	conn := &opsConn{
		contents: map[string]string{"/my/b.txt": body},
		readErr:  errors.New("connection reset"),
	}

	if _, err := Downloader(StreamOptions{Count: count})(t.Context(), conn, Job{
		Local:  filepath.Join(t.TempDir(), "b.txt"),
		Remote: "/my/b.txt",
	}); err == nil {
		t.Fatal("a broken transfer should be an error")
	}

	if got := total.Load(); got != int64(len(body)) {
		t.Errorf("counted %d bytes, want the %d that did arrive", got, len(body))
	}
}

func TestOperationsTakeANilCounter(t *testing.T) {
	if _, err := Uploader(StreamOptions{InPlace: true})(t.Context(), &fakeConn{}, Job{
		Local:  localFile(t, "x"),
		Remote: "/my/a.txt",
	}); err != nil {
		t.Fatal(err)
	}
}
