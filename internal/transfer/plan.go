package transfer

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"

	"github.com/happydez/go-ftp/internal/ftpx"
)

// Job is one file to move.
type Job struct {
	// Local is the path on this machine, written the way this machine writes
	// paths.
	Local string
	// Remote is the absolute path on the server, always separated by slashes.
	Remote string
	// Size is what the source says, used for the report and for skipping files
	// that are already there.
	Size int64
}

// Lister is the part of an FTP client the planner needs.
type Lister interface {
	Stat(ctx context.Context, remotePath string) (ftpx.Entry, bool, error)
	Walk(ctx context.Context, dir string) ([]ftpx.Entry, error)
}

var _ Lister = (*ftpx.Client)(nil)

// PlanUpload lists what goes from local to the server.
func PlanUpload(base, local, remote string) ([]Job, error) {
	remoteDir, err := Resolve(base, remote)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(local)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", local, err)
	}

	if !info.IsDir() {
		return []Job{{
			Local:  local,
			Remote: path.Join(remoteDir, filepath.Base(local)),
			Size:   info.Size(),
		}}, nil
	}

	var jobs []Job

	walkErr := filepath.WalkDir(local, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		rel, err := filepath.Rel(local, current)
		if err != nil {
			return err
		}

		stat, err := entry.Info()
		if err != nil {
			return err
		}

		jobs = append(jobs, Job{
			Local:  current,
			Remote: path.Join(remoteDir, toSlash(rel)),
			Size:   stat.Size(),
		})

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("read %s: %w", local, walkErr)
	}

	sortJobs(jobs)

	return jobs, nil
}

// PlanDownload lists what comes from the server into a local directory. It
// mirrors PlanUpload, so the remote side may be a file or a directory and the
// local side is always a directory.
func PlanDownload(ctx context.Context, lister Lister, base, remote, local string) ([]Job, error) {
	remotePath, err := Resolve(base, remote)
	if err != nil {
		return nil, err
	}

	entry, found, err := lister.Stat(ctx, remotePath)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%s is not on the server", remotePath)
	}

	if !entry.Dir {
		return []Job{{
			Local:  filepath.Join(local, path.Base(remotePath)),
			Remote: remotePath,
			Size:   entry.Size,
		}}, nil
	}

	entries, err := lister.Walk(ctx, remotePath)
	if err != nil {
		return nil, err
	}

	var jobs []Job

	for _, item := range entries {
		if item.Dir {
			continue
		}

		rel := Relative(remotePath, item.Path)
		if rel == "" {
			continue
		}

		jobs = append(jobs, Job{
			Local:  filepath.Join(local, filepath.FromSlash(rel)),
			Remote: item.Path,
			Size:   item.Size,
		})
	}

	sortJobs(jobs)

	return jobs, nil
}

// TotalSize is what the whole plan adds up to.
func TotalSize(jobs []Job) int64 {
	var total int64
	for _, job := range jobs {
		total += job.Size
	}

	return total
}

func sortJobs(jobs []Job) {
	sort.Slice(jobs, func(i, j int) bool {
		return jobs[i].Remote < jobs[j].Remote
	})
}
