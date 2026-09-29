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

// Plan is everything one run has to do.
type Plan struct {
	Jobs []Job
	// Dirs are directories that hold no files of their own and would therefore
	// never be created by a transfer. They are what keeps an empty directory in
	// the source from quietly disappearing.
	Dirs []string
	// Ignored lists source paths that were passed over, which is anything that
	// is not a plain file. Saying so is better than losing them in silence.
	Ignored []string
}

func (p Plan) Empty() bool {
	return len(p.Jobs) == 0 && len(p.Dirs) == 0
}

// TotalSize is what the whole plan adds up to.
func (p Plan) TotalSize() int64 {
	var total int64
	for _, job := range p.Jobs {
		total += job.Size
	}

	return total
}

// Lister is the part of an FTP client the planner needs.
type Lister interface {
	Stat(ctx context.Context, remotePath string) (ftpx.Entry, bool, error)
	Walk(ctx context.Context, dir string) ([]ftpx.Entry, error)
}

var _ Lister = (*ftpx.Client)(nil)

// PlanUpload lists what goes from local into remoteDir, which the caller has
// already resolved against base_dir.
func PlanUpload(remoteDir, local string, contentsOnly bool) (Plan, error) {
	info, err := os.Lstat(local)
	if err != nil {
		return Plan{}, fmt.Errorf("read %s: %w", local, err)
	}

	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return Plan{Ignored: []string{local}}, nil
		}

		return Plan{Jobs: []Job{{
			Local:  local,
			Remote: path.Join(remoteDir, filepath.Base(local)),
			Size:   info.Size(),
		}}}, nil
	}

	destination := remoteDir
	if !contentsOnly {
		if name := SourceName(local); name != "" {
			destination = path.Join(remoteDir, name)
		}
	}

	plan, err := walkLocal(local, destination)
	if err != nil {
		return Plan{}, fmt.Errorf("read %s: %w", local, err)
	}

	return plan, nil
}

// SourceName is the directory name a transfer carries over. It is empty when the
// path does not name one, which is the case for "." and for a filesystem root,
// and then only the contents can travel.
func SourceName(local string) string {
	base := filepath.Base(filepath.Clean(local))

	switch base {
	case ".", "..", "/", "\\":
		return ""
	}

	if len(base) == 2 && base[1] == ':' {
		return ""
	}

	return base
}

func walkLocal(local, remoteDir string) (Plan, error) {
	var (
		plan    Plan
		dirs    []string
		covered = make(coverage)
	)

	walkErr := filepath.WalkDir(local, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(local, current)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}

		rel := toSlash(relative)

		if entry.IsDir() {
			dirs = append(dirs, rel)

			return nil
		}

		// A symlink, a socket, a device. There is nothing sensible to send, and
		// pretending otherwise would upload the wrong bytes.
		if !entry.Type().IsRegular() {
			plan.Ignored = append(plan.Ignored, current)

			return nil
		}

		stat, err := entry.Info()
		if err != nil {
			return err
		}

		plan.Jobs = append(plan.Jobs, Job{
			Local:  current,
			Remote: path.Join(remoteDir, rel),
			Size:   stat.Size(),
		})
		covered.add(rel)

		return nil
	})
	if walkErr != nil {
		return Plan{}, walkErr
	}

	for _, rel := range dirs {
		if !covered.holds(rel) {
			plan.Dirs = append(plan.Dirs, path.Join(remoteDir, rel))
		}
	}

	plan.sort()

	return plan, nil
}

// PlanDownload lists what comes from remotePath into a local directory.
func PlanDownload(ctx context.Context, lister Lister, remotePath, local string, contentsOnly bool) (Plan, error) {
	entry, found, err := lister.Stat(ctx, remotePath)
	if err != nil {
		return Plan{}, err
	}
	if !found {
		return Plan{}, fmt.Errorf("%s is not on the server", remotePath)
	}

	if !entry.Dir {
		return Plan{Jobs: []Job{{
			Local:  filepath.Join(local, path.Base(remotePath)),
			Remote: remotePath,
			Size:   entry.Size,
		}}}, nil
	}

	if !contentsOnly {
		if name := SourceName(path.Clean(remotePath)); name != "" {
			local = filepath.Join(local, name)
		}
	}

	entries, err := lister.Walk(ctx, remotePath)
	if err != nil {
		return Plan{}, err
	}

	var (
		plan    Plan
		dirs    []string
		covered = make(coverage)
	)

	for _, item := range entries {
		rel := Relative(remotePath, item.Path)
		if rel == "" {
			continue
		}

		if item.Dir {
			dirs = append(dirs, rel)

			continue
		}

		plan.Jobs = append(plan.Jobs, Job{
			Local:  filepath.Join(local, filepath.FromSlash(rel)),
			Remote: item.Path,
			Size:   item.Size,
		})
		covered.add(rel)
	}

	for _, rel := range dirs {
		if !covered.holds(rel) {
			plan.Dirs = append(plan.Dirs, filepath.Join(local, filepath.FromSlash(rel)))
		}
	}

	plan.sort()

	return plan, nil
}

// coverage is the set of directories that moving the files creates on the way,
// held as paths relative to the root of the transfer.
type coverage map[string]struct{}

// add marks the directory a file lives in and every parent of it.
func (c coverage) add(fileRel string) {
	for dir := path.Dir(fileRel); dir != "." && dir != "/"; dir = path.Dir(dir) {
		if _, seen := c[dir]; seen {
			return
		}
		c[dir] = struct{}{}
	}
}

func (c coverage) holds(dirRel string) bool {
	_, ok := c[dirRel]
	return ok
}

func (p *Plan) sort() {
	sort.Slice(p.Jobs, func(i, j int) bool {
		return p.Jobs[i].Remote < p.Jobs[j].Remote
	})
	sort.Strings(p.Dirs)
	sort.Strings(p.Ignored)
}
