package transport

import (
	"context"
	"net/http"
	"time"
)

const (
	defaultMaxIdleConns        = 100
	defaultMaxIdleConnsPerHost = 100
	defaultIdleConnTimeout     = 90 * time.Second
)

// HTTPTransportOptions controls the connection pool used by an HTTP transport.
type HTTPTransportOptions struct {
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

// HTTPTransport implements the Transport interface for making HTTP requests.
type HTTPTransport struct {
	client *http.Client
}

// NewHTTPTransport creates a new HTTPTransport with the specified timeout.
func NewHTTPTransport(timeout time.Duration, options ...HTTPTransportOptions) *HTTPTransport {
	poolOptions := HTTPTransportOptions{
		MaxIdleConns:        defaultMaxIdleConns,
		MaxIdleConnsPerHost: defaultMaxIdleConnsPerHost,
		IdleConnTimeout:     defaultIdleConnTimeout,
	}
	if len(options) > 0 {
		if options[0].MaxIdleConns != 0 {
			poolOptions.MaxIdleConns = options[0].MaxIdleConns
		}
		if options[0].MaxIdleConnsPerHost != 0 {
			poolOptions.MaxIdleConnsPerHost = options[0].MaxIdleConnsPerHost
		}
		if options[0].IdleConnTimeout != 0 {
			poolOptions.IdleConnTimeout = options[0].IdleConnTimeout
		}
	}

	pool := http.DefaultTransport.(*http.Transport).Clone()
	pool.MaxIdleConns = poolOptions.MaxIdleConns
	pool.MaxIdleConnsPerHost = poolOptions.MaxIdleConnsPerHost
	pool.IdleConnTimeout = poolOptions.IdleConnTimeout
	return &HTTPTransport{
		client: &http.Client{
			Timeout:   timeout,
			Transport: pool,
		},
	}
}

// Do sends an HTTP request and returns an HTTP response.
func (t *HTTPTransport) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return t.client.Do(req)
}

var _ Transport = (*HTTPTransport)(nil)
