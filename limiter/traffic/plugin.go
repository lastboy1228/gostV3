package traffic

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-gost/core/limiter/traffic"
	"github.com/go-gost/core/logger"
	"github.com/go-gost/plugin/limiter/traffic/proto"
	"github.com/go-gost/x/internal/plugin"
	"google.golang.org/grpc"
)

type grpcPlugin struct {
	conn   grpc.ClientConnInterface
	client proto.LimiterClient
	log    logger.Logger
}

// NewGRPCPlugin creates a traffic limiter plugin based on gRPC.
func NewGRPCPlugin(name string, addr string, opts ...plugin.Option) traffic.TrafficLimiter {
	var options plugin.Options
	for _, opt := range opts {
		opt(&options)
	}

	log := logger.Default().WithFields(map[string]any{
		"kind":    "limiter",
		"limiter": name,
	})
	conn, err := plugin.NewGRPCConn(addr, &options)
	if err != nil {
		log.Error(err)
	}

	p := &grpcPlugin{
		conn: conn,
		log:  log,
	}
	if conn != nil {
		p.client = proto.NewLimiterClient(conn)
	}
	return p
}

func (p *grpcPlugin) In(ctx context.Context, key string, opts ...traffic.Option) traffic.Limiter {
	if p.client == nil {
		return nil
	}

	var options traffic.Options
	for _, opt := range opts {
		opt(&options)
	}

	r, err := p.client.Limit(ctx,
		&proto.LimitRequest{
			Network: options.Network,
			Addr:    options.Addr,
			Client:  options.Client,
			Src:     options.Src,
		})
	if err != nil {
		p.log.Error(err)
		return nil
	}

	return NewLimiter(int(r.In))
}

func (p *grpcPlugin) Out(ctx context.Context, key string, opts ...traffic.Option) traffic.Limiter {
	if p.client == nil {
		return nil
	}

	var options traffic.Options
	for _, opt := range opts {
		opt(&options)
	}

	r, err := p.client.Limit(ctx,
		&proto.LimitRequest{
			Network: options.Network,
			Addr:    options.Addr,
			Client:  options.Client,
			Src:     options.Src,
		})
	if err != nil {
		p.log.Error(err)
		return nil
	}

	return NewLimiter(int(r.Out))
}

func (p *grpcPlugin) Close() error {
	if closer, ok := p.conn.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

type httpPluginRequest struct {
	Network string `json:"network"`
	Addr    string `json:"addr"`
	Client  string `json:"client"`
	Src     string `json:"src"`
}

type httpPluginResponse struct {
	In  int64 `json:"in"`
	Out int64 `json:"out"`
}

type httpPlugin struct {
	url    string
	client *http.Client
	header http.Header
	log    logger.Logger
}

// NewHTTPPlugin creates a traffic limiter plugin based on HTTP.
func NewHTTPPlugin(name string, url string, opts ...plugin.Option) traffic.TrafficLimiter {
	var options plugin.Options
	for _, opt := range opts {
		opt(&options)
	}

	return &httpPlugin{
		url:    url,
		client: plugin.NewHTTPClient(&options),
		header: options.Header,
		log: logger.Default().WithFields(map[string]any{
			"kind":    "limiter",
			"limiter": name,
		}),
	}
}

func (p *httpPlugin) In(ctx context.Context, key string, opts ...traffic.Option) traffic.Limiter {
	if p.client == nil {
		return nil
	}

	var options traffic.Options
	for _, opt := range opts {
		opt(&options)
	}

	rb := httpPluginRequest{
		Network: options.Network,
		Addr:    options.Addr,
		Client:  options.Client,
		Src:     options.Src,
	}
	v, err := json.Marshal(&rb)
	if err != nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(v))
	if err != nil {
		return nil
	}

	if p.header != nil {
		req.Header = p.header.Clone()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	res := httpPluginResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil
	}
	return NewLimiter(int(res.In))
}

func (p *httpPlugin) Out(ctx context.Context, key string, opts ...traffic.Option) traffic.Limiter {
	if p.client == nil {
		return nil
	}

	var options traffic.Options
	for _, opt := range opts {
		opt(&options)
	}

	rb := httpPluginRequest{
		Network: options.Network,
		Addr:    options.Addr,
		Client:  options.Client,
		Src:     options.Src,
	}
	v, err := json.Marshal(&rb)
	if err != nil {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.url, bytes.NewReader(v))
	if err != nil {
		return nil
	}

	if p.header != nil {
		req.Header = p.header.Clone()
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	res := httpPluginResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return nil
	}
	return NewLimiter(int(res.Out))
}
