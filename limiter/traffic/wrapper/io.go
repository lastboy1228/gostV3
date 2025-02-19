package wrapper

import (
	"bytes"
	"context"
	"io"
	"time"

	limiter "github.com/go-gost/core/limiter/traffic"
)

// readWriter is an io.ReadWriter with traffic limiter supported.
type readWriter struct {
	io.ReadWriter
	rbuf       bytes.Buffer
	limiter    limiter.TrafficLimiter
	limiterIn  limiter.Limiter
	limiterOut limiter.Limiter
	expIn      int64
	expOut     int64
	opts       []limiter.Option
	key        string
}

func WrapReadWriter(limiter limiter.TrafficLimiter, rw io.ReadWriter, key string, opts ...limiter.Option) io.ReadWriter {
	if limiter == nil {
		return rw
	}

	return &readWriter{
		ReadWriter: rw,
		limiter:    limiter,
		opts:       opts,
	}
}

func (p *readWriter) getInLimiter() limiter.Limiter {
	now := time.Now().UnixNano()
	// cache the limiter for 60s
	if p.limiter != nil && time.Duration(now-p.expIn) > 60*time.Second {
		if lim := p.limiter.In(context.Background(), p.key, p.opts...); lim != nil {
			p.limiterIn = lim
		}
		p.expIn = now
	}
	return p.limiterIn
}

func (p *readWriter) getOutLimiter() limiter.Limiter {
	now := time.Now().UnixNano()
	// cache the limiter for 60s
	if p.limiter != nil && time.Duration(now-p.expOut) > 60*time.Second {
		if lim := p.limiter.Out(context.Background(), p.key, p.opts...); lim != nil {
			p.limiterOut = lim
		}
		p.expOut = now
	}
	return p.limiterOut
}

func (p *readWriter) Read(b []byte) (n int, err error) {
	limiter := p.getInLimiter()
	if limiter == nil {
		return p.ReadWriter.Read(b)
	}

	if p.rbuf.Len() > 0 {
		burst := len(b)
		if p.rbuf.Len() < burst {
			burst = p.rbuf.Len()
		}
		lim := limiter.Wait(context.Background(), burst)
		return p.rbuf.Read(b[:lim])
	}

	nn, err := p.ReadWriter.Read(b)
	if err != nil {
		return nn, err
	}

	n = limiter.Wait(context.Background(), nn)
	if n < nn {
		if _, err = p.rbuf.Write(b[n:nn]); err != nil {
			return 0, err
		}
	}

	return
}

func (p *readWriter) Write(b []byte) (n int, err error) {
	limiter := p.getOutLimiter()
	if limiter == nil {
		return p.ReadWriter.Write(b)
	}

	nn := 0
	for len(b) > 0 {
		nn, err = p.ReadWriter.Write(b[:limiter.Wait(context.Background(), len(b))])
		n += nn
		if err != nil {
			return
		}
		b = b[nn:]
	}

	return
}
