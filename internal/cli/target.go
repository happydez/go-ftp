package cli

import (
	"context"
	"strings"

	"github.com/happydez/go-ftp/internal/config"
	"github.com/happydez/go-ftp/internal/creds"
	"github.com/happydez/go-ftp/internal/ftpx"
	"github.com/happydez/go-ftp/internal/transfer"
	"github.com/happydez/go-ftp/internal/ui"
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

// session is the resolved answer to which server a command works with and how
// it authenticates.
type session struct {
	cfg      *config.Config
	profile  config.Profile
	store    *creds.Store
	password string
	source   creds.Source
}

// session gathers the config, the profile and the password.
func (g *globalOptions) session() (*session, error) {
	cfg, profile, err := g.target()
	if err != nil {
		return nil, err
	}

	store, err := g.credentials()
	if err != nil {
		return nil, err
	}

	password, source, err := creds.Resolve(creds.Options{
		Profile: profile.Name(),
		Store:   store,
	})
	if err != nil {
		password, source = "", creds.SourceNone
	}

	return &session{
		cfg:      cfg,
		profile:  profile,
		store:    store,
		password: password,
		source:   source,
	}, nil
}

// client builds a connection for the profile, and is where a missing password
// finally becomes an error.
func (s *session) client() (*ftpx.Client, error) {
	if s.source == creds.SourceNone {
		return nil, newUsageError("no password for profile %q, run `go-ftp login` or set %s", s.profile.Name(), creds.EnvPassword)
	}

	return newClient(s.profile, s.password), nil
}

func newClient(profile config.Profile, password string) *ftpx.Client {
	return ftpx.New(ftpx.Options{
		Addr:        profile.Addr(),
		Host:        profile.Host,
		User:        creds.User(profile.User),
		Password:    password,
		TLS:         profile.TLS,
		TLSInsecure: profile.TLSInsecure,
	})
}

// connect hands out a fresh connection per worker. The password is checked once
// here, so that a run does not start only to fail on every single file.
func (s *session) connect() (func() *ftpx.Client, error) {
	if _, err := s.client(); err != nil {
		return nil, err
	}

	return func() *ftpx.Client {
		return newClient(s.profile, s.password)
	}, nil
}

// withTimeout applies the deadline for the whole run, if the config sets one.
func (s *session) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := s.cfg.Transfer.Timeout.Unwrap()
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}

	return context.WithTimeout(ctx, timeout)
}

func (s *session) workers(fromFlag int) int {
	if fromFlag > 0 {
		return fromFlag
	}

	return s.cfg.Transfer.Workers
}

// remotePath is the one way a remote path argument reaches the rest of the
// program. It undoes what the shell may have done to it, then resolves it
// against base_dir.
func (s *session) remotePath(raw string) (string, error) {
	cleaned, fixed, err := unmangleRemote(raw)
	if err != nil {
		return "", usageError{err}
	}
	if fixed != nil {
		ui.Warn("%s", fixed)
		ui.Warn("using %s, write it as //%s or set MSYS_NO_PATHCONV=1 to keep the shell out of it", ui.Path(cleaned), strings.TrimLeft(cleaned, "/"))
	}

	resolved, err := transfer.Resolve(s.profile.BaseDir, cleaned)
	if err != nil {
		return "", usageError{err}
	}

	return resolved, nil
}

// throttle builds the speed limit for a run. The flag wins over the config, and
// an empty flag leaves the config alone.
func (s *session) throttle(fromFlag string, fromConfig config.Size) (*transfer.Throttle, error) {
	limit := fromConfig

	if fromFlag != "" {
		parsed, err := config.ParseSize(fromFlag)
		if err != nil {
			return nil, usageError{err}
		}
		limit = parsed
	}

	if limit > 0 {
		ui.Info("holding the run to %s a second", ui.Bold(limit.String()))
	}

	return transfer.NewThrottle(limit.Bytes()), nil
}
