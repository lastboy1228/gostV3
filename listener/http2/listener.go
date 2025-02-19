package http2

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/go-gost/core/listener"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	admission "github.com/go-gost/x/admission/wrapper"
	"github.com/go-gost/x/config/parsing"
	xnet "github.com/go-gost/x/internal/net"
	"github.com/go-gost/x/internal/net/proxyproto"
	climiter "github.com/go-gost/x/limiter/conn/wrapper"
	limiter "github.com/go-gost/x/limiter/traffic/wrapper"
	mdx "github.com/go-gost/x/metadata"
	metrics "github.com/go-gost/x/metrics/wrapper"
	"github.com/go-gost/x/registry"
	"golang.org/x/net/http2"
)

func init() {
	registry.ListenerRegistry().Register("http2", NewListener)
}

type http2Listener struct {
	server  *http.Server
	addr    net.Addr
	cqueue  chan net.Conn
	errChan chan error
	logger  logger.Logger
	md      metadata
	options listener.Options
}

func NewListener(opts ...listener.Option) listener.Listener {
	options := listener.Options{}
	for _, opt := range opts {
		opt(&options)
	}
	return &http2Listener{
		logger:  options.Logger,
		options: options,
	}
}

func (l *http2Listener) Init(md md.Metadata) (err error) {
	if err = l.parseMetadata(md); err != nil {
		return
	}
	if l.options.TLSConfig == nil {
		l.options.TLSConfig = parsing.DefaultTLSConfig().Clone()
	}
	l.options.TLSConfig.MinVersion = tls.VersionTLS13
	l.options.TLSConfig.CurvePreferences = []tls.CurveID{tls.X25519}
	l.server = &http.Server{
		Addr:        l.options.Addr,
		Handler:     http.HandlerFunc(l.handleFunc),
		TLSConfig:   l.options.TLSConfig,
		IdleTimeout: l.md.maxIdleTimeout,
	}
	if err := http2.ConfigureServer(l.server, nil); err != nil {
		return err
	}

	network := "tcp"
	if xnet.IsIPv4(l.options.Addr) {
		network = "tcp4"
	}
	lc := net.ListenConfig{}
	if l.md.mptcp {
		lc.SetMultipathTCP(true)
		l.logger.Debugf("mptcp enabled: %v", lc.MultipathTCP())
	}
	ln, err := lc.Listen(context.Background(), network, l.options.Addr)
	if err != nil {
		return err
	}
	l.addr = ln.Addr()
	ln = proxyproto.WrapListener(l.options.ProxyProtocol, ln, 10*time.Second)
	ln = metrics.WrapListener(l.options.Service, ln)
	ln = admission.WrapListener(l.options.Admission, ln)
	ln = limiter.WrapListener(l.options.TrafficLimiter, ln)
	ln = climiter.WrapListener(l.options.ConnLimiter, ln)

	ln = tls.NewListener(
		ln,
		l.options.TLSConfig,
	)

	l.cqueue = make(chan net.Conn, l.md.backlog)
	l.errChan = make(chan error, 1)

	go func() {
		if err := l.server.Serve(ln); err != nil {
			l.logger.Error(err)
		}
	}()

	return
}

func (l *http2Listener) Accept() (conn net.Conn, err error) {
	var ok bool
	select {
	case conn = <-l.cqueue:
	case err, ok = <-l.errChan:
		if !ok {
			err = listener.ErrClosed
		}
	}
	return
}

func (l *http2Listener) Addr() net.Addr {
	return l.addr
}

func (l *http2Listener) Close() (err error) {
	select {
	case <-l.errChan:
	default:
		err = l.server.Close()
		l.errChan <- http.ErrServerClosed
		close(l.errChan)
	}
	return
}

func (l *http2Listener) handleFunc(w http.ResponseWriter, r *http.Request) {
	raddr, _ := net.ResolveTCPAddr("tcp", r.RemoteAddr)
	conn := &conn{
		laddr:  l.addr,
		raddr:  raddr,
		closed: make(chan struct{}),
		md: mdx.NewMetadata(map[string]any{
			"r": r,
			"w": w,
		}),
	}
	select {
	case l.cqueue <- conn:
	default:
		l.logger.Warnf("connection queue is full, client %s discarded", r.RemoteAddr)
		return
	}

	<-conn.Done()
}
