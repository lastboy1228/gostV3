package http2

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-gost/core/dialer"
	"github.com/go-gost/core/logger"
	md "github.com/go-gost/core/metadata"
	mdx "github.com/go-gost/x/metadata"
	"github.com/go-gost/x/registry"
	"golang.org/x/net/http2"
)

func init() {
	registry.DialerRegistry().Register("http2", NewDialer)
}

type http2Dialer struct {
	clients     map[string]*http.Client
	clientMutex sync.Mutex
	logger      logger.Logger
	md          metadata
	options     dialer.Options
}

func NewDialer(opts ...dialer.Option) dialer.Dialer {
	options := dialer.Options{}
	for _, opt := range opts {
		opt(&options)
	}

	return &http2Dialer{
		clients: make(map[string]*http.Client),
		logger:  options.Logger,
		options: options,
	}
}

func (d *http2Dialer) Init(md md.Metadata) (err error) {
	if err = d.parseMetadata(md); err != nil {
		return
	}

	return nil
}

// Multiplex implements dialer.Multiplexer interface.
func (d *http2Dialer) Multiplex() bool {
	return true
}

func (d *http2Dialer) Dial(ctx context.Context, address string, opts ...dialer.DialOption) (net.Conn, error) {
	raddr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		d.logger.Error(err)
		return nil, err
	}

	d.clientMutex.Lock()
	defer d.clientMutex.Unlock()

	client, ok := d.clients[address]
	if !ok {
		options := dialer.DialOptions{}
		for _, opt := range opts {
			opt(&options)
		}

		client = &http.Client{
			Transport: &http2.Transport{
				// 客户端发送ping frame进行连接探测；服务端IdleTimeout默认是5分钟，即使期间存在ping也认为是空闲连接
				ReadIdleTimeout: d.md.ttl,
				TLSClientConfig: d.options.TLSConfig,
				DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
					// NetDialer的DefaultTimeout是10s
					conn, err := options.NetDialer.Dial(ctx, network, addr)
					if err != nil {
						return nil, err
					}
					conn.SetDeadline(time.Now().Add(5 * time.Second))
					defer conn.SetDeadline(time.Time{})
					tlsConn := tls.Client(conn, cfg)
					if err = tlsConn.Handshake(); err != nil {
						tlsConn.Close()
						return nil, err
					}
					return tlsConn, nil
				},
			},
		}
		d.clients[address] = client
	}

	c := &conn{
		localAddr:  &net.TCPAddr{},
		remoteAddr: raddr,
		onClose: func() {
			d.clientMutex.Lock()
			defer d.clientMutex.Unlock()
			delete(d.clients, address)
		},
		md: mdx.NewMetadata(map[string]any{"client": client}),
	}

	return c, nil
}
