package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/happydez/go-ftp/internal/ftpx"
)

func listing() []ftpx.Entry {
	moment := time.Date(2026, 9, 29, 2, 14, 0, 0, time.UTC)

	return []ftpx.Entry{
		{Path: "/my/a.txt", Name: "a.txt", Size: 10, ModTime: moment},
		{Path: "/my/sub", Name: "sub", Dir: true, ModTime: moment},
		{Path: "/my/sub/b.txt", Name: "b.txt", Size: 2048, ModTime: moment},
	}
}

// render runs a format and hands back what it wrote.
func render(t *testing.T, entries []ftpx.Entry, output string) string {
	t.Helper()

	var out bytes.Buffer
	if err := writeEntries(&out, entries, "/my", output); err != nil {
		t.Fatal(err)
	}

	return out.String()
}

func TestTableShowsNamesSizesAndATotal(t *testing.T) {
	got := render(t, listing(), "table")

	for _, want := range []string{"NAME", "SIZE", "MODIFIED", "a.txt", "sub", "sub/b.txt", "2.0 KB"} {
		if !strings.Contains(got, want) {
			t.Errorf("the table is missing %q:\n%s", want, got)
		}
	}

	if !strings.Contains(got, "2 file(s), 2.0 KB") {
		t.Errorf("the total is wrong:\n%s", got)
	}
	if strings.Contains(got, "PATH") {
		t.Errorf("the plain table should not carry the wide columns:\n%s", got)
	}
}

func TestWideAddsTheTypeAndTheFullPath(t *testing.T) {
	got := render(t, listing(), "wide")
	for _, want := range []string{"TYPE", "PATH", "file", "dir", "/my/sub/b.txt"} {
		if !strings.Contains(got, want) {
			t.Errorf("the wide table is missing %q:\n%s", want, got)
		}
	}
}

func TestNameListsOnePathPerLine(t *testing.T) {
	got := render(t, listing(), "name")
	if got != "a.txt\nsub\nsub/b.txt\n" {
		t.Errorf("names = %q", got)
	}
}

func TestJSONCarriesEveryField(t *testing.T) {
	var decoded []ftpx.Entry
	if err := json.Unmarshal([]byte(render(t, listing(), "json")), &decoded); err != nil {
		t.Fatal(err)
	}

	if len(decoded) != 3 {
		t.Fatalf("decoded %d entries, want 3", len(decoded))
	}
	if decoded[2].Path != "/my/sub/b.txt" || decoded[2].Size != 2048 {
		t.Errorf("entry = %+v, want the full path and size", decoded[2])
	}
	if !decoded[1].Dir {
		t.Error("the directory lost its dir flag")
	}
	if decoded[0].ModTime.IsZero() {
		t.Error("the modification time did not survive the round trip")
	}
}

func TestEmptyListings(t *testing.T) {
	if got := render(t, nil, "json"); strings.TrimSpace(got) != "[]" {
		t.Errorf("json = %q, want []", got)
	}
	if got := render(t, nil, "name"); got != "" {
		t.Errorf("name = %q, want nothing", got)
	}
	if got := render(t, nil, "table"); !strings.Contains(got, "is empty") {
		t.Errorf("table = %q, want a word about the directory being empty", got)
	}
}

func TestUnknownOutputFormatIsAUsageMistake(t *testing.T) {
	var out bytes.Buffer
	err := writeEntries(&out, listing(), "/my", "xml")
	if err == nil {
		t.Fatal("an unknown format should be refused")
	}

	var ue usageError
	if !errors.As(err, &ue) {
		t.Errorf("error = %v, want a usage error so that the exit code is 2", err)
	}
}

func TestDisplayNameFallsBackToTheBareName(t *testing.T) {
	entry := ftpx.Entry{Path: "/my/a.txt", Name: "a.txt"}
	if got := displayName(entry, "/my/a.txt"); got != "a.txt" {
		t.Errorf("displayName = %q, want a.txt", got)
	}
	if got := displayName(entry, "/my"); got != "a.txt" {
		t.Errorf("displayName = %q, want a.txt", got)
	}
}

func TestSizeColumnAndKind(t *testing.T) {
	dir := ftpx.Entry{Dir: true}
	link := ftpx.Entry{Link: true}
	file := ftpx.Entry{Size: 1536}

	if got := sizeColumn(dir); got != "-" {
		t.Errorf("a directory shows %q, want -", got)
	}
	if got := sizeColumn(file); got != "1.5 KB" {
		t.Errorf("a file shows %q, want 1.5 KB", got)
	}

	for entry, want := range map[ftpx.Entry]string{dir: "dir", link: "link", file: "file"} {
		if got := kindOf(entry); got != want {
			t.Errorf("kindOf(%+v) = %q, want %q", entry, got, want)
		}
	}
}

func TestModifiedHandlesAServerThatGivesNoTime(t *testing.T) {
	if got := modified(ftpx.Entry{}); got != "-" {
		t.Errorf("modified = %q, want -", got)
	}
}
