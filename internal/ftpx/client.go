// Package ftpx wraps one FTP connection.
package ftpx

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

// DefaultTimeout is how long dialling and the commands on the control channel
// are given. It is not a limit on how long a transfer may take.
const DefaultTimeout = 30 * time.Second

// Options is everything one connection needs.
type Options struct {
	Addr        string
	Host        string
	User        string
	Password    string
	TLS         bool
	TLSInsecure bool
	Timeout     time.Duration
}

// Client is a single FTP connection. It is NOT safe for concurrent use, since
// the protocol gives one control channel per connection, so every worker holds
// its own.
type Client struct {
	opts  Options
	conn  *ftp.ServerConn
	known map[string]struct{}
}

func New(opts Options) *Client {
	if opts.Timeout <= 0 {
		opts.Timeout = DefaultTimeout
	}

	return &Client{
		opts:  opts,
		known: make(map[string]struct{}),
	}
}

// Connect opens the control connection and logs in. Calling it on a live client
// does nothing.
func (c *Client) Connect(ctx context.Context) error {
	if c.conn != nil {
		return nil
	}

	options := []ftp.DialOption{
		ftp.DialWithContext(ctx),
		ftp.DialWithTimeout(c.opts.Timeout),
	}
	if c.opts.TLS {
		options = append(options, ftp.DialWithExplicitTLS(&tls.Config{
			ServerName:         c.opts.Host,
			InsecureSkipVerify: c.opts.TLSInsecure,
			MinVersion:         tls.VersionTLS12,
		}))
	}

	conn, err := ftp.Dial(c.opts.Addr, options...)
	if err != nil {
		return fmt.Errorf("dial %s: %w", c.opts.Addr, err)
	}

	if err := conn.Login(c.opts.User, c.opts.Password); err != nil {
		_ = conn.Quit()

		return fmt.Errorf("log in as %s: %w", c.opts.User, err)
	}

	c.conn = conn

	return nil
}

// Close ends the session. Calling it twice is safe.
func (c *Client) Close() {
	if c.conn == nil {
		return
	}

	_ = c.conn.Quit()
	c.conn = nil
}

// Reset drops the connection so that the next call reconnects. Used after an
// error that may have left the control channel in an unknown state.
func (c *Client) Reset() {
	c.Close()
	c.known = make(map[string]struct{})
}

// EnsureDir creates a directory and every parent of it. MKD only ever creates
// one level, so the path is walked from the top.
func (c *Client) EnsureDir(ctx context.Context, dir string) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}
	if _, ok := c.known[dir]; ok {
		return nil
	}

	current := ""
	for _, part := range strings.Split(strings.Trim(dir, "/"), "/") {
		if part == "" {
			continue
		}

		current += "/" + part
		if _, ok := c.known[current]; ok {
			continue
		}

		_ = c.conn.MakeDir(current)
		if err := c.conn.ChangeDir(current); err != nil {
			return fmt.Errorf("cannot create or enter %s: %w", current, err)
		}

		c.known[current] = struct{}{}
	}

	return nil
}

// Size returns the size of a remote file, and false when it is not there.
func (c *Client) Size(ctx context.Context, remotePath string) (int64, bool) {
	if err := c.Connect(ctx); err != nil {
		return 0, false
	}

	size, err := c.conn.FileSize(remotePath)
	if err != nil {
		return 0, false
	}

	return size, true
}

// Upload stores r at remotePath. The directory has to exist already.
func (c *Client) Upload(ctx context.Context, remotePath string, r io.Reader) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}

	if err := c.conn.Stor(remotePath, &ctxReader{ctx: ctx, r: r}); err != nil {
		return fmt.Errorf("store %s: %w", remotePath, err)
	}

	return nil
}

// Download opens a remote file for reading.
func (c *Client) Download(ctx context.Context, remotePath string) (io.ReadCloser, error) {
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}

	resp, err := c.conn.Retr(remotePath)
	if err != nil {
		return nil, fmt.Errorf("retrieve %s: %w", remotePath, err)
	}

	return &ctxReadCloser{
		ctxReader: ctxReader{ctx: ctx, r: resp},
		closer:    resp,
	}, nil
}

// List reads one directory, without descending into it.
func (c *Client) List(ctx context.Context, dir string) ([]Entry, error) {
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}

	raw, err := c.conn.List(dir)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}

	entries := make([]Entry, 0, len(raw))
	for _, item := range raw {
		if item.Name == "." || item.Name == ".." {
			continue
		}
		entries = append(entries, newEntry(path.Join(dir, item.Name), item))
	}

	sortEntries(entries)

	return entries, nil
}

// Walk reads a directory and everything under it.
func (c *Client) Walk(ctx context.Context, dir string) ([]Entry, error) {
	if err := c.Connect(ctx); err != nil {
		return nil, err
	}

	var entries []Entry

	walker := c.conn.Walk(dir)
	for walker.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries = append(entries, newEntry(walker.Path(), walker.Stat()))
	}
	if err := walker.Err(); err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}

	sortEntries(entries)

	return entries, nil
}

// Stat describes one remote path and says whether it is there at all.
func (c *Client) Stat(ctx context.Context, remotePath string) (Entry, bool, error) {
	if err := c.Connect(ctx); err != nil {
		return Entry{}, false, err
	}

	if entry, err := c.conn.GetEntry(remotePath); err == nil {
		return newEntry(remotePath, entry), true, nil
	}

	parent, name := path.Split(strings.TrimRight(remotePath, "/"))
	if name == "" {
		return Entry{Path: "/", Name: "/", Dir: true}, true, nil
	}

	siblings, err := c.List(ctx, path.Clean(parent))
	if err != nil {
		return Entry{}, false, err
	}

	for _, entry := range siblings {
		if entry.Name == name {
			return entry, true, nil
		}
	}

	return Entry{}, false, nil
}

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
}

// Rename moves a remote file. An upload uses it to put a finished file in place
// without ever showing a half written one under the real name.
func (c *Client) Rename(ctx context.Context, from, to string) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}

	if err := c.conn.Rename(from, to); err != nil {
		return fmt.Errorf("rename %s to %s: %w", from, to, err)
	}

	return nil
}

// Remove deletes a remote file.
func (c *Client) Remove(ctx context.Context, remotePath string) error {
	if err := c.Connect(ctx); err != nil {
		return err
	}

	if err := c.conn.Delete(remotePath); err != nil {
		return fmt.Errorf("delete %s: %w", remotePath, err)
	}

	return nil
}
