package transfer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// Uploader sends a local file to the server, creating the directory it belongs
// in on the way.
func Uploader(skipExisting bool) Operation {
	return func(ctx context.Context, conn Conn, job Job) (Outcome, error) {
		if err := conn.EnsureDir(ctx, path.Dir(job.Remote)); err != nil {
			return Moved, err
		}

		if skipExisting {
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

		if err := conn.Upload(ctx, job.Remote, file); err != nil {
			return Moved, err
		}

		return Moved, nil
	}
}

// Downloader brings a remote file here. It writes to a temporary name and
// renames at the end, so that a run that is interrupted leaves no half file
// looking like a finished one.
func Downloader(skipExisting bool) Operation {
	return func(ctx context.Context, conn Conn, job Job) (Outcome, error) {
		if skipExisting {
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

		if err := writeFile(job.Local, reader); err != nil {
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
