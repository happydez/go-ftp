package creds

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func storeWith(t *testing.T, profile, password string) *Store {
	t.Helper()

	store, err := Open(filepath.Join(t.TempDir(), "credentials"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set(profile, password); err != nil {
		t.Fatal(err)
	}

	return store
}

func neverAsked(t *testing.T) Prompter {
	t.Helper()

	return func(string) (string, error) {
		t.Error("the prompt should not have been reached")

		return "", nil
	}
}

func TestStdinWinsOverEverything(t *testing.T) {
	t.Setenv(EnvPassword, "from-env")

	password, source, err := Resolve(Options{
		Profile:  "prod",
		Store:    storeWith(t, "prod", "from-store"),
		Stdin:    strings.NewReader("from-stdin\n"),
		UseStdin: true,
		Prompt:   neverAsked(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if password != "from-stdin" {
		t.Errorf("password = %q, want from-stdin", password)
	}
	if source != SourceStdin {
		t.Errorf("source = %s, want --password-stdin", source)
	}
}

func TestEnvironmentWinsOverTheStore(t *testing.T) {
	t.Setenv(EnvPassword, "from-env")

	password, source, err := Resolve(Options{
		Profile: "prod",
		Store:   storeWith(t, "prod", "from-store"),
		Prompt:  neverAsked(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if password != "from-env" {
		t.Errorf("password = %q, want from-env", password)
	}
	if source != SourceEnv {
		t.Errorf("source = %s, want the environment", source)
	}
}

func TestStoreWinsOverThePrompt(t *testing.T) {
	t.Setenv(EnvPassword, "")

	password, source, err := Resolve(Options{
		Profile: "prod",
		Store:   storeWith(t, "prod", "from-store"),
		Prompt:  neverAsked(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	if password != "from-store" {
		t.Errorf("password = %q, want from-store", password)
	}
	if source != SourceStore {
		t.Errorf("source = %s, want the credentials file", source)
	}
}

func TestPromptIsTheLastResort(t *testing.T) {
	t.Setenv(EnvPassword, "")

	var asked string
	password, source, err := Resolve(Options{
		Profile: "prod",
		Store:   storeWith(t, "other", "not-this-one"),
		Prompt: func(prompt string) (string, error) {
			asked = prompt

			return "typed", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if password != "typed" {
		t.Errorf("password = %q, want typed", password)
	}
	if source != SourcePrompt {
		t.Errorf("source = %s, want the prompt", source)
	}
	if !strings.Contains(asked, "prod") {
		t.Errorf("the prompt %q should name the profile", asked)
	}
}

func TestNoPasswordAnywhere(t *testing.T) {
	t.Setenv(EnvPassword, "")

	_, _, err := Resolve(Options{Profile: "prod"})
	if !errors.Is(err, ErrNoPassword) {
		t.Errorf("error = %v, want ErrNoPassword", err)
	}
}

func TestStdinDropsOnlyTheTrailingBreak(t *testing.T) {
	cases := map[string]string{
		"s3cret\n":     "s3cret",
		"s3cret\r\n":   "s3cret",
		"s3cret":       "s3cret",
		" spaced  \n":  " spaced  ",
		"two\nlines\n": "two\nlines",
	}

	for in, want := range cases {
		password, _, err := Resolve(Options{
			Profile:  "prod",
			Stdin:    strings.NewReader(in),
			UseStdin: true,
		})
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if password != want {
			t.Errorf("%q gave %q, want %q", in, password, want)
		}
	}
}

func TestEmptyStdinIsAnError(t *testing.T) {
	_, _, err := Resolve(Options{
		Profile:  "prod",
		Stdin:    strings.NewReader("\n"),
		UseStdin: true,
	})
	if err == nil {
		t.Error("an empty password should be refused")
	}
}

func TestUserCanBeOverriddenByTheEnvironment(t *testing.T) {
	t.Setenv(EnvUser, "")
	if got := User("deploy"); got != "deploy" {
		t.Errorf("user = %q, want the one from the profile", got)
	}

	t.Setenv(EnvUser, "ci")
	if got := User("deploy"); got != "ci" {
		t.Errorf("user = %q, want ci", got)
	}
}
