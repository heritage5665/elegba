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
