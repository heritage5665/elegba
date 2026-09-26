package transport

import (
	"context"
	"net/http"
	"time"
)

// HTTPTransport implements the Transport interface for making HTTP requests.
type HTTPTransport struct {
	client *http.Client
}

// NewHTTPTransport creates a new HTTPTransport with the specified timeout.
func NewHTTPTransport(timeout time.Duration) *HTTPTransport {
	return &HTTPTransport{
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// Do sends an HTTP request and returns an HTTP response.
func (t *HTTPTransport) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return t.client.Do(req)
}

var _ Transport = (*HTTPTransport)(nil)
