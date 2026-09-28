package config

import (
	"os"
	"strings"
	"testing"
)

func TestSetCurrentProfileKeepsTheRestOfTheFile(t *testing.T) {
	body := "# a comment worth keeping\ncurrent_profile: local\n\nprofiles:\n  local:\n    host: h\n    user: u\n  prod:\n    host: h2\n    user: u2\n"
	path := write(t, body)

	if err := SetCurrentProfile(path, "prod"); err != nil {
		t.Fatal(err)
	}

	got := read(t, path)
	if !strings.Contains(got, "current_profile: prod") {
		t.Errorf("profile was not switched:\n%s", got)
	}
	if !strings.Contains(got, "# a comment worth keeping") {
		t.Errorf("the comment was lost:\n%s", got)
	}
	if strings.Contains(got, "current_profile: local") {
		t.Errorf("the old value is still there:\n%s", got)
	}
}

func TestSetCurrentProfileKeepsCRLF(t *testing.T) {
	path := write(t, "current_profile: local\r\nprofiles:\r\n  prod:\r\n    host: h\r\n")

	if err := SetCurrentProfile(path, "prod"); err != nil {
		t.Fatal(err)
	}

	got := read(t, path)
	if !strings.Contains(got, "current_profile: prod\r\n") {
		t.Errorf("the line break style was not kept: %q", got)
	}
	if strings.Contains(got, "\n\n") {
		t.Errorf("a stray line break appeared: %q", got)
	}
}

func TestSetCurrentProfileAddsTheKeyWhenAbsent(t *testing.T) {
	path := write(t, "profiles:\n  prod:\n    host: h\n    user: u\n")

	if err := SetCurrentProfile(path, "prod"); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("the file stopped parsing: %v", err)
	}
	if cfg.CurrentProfile != "prod" {
		t.Errorf("current_profile = %q, want prod", cfg.CurrentProfile)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return string(data)
}
