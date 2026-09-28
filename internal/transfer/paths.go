// Package transfer works out what has to move and then moves it.
package transfer

import (
	"fmt"
	"path"
	"strings"
)

// Resolve turns a path the user typed into an absolute path on the server.
// Everything is taken relative to base, so a leading slash means the base
// directory rather than the root of the server.
func Resolve(base, input string) (string, error) {
	rel := strings.TrimPrefix(toSlash(input), "/")
	if rel == "" {
		rel = "."
	}

	cleaned := path.Clean(rel)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%s is outside the base directory %s", input, base)
	}

	return path.Join(base, cleaned), nil
}

// Relative is the part of full that sits under base, which is what a local tree
// mirrors.
func Relative(base, full string) string {
	if full == base {
		return ""
	}

	return strings.TrimPrefix(full, strings.TrimSuffix(base, "/")+"/")
}

// toSlash accepts the separator the shell on this machine uses, so that a
// Windows user typing .\my\file gets the same answer as everyone else.
func toSlash(p string) string {
	return strings.ReplaceAll(p, `\`, "/")
}
