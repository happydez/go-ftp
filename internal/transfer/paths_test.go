package transfer

import (
	"strings"
	"testing"
)

func TestResolveTakesEverythingRelativeToBase(t *testing.T) {
	cases := []struct {
		base  string
		input string
		want  string
	}{
		{"/my", "/", "/my"},
		{"/my", "", "/my"},
		{"/my", ".", "/my"},
		{"/my", "./", "/my"},
		{"/my", "sub", "/my/sub"},
		{"/my", "/sub", "/my/sub"},
		{"/my", "./sub/file.txt", "/my/sub/file.txt"},
		{"/my", `.\sub\file.txt`, "/my/sub/file.txt"},
		{"/my", "sub//nested///", "/my/sub/nested"},
		{"/my", "sub/./nested", "/my/sub/nested"},
		{"/my", "sub/deep/../other", "/my/sub/other"},
		{"/", "sub", "/sub"},
		{"/", "/", "/"},
		{"/a/b", "c", "/a/b/c"},
	}

	for _, tc := range cases {
		got, err := Resolve(tc.base, tc.input)
		if err != nil {
			t.Errorf("Resolve(%q, %q): %v", tc.base, tc.input, err)

			continue
		}
		if got != tc.want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", tc.base, tc.input, got, tc.want)
		}
	}
}

func TestResolveRefusesToLeaveTheBase(t *testing.T) {
	cases := []struct {
		base  string
		input string
	}{
		{"/my", ".."},
		{"/my", "../"},
		{"/my", "../etc"},
		{"/my", "../../etc/passwd"},
		{"/my", "/../etc"},
		{"/my", "sub/../../etc"},
		{"/my", `..\etc`},
		{"/", "../etc"},
	}

	for _, tc := range cases {
		got, err := Resolve(tc.base, tc.input)
		if err == nil {
			t.Errorf("Resolve(%q, %q) = %q, want an error", tc.base, tc.input, got)

			continue
		}
		if !strings.Contains(err.Error(), "outside the base directory") {
			t.Errorf("Resolve(%q, %q) gave %q, which does not say why", tc.base, tc.input, err)
		}
	}
}

func TestRelative(t *testing.T) {
	cases := []struct {
		base string
		full string
		want string
	}{
		{"/my", "/my", ""},
		{"/my", "/my/a.txt", "a.txt"},
		{"/my", "/my/sub/a.txt", "sub/a.txt"},
		{"/", "/a.txt", "a.txt"},
		{"/", "/sub/a.txt", "sub/a.txt"},
	}

	for _, tc := range cases {
		if got := Relative(tc.base, tc.full); got != tc.want {
			t.Errorf("Relative(%q, %q) = %q, want %q", tc.base, tc.full, got, tc.want)
		}
	}
}
