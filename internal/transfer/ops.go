package transfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// StreamOptions is everything that watches or holds back the bytes of one file.
// The zero value adds nothing to a plain transfer.
type StreamOptions struct {
	// SkipExisting leaves a file alone when its size already matches.
	SkipExisting bool
	// InPlace writes straight to the target name instead of uploading under a
	// scratch name and renaming. Uploads only.
	InPlace bool
	// Count is told about bytes as they travel.
	Count ByteCounter
	// Throttle holds the whole run to a speed. Shared by every worker.
	Throttle *Throttle
}

// watched wraps a stream with everything the options ask for.
func (o StreamOptions) watched(ctx context.Context, r io.Reader) io.Reader {
	return countingReader{r: o.Throttle.reader(ctx, r), count: o.Count}
}

// Uploader sends a local file to the server, creating the directory it belongs
// in on the way.
func Uploader(opts StreamOptions) Operation {
	return func(ctx context.Context, conn Conn, job Job) (Outcome, error) {
		if err := conn.EnsureDir(ctx, path.Dir(job.Remote)); err != nil {
			return Moved, err
		}

		if opts.SkipExisting {
			if size, ok := conn.Size(ctx, job.Remote); ok && size == job.Size {
				return Skipped, nil
			}
		}

		file, err := os.Open(job.Local)
		if err != nil {
			return Moved, err
		}
		defer func() {
			_ = file.Close()
		}()

		source := opts.watched(ctx, file)

		if opts.InPlace {
			if err := conn.Upload(ctx, job.Remote, source); err != nil {
				return Moved, err
			}

			return Moved, nil
		}

		scratch := scratchName(job.Remote)

		if err := conn.Upload(ctx, scratch, source); err != nil {
			_ = conn.Remove(ctx, scratch)
			return Moved, err
		}

		if err := putInPlace(ctx, conn, scratch, job.Remote); err != nil {
			_ = conn.Remove(ctx, scratch)
			return Moved, err
		}

		return Moved, nil
	}
}

// scratchName sits next to the target, which keeps the rename inside one
// directory and off any other filesystem the server may have mounted.
func scratchName(remote string) string {
	return path.Join(path.Dir(remote), ".go-ftp-"+path.Base(remote)+".part")
}

// putInPlace renames the finished upload over the target.
func putInPlace(ctx context.Context, conn Conn, from, to string) error {
	err := conn.Rename(ctx, from, to)
	if err == nil {
		return nil
	}

	if _, exists := conn.Size(ctx, to); !exists {
		return err
	}
	if removeErr := conn.Remove(ctx, to); removeErr != nil {
		return err
	}

	return conn.Rename(ctx, from, to)
}

// Downloader brings a remote file here. It writes to a temporary name and
// renames at the end, so that a run that is interrupted leaves no half file
// looking like a finished one.
func Downloader(opts StreamOptions) Operation {
	return func(ctx context.Context, conn Conn, job Job) (Outcome, error) {
		if opts.SkipExisting {
			if info, err := os.Stat(job.Local); err == nil && info.Size() == job.Size {
				return Skipped, nil
			}
		}

		dir := filepath.Dir(job.Local)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return Moved, fmt.Errorf("create %s: %w", dir, err)
		}

		reader, err := conn.Download(ctx, job.Remote)
		if err != nil {
			return Moved, err
		}
		defer func() {
			_ = reader.Close()
		}()

		if err := writeFile(job.Local, opts.watched(ctx, reader)); err != nil {
			return Moved, err
		}

		return Moved, nil
	}
}

// writeFile puts the contents of r at target through a temporary file in the
// same directory, which keeps the rename on one filesystem.
func writeFile(target string, r io.Reader) error {
	temp, err := os.CreateTemp(filepath.Dir(target), ".go-ftp-*")
	if err != nil {
		return fmt.Errorf("create a temporary file next to %s: %w", target, err)
	}

	name := temp.Name()

	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(name)
	}

	if _, err := io.Copy(temp, r); err != nil {
		cleanup()

		return fmt.Errorf("write %s: %w", target, err)
	}

	if err := temp.Close(); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("close %s: %w", name, err)
	}

	if err := os.Rename(name, target); err != nil {
		_ = os.Remove(name)

		return fmt.Errorf("move %s into place: %w", target, err)
	}

	return nil
}

// ByteCounter is told how many bytes have just travelled. Every worker calls it
// at once, so whatever is behind it has to be safe for that.
type ByteCounter func(n int64)

// countingReader reports what passes through it. Counting on the way past is
// what lets a single large file show progress, since nothing else happens
// between the start of a transfer and the end of it.
type countingReader struct {
	r     io.Reader
	count ByteCounter
}

func (c countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.count != nil {
		c.count(int64(n))
	}

	return n, err
}
