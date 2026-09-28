package creds

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// EnvPassword and EnvUser let a CI job pass credentials without a file.
const (
	EnvPassword = "GOFTP_PASSWORD"
	EnvUser     = "GOFTP_USER"
)

// Source says where a password came from, which is worth reporting when several
// places could have supplied it.
type Source int

const (
	SourceNone Source = iota
	SourceStdin
	SourceEnv
	SourceStore
	SourcePrompt
)

func (s Source) String() string {
	switch s {
	case SourceStdin:
		return "--password-stdin"
	case SourceEnv:
		return EnvPassword
	case SourceStore:
		return "the credentials file"
	case SourcePrompt:
		return "the prompt"
	default:
		return "nowhere"
	}
}

// ErrNoPassword means none of the places held one.
var ErrNoPassword = errors.New("no password found")

// Prompter asks the user for a password. It is a field rather than a direct call
// so that the tests do not need a terminal.
type Prompter func(prompt string) (string, error)

// Options describes where Resolve is allowed to look.
type Options struct {
	Profile  string
	Store    *Store
	Stdin    io.Reader
	UseStdin bool
	Prompt   Prompter
}

// Resolve finds the password for a profile and says where it came from.
func Resolve(opts Options) (string, Source, error) {
	if opts.UseStdin {
		password, err := readAll(opts.Stdin)
		if err != nil {
			return "", SourceNone, err
		}

		return password, SourceStdin, nil
	}

	if password, ok := os.LookupEnv(EnvPassword); ok && password != "" {
		return password, SourceEnv, nil
	}

	if opts.Store != nil {
		if password, ok := opts.Store.Get(opts.Profile); ok {
			return password, SourceStore, nil
		}
	}

	if opts.Prompt != nil {
		password, err := opts.Prompt(fmt.Sprintf("Password for profile %s: ", opts.Profile))
		if err != nil {
			return "", SourceNone, err
		}
		if password == "" {
			return "", SourceNone, errors.New("the password cannot be empty")
		}

		return password, SourcePrompt, nil
	}

	return "", SourceNone, fmt.Errorf("%w for profile %q, run `go-ftp login` or set %s",
		ErrNoPassword, opts.Profile, EnvPassword)
}

// User is the account to log in as, which the environment may override so that a
// shared config can be reused.
func User(fromProfile string) string {
	if fromEnv := os.Getenv(EnvUser); fromEnv != "" {
		return fromEnv
	}

	return fromProfile
}

// readAll takes everything on the pipe and drops one trailing line break, which
// is what `echo secret | go-ftp login --password-stdin` leaves behind.
func readAll(r io.Reader) (string, error) {
	if r == nil {
		return "", errors.New("nothing to read the password from")
	}

	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("read the password: %w", err)
	}

	password := strings.TrimRight(string(data), "\r\n")
	if password == "" {
		return "", errors.New("the password read was empty")
	}

	return password, nil
}
