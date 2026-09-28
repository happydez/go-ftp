package creds

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newStore points a store at a file inside a temporary directory that does not
// exist yet, which is the state before the first login.
func newStore(t *testing.T) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "go-ftp", "credentials"))
	if err != nil {
		t.Fatal(err)
	}

	return store
}

func TestOpenTreatsAMissingFileAsEmpty(t *testing.T) {
	store := newStore(t)

	if got := store.Profiles(); len(got) != 0 {
		t.Errorf("profiles = %v, want none", got)
	}
	if _, ok := store.Get("prod"); ok {
		t.Error("an empty store should hold nothing")
	}
}

func TestSetThenGetSurvivesReopening(t *testing.T) {
	store := newStore(t)

	if err := store.Set("prod", "s3cret"); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(store.Path())
	if err != nil {
		t.Fatal(err)
	}

	password, ok := reopened.Get("prod")
	if !ok {
		t.Fatal("the password was not saved")
	}
	if password != "s3cret" {
		t.Errorf("password = %q, want s3cret", password)
	}
}

func TestPasswordsAreTakenVerbatim(t *testing.T) {
	store := newStore(t)

	const awkward = " a=b # not a comment  "

	if err := store.Set("prod", awkward); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(store.Path())
	if err != nil {
		t.Fatal(err)
	}

	password, _ := reopened.Get("prod")
	if password != awkward {
		t.Errorf("password = %q, want %q", password, awkward)
	}
}

func TestSetRejectsLineBreaks(t *testing.T) {
	store := newStore(t)

	if err := store.Set("prod", "two\nlines"); err == nil {
		t.Error("a password with a line break would corrupt the file")
	}
	if err := store.Set("a=b", "x"); err == nil {
		t.Error("a profile name with an equals sign would corrupt the file")
	}
	if err := store.Set("prod", ""); err == nil {
		t.Error("an empty password should be refused")
	}
}

func TestDeleteReportsWhetherThereWasAnything(t *testing.T) {
	store := newStore(t)

	if err := store.Set("prod", "s3cret"); err != nil {
		t.Fatal(err)
	}

	removed, err := store.Delete("prod")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Error("the first delete should report a removal")
	}

	removed, err = store.Delete("prod")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Error("the second delete should report nothing to remove")
	}
}

func TestFileIsWrittenPrivately(t *testing.T) {
	store := newStore(t)

	if err := store.Set("prod", "s3cret"); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatal(err)
	}

	if perm := info.Mode().Perm(); perm&0o077 != 0 && perm != 0o666 {
		t.Errorf("mode = %o, want no access for other users", perm)
	}
	if store.TooOpen() {
		t.Error("a freshly written file should not be too open")
	}
}

func TestCommentsAndBlankLinesAreIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	body := "# a comment\n\n  # an indented comment\nprod=s3cret\r\nlocal=hunter2\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}

	if got := strings.Join(store.Profiles(), ","); got != "local,prod" {
		t.Errorf("profiles = %q, want local,prod", got)
	}
	if password, _ := store.Get("prod"); password != "s3cret" {
		t.Errorf("a CRLF line gave %q, want s3cret", password)
	}
}

func TestOpenRejectsAGarbledFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials")
	if err := os.WriteFile(path, []byte("prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(path); err == nil {
		t.Error("a line without an equals sign should be reported, not ignored")
	}
}
