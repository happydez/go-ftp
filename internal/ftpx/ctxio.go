package ftpx

import (
	"context"
	"io"
)

// The FTP library takes no context, so cancellation is applied at the only place
// a long transfer passes through often enough to notice it.
type ctxReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *ctxReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}

	return r.r.Read(p)
}

type ctxReadCloser struct {
	ctxReader
	closer io.Closer
}

func (r *ctxReadCloser) Close() error {
	return r.closer.Close()
}
