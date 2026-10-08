package transport

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTransportForwardsRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("X-Test") != "value" {
			t.Errorf("header was not forwarded: %v", req.Header)
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	transport := NewHTTPTransport(time.Second)
	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("X-Test", "value")
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("unexpected response %q, %v", body, err)
	}
}

func TestHTTPTransportHonorsCanceledContext(t *testing.T) {
	transport := NewHTTPTransport(time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transport.Do(ctx, req); err == nil {
		t.Fatal("expected canceled request error")
	}
}

func TestHTTPTransportConfiguresConnectionPool(t *testing.T) {
	transport := NewHTTPTransport(time.Second, HTTPTransportOptions{
		MaxIdleConns:        25,
		MaxIdleConnsPerHost: 5,
		IdleConnTimeout:     30 * time.Second,
	})
	pool, ok := transport.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("HTTP transport = %T, want *http.Transport", transport.client.Transport)
	}
	if pool.MaxIdleConns != 25 {
		t.Fatalf("MaxIdleConns = %d, want 25", pool.MaxIdleConns)
	}
	if pool.MaxIdleConnsPerHost != 5 {
		t.Fatalf("MaxIdleConnsPerHost = %d, want 5", pool.MaxIdleConnsPerHost)
	}
	if pool.IdleConnTimeout != 30*time.Second {
		t.Fatalf("IdleConnTimeout = %s, want 30s", pool.IdleConnTimeout)
	}
}
