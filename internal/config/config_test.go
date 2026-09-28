package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const twoProfiles = `
current_profile: prod

profiles:
  prod:
    host: ftp.example.com
    user: deploy
    base_dir: /upload/
    tls: true
  local:
    host: 127.0.0.1
    user: user
`

// write drops a config into a temporary directory and returns its path.
func write(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "go-ftp.yml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

func TestLoadFillsInDefaults(t *testing.T) {
	cfg, err := Load(write(t, twoProfiles))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Transfer.Workers != defaultWorkers {
		t.Errorf("workers = %d, want %d", cfg.Transfer.Workers, defaultWorkers)
	}
	if cfg.Transfer.MaxRetries != defaultMaxRetries {
		t.Errorf("max_retries = %d, want %d", cfg.Transfer.MaxRetries, defaultMaxRetries)
	}
	if cfg.Transfer.Timeout.Unwrap() != defaultTimeout {
		t.Errorf("timeout = %s, want %s", cfg.Transfer.Timeout, defaultTimeout)
	}

	local := cfg.Profiles["local"]
	if local.Port != defaultPort {
		t.Errorf("local port = %d, want %d", local.Port, defaultPort)
	}
	if local.BaseDir != "/" {
		t.Errorf("local base_dir = %q, want %q", local.BaseDir, "/")
	}
}

func TestProfilePicksCurrentThenFlag(t *testing.T) {
	cfg, err := Load(write(t, twoProfiles))
	if err != nil {
		t.Fatal(err)
	}

	current, err := cfg.Profile("")
	if err != nil {
		t.Fatal(err)
	}
	if current.Name() != "prod" {
		t.Errorf("default profile = %q, want prod", current.Name())
	}
	if current.Addr() != "ftp.example.com:21" {
		t.Errorf("addr = %q, want ftp.example.com:21", current.Addr())
	}
	if current.BaseDir != "/upload" {
		t.Errorf("base_dir = %q, want /upload", current.BaseDir)
	}

	asked, err := cfg.Profile("local")
	if err != nil {
		t.Fatal(err)
	}
	if asked.Name() != "local" {
		t.Errorf("asked profile = %q, want local", asked.Name())
	}

	if _, err := cfg.Profile("nope"); err == nil {
		t.Error("an unknown profile should be an error")
	}
}

func TestLoneProfileNeedsNoCurrent(t *testing.T) {
	cfg, err := Load(write(t, "profiles:\n  only:\n    host: h\n    user: u\n"))
	if err != nil {
		t.Fatal(err)
	}

	profile, err := cfg.Profile("")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name() != "only" {
		t.Errorf("profile = %q, want only", profile.Name())
	}
}

func TestLoadRejectsBadFiles(t *testing.T) {
	cases := map[string]struct {
		body string
		want string
	}{
		"misspelled key": {
			body: "profiles:\n  a:\n    host: h\n    usr: u\n",
			want: "field usr not found",
		},
		"no profiles": {
			body: "transfer:\n  workers: 2\n",
			want: "no profiles are defined",
		},
		"current_profile missing from profiles": {
			body: "current_profile: gone\nprofiles:\n  a:\n    host: h\n    user: u\n",
			want: `current_profile "gone" is not among the profiles`,
		},
		"several profiles and no current": {
			body: "profiles:\n  a:\n    host: h\n    user: u\n  b:\n    host: h\n    user: u\n",
			want: "current_profile is required",
		},
		"negative workers": {
			body: "profiles:\n  a:\n    host: h\n    user: u\ntransfer:\n  workers: -1\n",
			want: "transfer.workers must be at least 1",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(write(t, tc.body))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestProfileValidationReportsEveryProblemAtOnce(t *testing.T) {
	cfg, err := Load(write(t, "profiles:\n  a:\n    base_dir: /x\n    tls_insecure: true\n"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = cfg.Profile("a")
	if err == nil {
		t.Fatal("expected an error")
	}

	for _, want := range []string{"host is required", "user is required", "tls_insecure has no effect"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestNormalizeDir(t *testing.T) {
	cases := map[string]string{
		"":            "/",
		"/":           "/",
		"my":          "/my",
		"/my/":        "/my",
		"/my/sub/":    "/my/sub",
		"/my/./sub":   "/my/sub",
		`\my\sub`:     "/my/sub",
		"/my//sub///": "/my/sub",
	}

	for in, want := range cases {
		if got := normalizeDir(in); got != want {
			t.Errorf("normalizeDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAddrBracketsIPv6(t *testing.T) {
	p := Profile{Host: "::1", Port: 2121}
	if got := p.Addr(); got != "[::1]:2121" {
		t.Errorf("addr = %q, want [::1]:2121", got)
	}
}

func TestExplicitZeroBeatsTheDefault(t *testing.T) {
	body := "profiles:\n  a:\n    host: h\n    user: u\ntransfer:\n  max_retries: 0\n"

	cfg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Transfer.MaxRetries != 0 {
		t.Errorf("max_retries = %d, want 0 for no retries", cfg.Transfer.MaxRetries)
	}
}

func TestTimeoutIsTheWholeRunAndOffByDefault(t *testing.T) {
	cfg, err := Load(write(t, "profiles:\n  a:\n    host: h\n    user: u\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transfer.Timeout != 0 {
		t.Errorf("timeout = %s, want no limit by default", cfg.Transfer.Timeout)
	}

	cfg, err = Load(write(t, "profiles:\n  a:\n    host: h\n    user: u\ntransfer:\n  timeout: 90s\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Transfer.Timeout.Unwrap() != 90*time.Second {
		t.Errorf("timeout = %s, want 90s", cfg.Transfer.Timeout)
	}
}
