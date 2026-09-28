package ftpx

import (
	"crypto/tls"
	"errors"
	"net"
	"testing"

	ftpserver "github.com/fclairamb/ftpserverlib"
	"github.com/spf13/afero"
)

const (
	testUser     = "tester"
	testPassword = "s3cret"
)

// testDriver serves one temporary directory to one hard coded account.
type testDriver struct {
	fs       afero.Fs
	listener net.Listener
}

func (d *testDriver) GetSettings() (*ftpserver.Settings, error) {
	return &ftpserver.Settings{
		Listener:            d.listener,
		DefaultTransferType: ftpserver.TransferTypeBinary,
	}, nil
}

func (d *testDriver) ClientConnected(ftpserver.ClientContext) (string, error) {
	return "go-ftp test server", nil
}

func (d *testDriver) ClientDisconnected(ftpserver.ClientContext) {
	// Nothing is held per client.
}

func (d *testDriver) AuthUser(_ ftpserver.ClientContext, user, password string) (ftpserver.ClientDriver, error) {
	if user != testUser || password != testPassword {
		return nil, errors.New("wrong credentials")
	}

	return d.fs, nil
}

func (d *testDriver) GetTLSConfig() (*tls.Config, error) {
	return nil, errors.New("this server speaks plain FTP only")
}

// startServer runs an FTP server in this process over a temporary directory and
// returns the address to dial and the directory it serves.
func startServer(t *testing.T) (addr, root string) {
	t.Helper()

	root = t.TempDir()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	server := ftpserver.NewFtpServer(&testDriver{
		fs:       afero.NewBasePathFs(afero.NewOsFs(), root),
		listener: listener,
	})

	// The socket is already accepting, so the address is usable before Serve
	// has got round to it.
	go func() {
		_ = server.ListenAndServe()
	}()
	t.Cleanup(func() {
		_ = server.Stop()
	})

	return listener.Addr().String(), root
}

// dial returns a client logged in to a fresh server.
func dial(t *testing.T) (*Client, string) {
	t.Helper()

	addr, root := startServer(t)

	client := New(Options{
		Addr:     addr,
		Host:     "127.0.0.1",
		User:     testUser,
		Password: testPassword,
	})
	t.Cleanup(client.Close)

	return client, root
}
