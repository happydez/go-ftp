package config

import (
	"strings"
	"testing"
)

func TestParseSize(t *testing.T) {
	cases := map[string]int64{
		"":        0,
		"0":       0,
		"512":     512,
		"512B":    512,
		"1KB":     1024,
		"1kb":     1024,
		"1 KB":    1024,
		"1K":      1024,
		"1KiB":    1024,
		"500KB":   500 * 1024,
		"1MB":     1 << 20,
		"2mb":     2 << 20,
		"1.5MB":   1536 * 1024,
		"1GB":     1 << 30,
		"1TB":     1 << 40,
		"  2MB  ": 2 << 20,
	}

	for in, want := range cases {
		got, err := ParseSize(in)
		if err != nil {
			t.Errorf("ParseSize(%q): %v", in, err)

			continue
		}
		if got.Bytes() != want {
			t.Errorf("ParseSize(%q) = %d, want %d", in, got.Bytes(), want)
		}
	}
}

func TestParseSizeRejectsNonsense(t *testing.T) {
	for _, in := range []string{"-1", "-1MB", "MB", "1XB", "one", "1,5MB", "1 2 MB"} {
		if got, err := ParseSize(in); err == nil {
			t.Errorf("ParseSize(%q) = %d, want an error", in, got.Bytes())
		}
	}
}

func TestSizeReadsBackTheWayItWasWritten(t *testing.T) {
	for _, in := range []string{"0", "512B", "1KB", "500KB", "2MB", "1GB", "1TB"} {
		parsed, err := ParseSize(in)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", in, err)
		}

		if got := parsed.String(); got != in {
			t.Errorf("%q printed back as %q", in, got)
		}

		again, err := ParseSize(parsed.String())
		if err != nil || again != parsed {
			t.Errorf("%q did not survive a round trip: %v %v", in, again, err)
		}
	}
}

func TestLimitsInTheConfig(t *testing.T) {
	body := "profiles:\n  a:\n    host: h\n    user: u\ntransfer:\n  upload_limit: 2MB\n  download_limit: 500KB\n"

	cfg, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}

	if got := cfg.Transfer.UploadLimit.Bytes(); got != 2<<20 {
		t.Errorf("upload_limit = %d, want 2MB", got)
	}
	if got := cfg.Transfer.DownloadLimit.Bytes(); got != 500*1024 {
		t.Errorf("download_limit = %d, want 500KB", got)
	}
}

func TestLimitsAreOffByDefault(t *testing.T) {
	cfg, err := Load(write(t, "profiles:\n  a:\n    host: h\n    user: u\n"))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Transfer.UploadLimit != 0 || cfg.Transfer.DownloadLimit != 0 {
		t.Errorf("limits = %s and %s, want no limit at all",
			cfg.Transfer.UploadLimit, cfg.Transfer.DownloadLimit)
	}
}

func TestANegativeLimitIsRefused(t *testing.T) {
	body := "profiles:\n  a:\n    host: h\n    user: u\ntransfer:\n  upload_limit: -1MB\n"

	_, err := Load(write(t, body))
	if err == nil {
		t.Fatal("a negative limit should be refused")
	}
	if !strings.Contains(err.Error(), "cannot be negative") {
		t.Errorf("error %q does not say what is wrong", err)
	}
}
