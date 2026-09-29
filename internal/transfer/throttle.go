package transfer

import (
	"context"
	"io"

	"golang.org/x/time/rate"
)

// minBurst keeps a very small limit usable.
const minBurst = 4 << 10

// Throttle holds bytes to a speed.
type Throttle struct {
	limiter *rate.Limiter
	burst   int
}

// NewThrottle returns a throttle for that many bytes a second, or nil when the
// answer is that there is no limit.
func NewThrottle(bytesPerSecond int64) *Throttle {
	if bytesPerSecond <= 0 {
		return nil
	}

	burst := max(int(min(bytesPerSecond, int64(1<<30))), minBurst)

	return &Throttle{
		limiter: rate.NewLimiter(rate.Limit(bytesPerSecond), burst),
		burst:   burst,
	}
}

// reader returns r held to the limit. A nil throttle hands r straight back, so
// the unlimited case costs nothing.
func (t *Throttle) reader(ctx context.Context, r io.Reader) io.Reader {
	if t == nil {
		return r
	}

	return &throttledReader{ctx: ctx, r: r, throttle: t}
}

// throttledReader waits for its allowance after every read.
type throttledReader struct {
	ctx      context.Context
	r        io.Reader
	throttle *Throttle
}

func (t *throttledReader) Read(p []byte) (int, error) {
	// Never ask for more in one go than the bucket can hold.
	if len(p) > t.throttle.burst {
		p = p[:t.throttle.burst]
	}

	n, err := t.r.Read(p)
	if n <= 0 {
		return n, err
	}

	if waitErr := t.throttle.limiter.WaitN(t.ctx, n); waitErr != nil {
		return n, waitErr
	}

	return n, err
}
