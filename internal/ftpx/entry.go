package ftpx

import (
	"time"

	"github.com/jlaffaye/ftp"
)

// Entry is one remote file or directory, carrying the full path rather than the
// bare name the server answers with.
type Entry struct {
	Path    string
	Name    string
	Size    int64
	Dir     bool
	Link    bool
	ModTime time.Time
}

func newEntry(fullPath string, from *ftp.Entry) Entry {
	return Entry{
		Path:    fullPath,
		Name:    from.Name,
		Size:    int64(from.Size),
		Dir:     from.Type == ftp.EntryTypeFolder,
		Link:    from.Type == ftp.EntryTypeLink,
		ModTime: from.Time,
	}
}
