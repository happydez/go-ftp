package cli

import (
	"fmt"
	"os"
	"path"
	"strings"
)

// A Unix-like shell on Windows rewrites an argument that begins with a slash
// into a Windows path before the program is started. Git Bash turns --ftp /files
// into C:/Program Files/Git/files and since that is a perfectly legal directory
// name on an FTP server, nothing downstream can tell it was an accident.
//
// The shell says where its own root is, so the damage can be undone exactly
// rather than guessed at. A path that looks like a Windows one but does not come
// from there is refused instead, because there is no way to know what was meant.

// mangled records a path the shell rewrote, so the caller can say so out loud.
type mangled struct {
	given     string
	recovered string
}

func (m mangled) String() string {
	return fmt.Sprintf("the shell rewrote %s into %s before go-ftp saw it", m.recovered, m.given)
}

// unmangleRemote returns the remote path that was typed. The second result is
// set when something had to be undone.
func unmangleRemote(given string) (string, *mangled, error) {
	if !looksLikeAWindowsPath(given) {
		return given, nil, nil
	}

	root := shellRoot()
	if root != "" {
		if rest, ok := cutPrefixFold(toSlash(given), root); ok {
			recovered := "/" + strings.TrimPrefix(rest, "/")

			return recovered, &mangled{given: given, recovered: recovered}, nil
		}
	}

	return "", nil, fmt.Errorf("%s names a drive on this machine, not a path on the server; if a Unix-like shell rewrote it, set MSYS_NO_PATHCONV=1 and run it again", given)
}

// looksLikeAWindowsPath reports whether the argument starts with a drive letter.
// No FTP server path does.
func looksLikeAWindowsPath(p string) bool {
	if len(p) < 3 || p[1] != ':' {
		return false
	}
	if p[2] != '/' && p[2] != '\\' {
		return false
	}

	letter := p[0] | 0x20

	return letter >= 'a' && letter <= 'z'
}

// shellRoot is where the Unix-like shell thinks its filesystem starts, taken
// from what the shell itself puts in the environment. An empty answer means no
// such shell is around.
func shellRoot() string {
	if prefix := os.Getenv("MSYSTEM_PREFIX"); prefix != "" {
		return path.Dir(toSlash(prefix))
	}
	if exe := os.Getenv("EXEPATH"); exe != "" {
		return path.Dir(toSlash(exe))
	}

	return ""
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}

	return s[len(prefix):], true
}

func toSlash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}
