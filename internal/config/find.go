package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvConfig names the file to use, and wins over every searched location.
const EnvConfig = "GOFTP_CONFIG"

// FileName is what the search looks for next to the binary and in the working
// directory.
const FileName = "go-ftp"

// ErrNotFound means the search came up empty.
var ErrNotFound = errors.New("no config file found")

// Find returns the first config file that exists, in this order: $GOFTP_CONFIG,
// the working directory, the directory the binary sits in, and the user config
// directory.
func Find() (string, error) {
	places := SearchPath()

	for _, place := range places {
		if isFile(place) {
			return place, nil
		}
	}

	return "", fmt.Errorf("%w, looked in:\n  %s", ErrNotFound, strings.Join(places, "\n  "))
}

// SearchPath lists every place Find looks at, in order. Duplicates are dropped,
// which is what happens when the binary sits in the working directory.
func SearchPath() []string {
	var places []string

	if fromEnv := os.Getenv(EnvConfig); fromEnv != "" {
		places = append(places, fromEnv)
	}
	if cwd, err := os.Getwd(); err == nil {
		places = append(places, named(cwd, FileName)...)
	}
	if exe, err := os.Executable(); err == nil {
		places = append(places, named(filepath.Dir(exe), FileName)...)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		places = append(places, named(filepath.Join(dir, "go-ftp"), "config")...)
	}

	return unique(places)
}

// LoadFound loads the given file, or the first one the search finds when the
// path is empty.
func LoadFound(configPath string) (*Config, error) {
	if configPath == "" {
		found, err := Find()
		if err != nil {
			return nil, err
		}
		configPath = found
	}

	return Load(configPath)
}

func named(dir, base string) []string {
	return []string{
		filepath.Join(dir, base+".yml"),
		filepath.Join(dir, base+".yaml"),
	}
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func unique(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))

	for _, p := range paths {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}

	return out
}
