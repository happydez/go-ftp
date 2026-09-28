// Package creds keeps the FTP passwords out of the config file.
package creds

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// EnvPath names the credentials file, and wins over the default location.
const EnvPath = "GOFTP_CREDS"

const header = "# go-ftp credentials. Keep this file to yourself.\n" +
	"# One line per profile, written by `go-ftp login`.\n"

// Store is the file that holds one password per profile.
type Store struct {
	path      string
	passwords map[string]string
}

// DefaultPath is the credentials file next to the user config file, which is
// ~/.config/go-ftp/credentials on Linux and %AppData%\go-ftp\credentials on
// Windows.
func DefaultPath() (string, error) {
	if fromEnv := os.Getenv(EnvPath); fromEnv != "" {
		return fromEnv, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate the user config directory: %w", err)
	}

	return filepath.Join(dir, "go-ftp", "credentials"), nil
}

// Open reads the store. A file that is not there yet is an empty store rather
// than an error, since that is the state before the first login.
func Open(path string) (*Store, error) {
	if path == "" {
		fromDefault, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = fromDefault
	}

	store := &Store{
		path:      path,
		passwords: make(map[string]string),
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return store, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}

	if err := store.parse(data); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return store, nil
}

func (s *Store) Path() string {
	return s.path
}

// Get returns the stored password for a profile.
func (s *Store) Get(profile string) (string, bool) {
	password, ok := s.passwords[profile]
	return password, ok
}

// Set writes the password for a profile and saves the file.
func (s *Store) Set(profile, password string) error {
	if err := checkLine(profile, "profile name"); err != nil {
		return err
	}
	if err := checkLine(password, "password"); err != nil {
		return err
	}
	if strings.Contains(profile, "=") {
		return fmt.Errorf("a profile name cannot contain %q", "=")
	}

	s.passwords[profile] = password

	return s.save()
}

// Delete drops the password for a profile and reports whether there was one.
func (s *Store) Delete(profile string) (bool, error) {
	if _, ok := s.passwords[profile]; !ok {
		return false, nil
	}

	delete(s.passwords, profile)

	return true, s.save()
}

// Profiles lists the profiles that have a password stored.
func (s *Store) Profiles() []string {
	names := make([]string, 0, len(s.passwords))
	for name := range s.passwords {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

func (s *Store) parse(data []byte) error {
	scanner := bufio.NewScanner(bytes.NewReader(data))

	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimRight(scanner.Text(), "\r")
		if strings.TrimSpace(text) == "" || strings.HasPrefix(strings.TrimSpace(text), "#") {
			continue
		}

		key, value, found := strings.Cut(text, "=")
		if !found {
			return fmt.Errorf("line %d is not profile=password", line)
		}

		s.passwords[strings.TrimSpace(key)] = value
	}

	return scanner.Err()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("create the credentials directory: %w", err)
	}

	var body strings.Builder
	body.WriteString(header)
	for _, profile := range s.Profiles() {
		body.WriteString(profile)
		body.WriteByte('=')
		body.WriteString(s.passwords[profile])
		body.WriteByte('\n')
	}

	if err := os.WriteFile(s.path, []byte(body.String()), 0o600); err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}

	return nil
}

// TooOpen reports whether other users can read the file.
func (s *Store) TooOpen() bool {
	if runtime.GOOS == "windows" {
		return false
	}

	info, err := os.Stat(s.path)
	if err != nil {
		return false
	}

	return info.Mode().Perm()&0o077 != 0
}

func checkLine(value, what string) error {
	if value == "" {
		return fmt.Errorf("the %s cannot be empty", what)
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("the %s cannot contain a line break", what)
	}

	return nil
}
