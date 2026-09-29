// Package config loads the file that describes the FTP targets.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Defaults for everything the file may leave out.
const (
	defaultPort       = 21
	defaultBaseDir    = "/"
	defaultWorkers    = 4
	defaultMaxRetries = 3
	// No deadline by default. A wall clock is the wrong tool for a run whose
	// length depends on how much there is to move, and a stuck connection is
	// caught by the connection timeout instead.
	defaultTimeout = 0 * time.Second
)

type Config struct {
	// Which profile the commands work with when --profile is not given. May be
	// left out when the file holds exactly one profile.
	CurrentProfile string             `yaml:"current_profile" json:"current_profile"`
	Profiles       map[string]Profile `yaml:"profiles" json:"profiles"`
	Transfer       Transfer           `yaml:"transfer" json:"transfer"`
	path           string
}

// Profile is one FTP target. It holds no password, that lives outside the file.
type Profile struct {
	Host    string `yaml:"host" json:"host"`
	Port    int    `yaml:"port" json:"port"`
	BaseDir string `yaml:"base_dir" json:"base_dir"`
	User    string `yaml:"user" json:"user"`
	// Explicit FTPS over AUTH TLS. Plain FTP when false.
	TLS         bool `yaml:"tls" json:"tls"`
	TLSInsecure bool `yaml:"tls_insecure" json:"tls_insecure"`
	name        string
}

type Transfer struct {
	// How many files travel at once. Every worker holds its own FTP connection.
	Workers int `yaml:"workers" json:"workers"`
	// Attempts per file on top of the first try, 0 means no second chance.
	MaxRetries int `yaml:"max_retries" json:"max_retries"`
	// A deadline for the whole run, not for one file. 0 means no limit.
	Timeout Duration `yaml:"timeout" json:"timeout"`
	// How fast bytes may travel, added up over the whole run rather than per
	// connection. 0 means no limit.
	UploadLimit   Size `yaml:"upload_limit" json:"upload_limit"`
	DownloadLimit Size `yaml:"download_limit" json:"download_limit"`
}

// Default is what a config looks like before a file is read.
func Default() *Config {
	return &Config{
		Profiles: make(map[string]Profile),
		Transfer: Transfer{
			Workers:    defaultWorkers,
			MaxRetries: defaultMaxRetries,
			Timeout:    Duration(defaultTimeout),
		},
	}
}

// Load reads one file and checks everything that does not depend on which
// profile is picked later.
func Load(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := Default()
	cfg.path = configPath

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parse %s: %w", configPath, err)
	}

	cfg.normalizeProfiles()
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Path is the file this config came from.
func (c *Config) Path() string {
	return c.path
}

// Profile returns the target to work with. An empty name means current_profile,
// or the only profile there is when the file does not name one.
func (c *Config) Profile(name string) (Profile, error) {
	if name == "" {
		name = c.CurrentProfile
	}
	if name == "" && len(c.Profiles) == 1 {
		for only := range c.Profiles {
			name = only
		}
	}
	if name == "" {
		return Profile{}, fmt.Errorf("no profile chosen, set current_profile in %s or pass --profile", c.path)
	}

	p, ok := c.Profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("profile %q is not defined in %s, available: %s",
			name, c.path, strings.Join(c.ProfileNames(), ", "))
	}

	p.name = name
	if err := p.validate(); err != nil {
		return Profile{}, err
	}

	return p, nil
}

// ProfileNames lists the defined profiles in a stable order.
func (c *Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

func (p Profile) Name() string {
	return p.name
}

// Addr is the host:port the FTP client dials.
func (p Profile) Addr() string {
	return net.JoinHostPort(p.Host, strconv.Itoa(p.Port))
}

func (c *Config) normalizeProfiles() {
	for name, p := range c.Profiles {
		if p.Port == 0 {
			p.Port = defaultPort
		}
		p.BaseDir = normalizeDir(p.BaseDir)
		c.Profiles[name] = p
	}
}

// validate collects every problem it can find, so that one run fixes the whole
// file instead of one line per attempt.
func (c *Config) validate() error {
	var problems []string

	if len(c.Profiles) == 0 {
		problems = append(problems, "no profiles are defined, at least one is required")
	}
	if c.CurrentProfile != "" && len(c.Profiles) > 0 {
		if _, ok := c.Profiles[c.CurrentProfile]; !ok {
			problems = append(problems, fmt.Sprintf("current_profile %q is not among the profiles (%s)", c.CurrentProfile, strings.Join(c.ProfileNames(), ", ")))
		}
	}
	if c.CurrentProfile == "" && len(c.Profiles) > 1 {
		problems = append(problems, "current_profile is required when more than one profile is defined")
	}
	if c.Transfer.Workers < 1 {
		problems = append(problems, fmt.Sprintf("transfer.workers must be at least 1, got %d", c.Transfer.Workers))
	}
	if c.Transfer.MaxRetries < 0 {
		problems = append(problems, fmt.Sprintf("transfer.max_retries cannot be negative, got %d", c.Transfer.MaxRetries))
	}
	if c.Transfer.Timeout < 0 {
		problems = append(problems, fmt.Sprintf("transfer.timeout cannot be negative, got %s", c.Transfer.Timeout))
	}
	if c.Transfer.UploadLimit < 0 {
		problems = append(problems, fmt.Sprintf("transfer.upload_limit cannot be negative, got %s", c.Transfer.UploadLimit))
	}
	if c.Transfer.DownloadLimit < 0 {
		problems = append(problems, fmt.Sprintf("transfer.download_limit cannot be negative, got %s", c.Transfer.DownloadLimit))
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("%s: %s", c.path, strings.Join(problems, "; "))
}

func (p Profile) validate() error {
	var problems []string

	if p.Host == "" {
		problems = append(problems, "host is required")
	}
	if p.Port < 1 || p.Port > 65535 {
		problems = append(problems, fmt.Sprintf("port must be between 1 and 65535, got %d", p.Port))
	}
	if p.User == "" {
		problems = append(problems, "user is required")
	}
	if p.TLSInsecure && !p.TLS {
		problems = append(problems, "tls_insecure has no effect while tls is off")
	}

	if len(problems) == 0 {
		return nil
	}

	return fmt.Errorf("profile %q: %s", p.name, strings.Join(problems, "; "))
}

func normalizeDir(dir string) string {
	if dir == "" {
		return defaultBaseDir
	}
	dir = strings.ReplaceAll(dir, `\`, "/")
	if !strings.HasPrefix(dir, "/") {
		dir = "/" + dir
	}

	return path.Clean(dir)
}
