package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func newTestResilient(t *testing.T, server *httptest.Server, options ResilienceOptions) *ResilientTransport {
	t.Helper()
	resilient, err := NewResilientTransport("test", NewHTTPTransport(2*time.Second), options)
	if err != nil {
		t.Fatal(err)
	}
	return resilient
}

func TestResilientTransportRetriesIdempotentRequests(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{Retries: RetryOptions{
		MaxRetries: 2, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond,
		Multiplier: 1, MaxElapsedTime: time.Second,
	}})
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || calls.Load() != 3 {
		t.Fatalf("status=%d calls=%d", response.StatusCode, calls.Load())
	}
}

func TestResilientTransportExhaustsRetryBudget(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{Retries: RetryOptions{
		MaxRetries: 1, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond,
		Multiplier: 1, MaxElapsedTime: time.Second,
	}})
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || calls.Load() != 2 {
		t.Fatalf("retry budget not enforced: status=%d calls=%d", response.StatusCode, calls.Load())
	}
}

func TestResilientTransportRetriesConnectionReset(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			connection, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Errorf("hijack failed: %v", err)
				return
			}
			_ = connection.Close()
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{Retries: RetryOptions{
		MaxRetries: 1, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond,
		Multiplier: 1, MaxElapsedTime: time.Second,
	}})
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "ok" || calls.Load() != 2 {
		t.Fatalf("network retry failed: body=%q calls=%d err=%v", body, calls.Load(), err)
	}
}

func TestResilientTransportDoesNotRetryNonIdempotentRequests(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{Retries: RetryOptions{
		MaxRetries: 3, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond,
		Multiplier: 1, MaxElapsedTime: time.Second,
	}})
	req, _ := http.NewRequest(http.MethodPost, server.URL, nil)
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || calls.Load() != 1 {
		t.Fatalf("status=%d calls=%d", response.StatusCode, calls.Load())
	}
}

func TestResilientTransportHonorsRetryAfter(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{Retries: RetryOptions{
		MaxRetries: 1, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond,
		Multiplier: 1, MaxElapsedTime: 2 * time.Second,
	}})
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	started := time.Now()
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || time.Since(started) < 900*time.Millisecond {
		t.Fatalf("Retry-After not honored: status=%d elapsed=%s", response.StatusCode, time.Since(started))
	}
}

func TestResilientTransportStopsAtRetryElapsedBudget(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{Retries: RetryOptions{
		MaxRetries: 5, InitialInterval: time.Millisecond, MaxInterval: time.Millisecond,
		Multiplier: 1, MaxElapsedTime: 10 * time.Millisecond,
	}})
	req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	started := time.Now()
	response, err := transport.Do(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusTooManyRequests || calls.Load() != 1 || time.Since(started) >= 500*time.Millisecond {
		t.Fatalf("elapsed retry budget was not enforced: status=%d calls=%d elapsed=%s", response.StatusCode, calls.Load(), time.Since(started))
	}
}

func TestResilientTransportBreakerOpensAndRecovers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{
		Retries: RetryOptions{MaxRetries: 0},
		Breaker: &BreakerOptions{MaxRequests: 1, Interval: time.Minute, Timeout: 20 * time.Millisecond, FailureThreshold: 1},
	})
	request := func() (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			return nil, err
		}
		return transport.Do(context.Background(), req)
	}
	response, err := request()
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unexpected initial response status %d", response.StatusCode)
	}
	if _, err := request(); !errors.Is(err, ErrCircuitBreakerOpen) {
		t.Fatalf("expected open-circuit error, got %v", err)
	}
	time.Sleep(30 * time.Millisecond)
	response, err = request()
	if err != nil {
		t.Fatalf("breaker did not recover to half-open: %v", err)
	}
	_ = response.Body.Close()
}

func TestResilientTransportRateLimitWaitHonorsContext(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{RateLimit: "1/s"})
	request := func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			return nil, err
		}
		return transport.Do(ctx, req)
	}
	response, err := request(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := request(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected rate-limit wait to stop at deadline, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one upstream call, got %d", calls.Load())
	}
}

func TestResilientTransportConcurrencyLimitHonorsContext(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
			w.WriteHeader(http.StatusOK)
		case <-req.Context().Done():
		}
	}))
	defer server.Close()
	transport := newTestResilient(t, server, ResilienceOptions{MaxConcurrent: 1})
	newRequest := func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			return nil, err
		}
		return transport.Do(ctx, req)
	}
	firstDone := make(chan error, 1)
	go func() {
		response, err := newRequest(context.Background())
		if response != nil {
			_ = response.Body.Close()
		}
		firstDone <- err
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := newRequest(ctx)
		secondDone <- err
	}()
	cancel()
	if err := <-secondDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled semaphore wait, got %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Fatalf("expected only one active upstream request, got %d", calls.Load())
	}
}

func TestResilientTransportHoldsConcurrencySlotUntilBodyClose(t *testing.T) {
	started := make(chan struct{}, 1)
	releaseServer := make(chan struct{})
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		started <- struct{}{}
		select {
		case <-releaseServer:
			_, _ = io.WriteString(w, "ok")
		case <-req.Context().Done():
		}
	}))
	defer server.Close()
	defer close(releaseServer)
	transport := newTestResilient(t, server, ResilienceOptions{MaxConcurrent: 1})
	firstRequest, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	firstResponse, err := transport.Do(context.Background(), firstRequest)
	if err != nil {
		t.Fatal(err)
	}
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	secondRequest, _ := http.NewRequest(http.MethodGet, server.URL, nil)
	if _, err := transport.Do(ctx, secondRequest); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected second request to wait for body close, got %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("second request entered upstream before first body closed: %d calls", calls.Load())
	}
	_ = firstResponse.Body.Close()
}
