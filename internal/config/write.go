package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
)

var currentProfileKey = regexp.MustCompile(`(?m)^current_profile:[^\r\n]*`)

// SetCurrentProfile rewrites only the current_profile line of the file. The rest
// of it, comments included, is left byte for byte as it was.
func SetCurrentProfile(configPath, name string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}

	info, err := os.Stat(configPath)
	if err != nil {
		return fmt.Errorf("stat config: %w", err)
	}

	line := []byte("current_profile: " + name)

	var updated []byte
	if currentProfileKey.Match(data) {
		updated = currentProfileKey.ReplaceAllLiteral(data, line)
	} else {
		updated = append(line, append(lineBreak(data), data...)...)
	}

	if err := os.WriteFile(configPath, updated, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	return nil
}

// lineBreak guesses what the file already uses, so an added line does not mix
// styles inside one file.
func lineBreak(data []byte) []byte {
	if bytes.Contains(data, []byte("\r\n")) {
		return []byte("\r\n")
	}

	return []byte("\n")
}
