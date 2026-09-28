package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/happydez/go-ftp/internal/version"
)

func TestVersionFormats(t *testing.T) {
	info := version.Get()

	var out bytes.Buffer
	if err := printVersion(&out, "text"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), info.Version) {
		t.Errorf("the text form does not carry the version:\n%s", out.String())
	}

	out.Reset()
	if err := printVersion(&out, "short"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != info.Version {
		t.Errorf("short = %q, want just %q", got, info.Version)
	}

	out.Reset()
	if err := printVersion(&out, "json"); err != nil {
		t.Fatal(err)
	}

	var decoded version.Info
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded != info {
		t.Errorf("json = %+v, want %+v", decoded, info)
	}
}

func TestVersionRefusesAnUnknownFormat(t *testing.T) {
	err := printVersion(&bytes.Buffer{}, "xml")
	if err == nil {
		t.Fatal("an unknown format should be refused")
	}

	var ue usageError
	if !errors.As(err, &ue) {
		t.Errorf("error = %v, want a usage error", err)
	}
}
