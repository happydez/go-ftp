package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindPrefersTheEnvironment(t *testing.T) {
	dir := t.TempDir()

	inCwd := filepath.Join(dir, "go-ftp.yml")
	elsewhere := filepath.Join(dir, "elsewhere.yml")
	for _, path := range []string{inCwd, elsewhere} {
		if err := os.WriteFile(path, []byte("profiles: {}\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Chdir(dir)
	t.Setenv(EnvConfig, elsewhere)

	found, err := Find()
	if err != nil {
		t.Fatal(err)
	}
	if found != elsewhere {
		t.Errorf("found %q, want the file from %s", found, EnvConfig)
	}
}

func TestFindFallsBackToTheWorkingDirectory(t *testing.T) {
	dir := t.TempDir()

	want := filepath.Join(dir, "go-ftp.yaml")
	if err := os.WriteFile(want, []byte("profiles: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Chdir(dir)
	t.Setenv(EnvConfig, "")

	found, err := Find()
	if err != nil {
		t.Fatal(err)
	}
	if found != want {
		t.Errorf("found %q, want %q", found, want)
	}
}

func TestFindListsWhereItLooked(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvConfig, "")

	_, err := Find()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "looked in:") {
		t.Errorf("error %q should list the places searched", err)
	}
}

func TestSearchPathHasNoDuplicates(t *testing.T) {
	seen := make(map[string]struct{})

	for _, place := range SearchPath() {
		if _, ok := seen[place]; ok {
			t.Errorf("%q is listed twice", place)
		}
		seen[place] = struct{}{}
	}
}
