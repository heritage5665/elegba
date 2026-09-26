package step

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	cachebackend "github.com/elegba-dev/elegba/internal/cache"
	"github.com/elegba-dev/elegba/internal/config"
)

type testCache struct {
	mu      sync.Mutex
	value   any
	key     string
	ttl     time.Duration
	updated chan struct{}
	getErr  error
	setErr  error
}

func (c *testCache) Get(_ context.Context, key string) (any, bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return nil, false, c.getErr
	}
	if c.value == nil || key != c.key {
		return nil, false, nil
	}
	return c.value, true, nil
}
func (c *testCache) Set(_ context.Context, key string, value any, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.setErr != nil {
		return c.setErr
	}
	c.key, c.value, c.ttl = key, value, ttl
	if c.updated != nil {
		select {
		case c.updated <- struct{}{}:
		default:
		}
	}
	return nil
}
func (c *testCache) Delete(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key == key {
		c.value = nil
	}
	return nil
}
func (c *testCache) Close() {}

func (c *testCache) snapshot() (string, any, time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.key, c.value, c.ttl
}

func TestFetchStepRendersRequestAuthenticatesAndCaches(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests++
		if req.URL.Path != "/users/7" || req.Method != http.MethodPost {
			t.Errorf("unexpected request %s %s", req.Method, req.URL.Path)
		}
		if req.Header.Get("Authorization") != "Bearer secret" || req.Header.Get("X-Trace") != "req-1" || req.Header.Get("X-Client") != "elegba" {
			t.Errorf("unexpected headers %v", req.Header)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil || string(body) != `{"id":"7"}` {
			t.Errorf("unexpected request body %q, %v", body, err)
		}
		_, _ = io.WriteString(w, `{"name":"Ada"}`)
	}))
	defer server.Close()
	backend := &testCache{}
	fetch, err := NewFetchStep(config.Step{
		ID: "user", Type: "fetch", Upstream: "users", Method: http.MethodPost,
		Path: "/users/{{ .path.id }}", Body: `{"id":"{{ .path.id }}"}`,
		Headers: map[string]string{"X-Trace": "{{ .requestID }}"},
		Cache:   &config.CacheRef{Backend: "memory", Key: "user:{{ .path.id }}", TTL: config.Duration(time.Minute)},
	}, config.Upstream{
		BaseURL: server.URL, Timeout: config.Duration(time.Second), MaxResponseBytes: 1024,
		Headers: map[string]string{"X-Client": "elegba"}, Auth: config.AuthConfig{Type: "bearer", Token: "secret"},
	}, backend, 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"path": map[string]any{"id": "7"}, "requestID": "req-1"}
	value, err := fetch.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	key, _, ttl := backend.snapshot()
	if value.(map[string]any)["name"] != "Ada" || key != "user:7" || ttl != time.Minute {
		t.Fatalf("unexpected fetch/cache result %#v, %#v", value, backend)
	}
	value, err = fetch.Execute(context.Background(), input)
	if err != nil || value.(map[string]any)["name"] != "Ada" || requests != 1 {
		t.Fatalf("expected cache hit, got value=%#v err=%v requests=%d", value, err, requests)
	}
}

func TestFetchStepServesStaleValueAndRevalidatesInBackground(t *testing.T) {
	updated := make(chan struct{}, 2)
	backend := &testCache{updated: updated}
	key := "user:7"
	if err := backend.Set(context.Background(), key, cachebackend.StaleValue{
		Value: map[string]any{"name": "stale"}, FreshUntil: time.Now().Add(-time.Second),
	}, time.Minute); err != nil {
		t.Fatal(err)
	}
	<-updated
	upstreamStarted := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamStarted <- struct{}{}
		_, _ = io.WriteString(w, `{"name":"fresh"}`)
	}))
	defer server.Close()
	fetch, err := NewFetchStep(config.Step{
		ID: "user", Type: "fetch", Upstream: "users", Path: "/users/{{ .path.id }}",
		Cache: &config.CacheRef{Backend: "memory", Key: "user:{{ .path.id }}", TTL: config.Duration(time.Minute), StaleWhileRevalidate: true},
	}, config.Upstream{BaseURL: server.URL, Timeout: config.Duration(time.Second), MaxResponseBytes: 1024}, backend, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"path": map[string]any{"id": "7"}}
	value, err := fetch.Execute(context.Background(), input)
	if err != nil || value.(map[string]any)["name"] != "stale" {
		t.Fatalf("expected stale result without waiting, got value=%#v err=%v", value, err)
	}
	select {
	case <-upstreamStarted:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	select {
	case <-updated:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not update the cache")
	}
	_, cached, _ := backend.snapshot()
	refreshed, ok := cached.(cachebackend.StaleValue)
	if !ok || refreshed.Value.(map[string]any)["name"] != "fresh" {
		t.Fatalf("expected refreshed stale entry, got %#v", cached)
	}
}

func TestFetchStepFailsOpenOnCacheErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"available":true}`)
	}))
	defer server.Close()
	backend := &testCache{getErr: errors.New("cache unavailable"), setErr: errors.New("cache unavailable")}
	fetch, err := NewFetchStep(config.Step{
		ID: "data", Type: "fetch", Upstream: "api", Cache: &config.CacheRef{Backend: "memory", Key: "data"},
	}, config.Upstream{BaseURL: server.URL, Timeout: config.Duration(time.Second), MaxResponseBytes: 1024}, backend, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	value, err := fetch.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("cache failures should not fail upstream fetch: %v", err)
	}
	if value.(map[string]any)["available"] != true {
		t.Fatalf("unexpected upstream value %#v", value)
	}
}

func TestFetchStepRejectsBadTemplatesStatusesAndLargeBodies(t *testing.T) {
	if _, err := NewFetchStep(config.Step{Path: "{{"}, config.Upstream{BaseURL: "https://example.test"}, nil, 0); err == nil {
		t.Fatal("expected invalid template error")
	}
	for _, test := range []struct {
		name     string
		status   int
		body     string
		maxBytes int64
		wantErr  string
	}{
		{name: "upstream failure", status: http.StatusBadGateway, body: "unavailable", maxBytes: 100, wantErr: "502"},
		{name: "response too large", status: http.StatusOK, body: strings.Repeat("x", 20), maxBytes: 4, wantErr: "exceeds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			fetch, err := NewFetchStep(config.Step{ID: "fetch"}, config.Upstream{
				BaseURL: server.URL, MaxResponseBytes: test.maxBytes, Timeout: config.Duration(time.Second),
			}, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = fetch.Execute(context.Background(), nil)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("expected %q error, got %v", test.wantErr, err)
			}
		})
	}
}

func TestApplyAuthModes(t *testing.T) {
	for _, test := range []struct {
		auth   config.AuthConfig
		want   string
		header string
	}{
		{auth: config.AuthConfig{Type: "bearer", Token: "token"}, header: "Authorization", want: "Bearer token"},
		{auth: config.AuthConfig{Type: "basic", User: "user", Pass: "pass"}, header: "Authorization", want: "Basic dXNlcjpwYXNz"},
		{auth: config.AuthConfig{Type: "apikey", Key: "key"}, header: "X-API-Key", want: "key"},
		{auth: config.AuthConfig{Type: "apikey", Header: "X-Key", Key: "key"}, header: "X-Key", want: "key"},
	} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		applyAuth(req, test.auth)
		if got := req.Header.Get(test.header); got != test.want {
			t.Fatalf("%s: expected %q, got %q", test.auth.Type, test.want, got)
		}
	}
}
