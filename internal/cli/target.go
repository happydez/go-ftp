package cli

import (
	"github.com/happydez/go-ftp/internal/config"
	"github.com/happydez/go-ftp/internal/creds"
)

// config loads the file the flags point at, or the first one the search finds.
// Anything wrong with it is the user's to fix, hence usageError.
func (g *globalOptions) config() (*config.Config, error) {
	cfg, err := config.LoadFound(g.configPath)
	if err != nil {
		return nil, usageError{err}
	}

	return cfg, nil
}

// target is the config together with the profile the command works against.
func (g *globalOptions) target() (*config.Config, config.Profile, error) {
	cfg, err := g.config()
	if err != nil {
		return nil, config.Profile{}, err
	}

	profile, err := cfg.Profile(g.profile)
	if err != nil {
		return nil, config.Profile{}, usageError{err}
	}

	return cfg, profile, nil
}

// credentials opens the password store the flags point at, or the default one.
func (g *globalOptions) credentials() (*creds.Store, error) {
	store, err := creds.Open(g.credsPath)
	if err != nil {
		return nil, usageError{err}
	}

	return store, nil
}
