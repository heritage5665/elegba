package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/elegba-dev/elegba/internal/config"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestEngineAggregatesUpstreamsAndInjectsRequestData(t *testing.T) {
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/users/42" {
			t.Errorf("unexpected user path %q", req.URL.Path)
		}
		_, _ = w.Write([]byte(`{"name":"Ada"}`))
	}))
	defer userServer.Close()
	ordersServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/orders/42" {
			t.Errorf("unexpected orders path %q", req.URL.Path)
		}
		_, _ = w.Write([]byte(`["book","pen"]`))
	}))
	defer ordersServer.Close()

	cfg := &config.Config{
		Version: "1",
		Upstreams: map[string]config.Upstream{
			"users":  {BaseURL: userServer.URL},
			"orders": {BaseURL: ordersServer.URL, Timeout: config.Duration(time.Second)},
		},
		Endpoints: []config.Endpoint{{
			Path:   "/dashboard/{id}",
			Method: http.MethodGet,
			Pipeline: []config.Step{
				{ID: "user", Type: "fetch", Upstream: "users", Path: "/users/{{ .path.id }}"},
				{ID: "orders", Type: "fetch", Upstream: "orders", Path: "/orders/{{ .path.id }}"},
				{ID: "combined", Type: "transform", DependsOn: []string{"user", "orders"}, Template: `{"user": {{ .user | toJSON }}, "orders": {{ .orders | toJSON }}, "search": {{ .query.q | toJSON }}}`},
			},
		}},
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dashboard/42?q=summary", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("unexpected status %d: %s", response.Code, response.Body.String())
	}
	if id := response.Header().Get("X-Request-ID"); len(id) != 26 {
		t.Fatalf("expected 26-character ULID request id, got %q", id)
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["search"] != "summary" || !strings.Contains(fmt.Sprint(result["user"]), "Ada") {
		t.Fatalf("unexpected aggregate response: %#v", result)
	}
	orders, ok := result["orders"].([]any)
	if !ok || len(orders) != 2 {
		t.Fatalf("unexpected orders result: %#v", result["orders"])
	}
}

func TestEngineHealthAndMethodMismatch(t *testing.T) {
	app, err := New(&config.Config{
		Version:   "1",
		Upstreams: map[string]config.Upstream{"users": {BaseURL: "https://example.test"}},
		Endpoints: []config.Endpoint{{Path: "/users/{id}", Method: http.MethodGet, Pipeline: []config.Step{{ID: "user", Type: "fetch", Upstream: "users"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	health := httptest.NewRecorder()
	app.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("health check returned %d", health.Code)
	}
	wrongMethod := httptest.NewRecorder()
	app.ServeHTTP(wrongMethod, httptest.NewRequest(http.MethodPost, "/users/1", nil))
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for wrong method, got %d", wrongMethod.Code)
	}
}

func TestEngineMetricsRecordRequests(t *testing.T) {
	app, err := New(&config.Config{
		Version:   "1",
		Upstreams: map[string]config.Upstream{"users": {BaseURL: "https://example.test"}},
		Endpoints: []config.Endpoint{{Path: "/users/{id}", Method: http.MethodGet, Pipeline: []config.Step{{ID: "user", Type: "transform", Template: `{"id":{{ .path.id | toJSON }}}`}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	metrics := httptest.NewRecorder()
	app.MetricsHandler().ServeHTTP(metrics, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK || !strings.Contains(metrics.Body.String(), `elegba_requests_total{endpoint="/users/{id}",method="GET",status="200"} 1`) || !strings.Contains(metrics.Body.String(), `elegba_step_duration_seconds_count{endpoint="/users/{id}",step="user",upstream=""} 1`) {
		t.Fatalf("request metrics missing: response=%d metrics=%s", response.Code, metrics.Body.String())
	}
}

func TestEngineReadinessProbesUpstreams(t *testing.T) {
	var unhealthy atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if unhealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	app, err := New(&config.Config{
		Version:   "1",
		Server:    config.ServerConfig{ReadinessTimeout: config.Duration(time.Second)},
		Upstreams: map[string]config.Upstream{"api": {BaseURL: upstream.URL}},
		Endpoints: []config.Endpoint{{Path: "/ready", Method: http.MethodGet, Pipeline: []config.Step{{ID: "ready", Type: "transform", Template: "ok"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := app.Readiness(context.Background()); err != nil {
		t.Fatalf("healthy upstream failed readiness: %v", err)
	}
	unhealthy.Store(true)
	if err := app.Readiness(context.Background()); err == nil {
		t.Fatal("expected unhealthy upstream to fail readiness")
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"NOT_READY"`) {
		t.Fatalf("unexpected readiness response %d: %s", response.Code, response.Body.String())
	}
}

func TestFetchStepServesSecondRequestFromCache(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"name":"Ada"}`))
	}))
	defer upstream.Close()
	app, err := New(&config.Config{
		Version: "1",
		Caches: map[string]config.Cache{
			"memory": {Type: "in-memory", MaxSize: 100, DefaultTTL: config.Duration(time.Minute)},
		},
		Upstreams: map[string]config.Upstream{"users": {BaseURL: upstream.URL}},
		Endpoints: []config.Endpoint{{Path: "/users/{id}", Method: http.MethodGet, Pipeline: []config.Step{{
			ID: "user", Type: "fetch", Upstream: "users", Path: "/users/{{ .path.id }}",
			Cache: &config.CacheRef{Backend: "memory", Key: "user:{{ .path.id }}"},
		}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	for range 2 {
		response := httptest.NewRecorder()
		app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/users/42", nil))
		if response.Code != http.StatusOK {
			t.Fatalf("unexpected response status %d: %s", response.Code, response.Body.String())
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("expected one upstream request, got %d", calls.Load())
	}
}

func TestEndpointCacheInvalidationRunsBeforePipeline(t *testing.T) {
	app, err := New(&config.Config{
		Version: "1",
		Caches: map[string]config.Cache{
			"memory": {Type: "in-memory", MaxSize: 100, DefaultTTL: config.Duration(time.Minute)},
		},
		Endpoints: []config.Endpoint{
			{Path: "/seed", Method: http.MethodGet, Pipeline: []config.Step{
				{ID: "value", Type: "transform", Template: `{"cached":true}`},
				{ID: "store", Type: "cache", Action: "set", Backend: "memory", Key: "key:{{ .query.id }}", Value: "value", DependsOn: []string{"value"}},
			}},
			{Path: "/invalidate", Method: http.MethodGet, CacheInvalidate: []string{"key:{{ .query.id }}"}, Pipeline: []config.Step{
				{ID: "cached", Type: "cache", Action: "get", Backend: "memory", Key: "key:{{ .query.id }}"},
				{ID: "result", Type: "transform", DependsOn: []string{"cached"}, Template: `{{ .cached | toJSON }}`},
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	seed := httptest.NewRecorder()
	app.ServeHTTP(seed, httptest.NewRequest(http.MethodGet, "/seed?id=7", nil))
	if seed.Code != http.StatusOK {
		t.Fatalf("could not seed cache: %d %s", seed.Code, seed.Body.String())
	}
	invalidated := httptest.NewRecorder()
	app.ServeHTTP(invalidated, httptest.NewRequest(http.MethodGet, "/invalidate?id=7", nil))
	if invalidated.Code != http.StatusOK || strings.TrimSpace(invalidated.Body.String()) != "null" {
		t.Fatalf("cache entry survived endpoint invalidation: %d %s", invalidated.Code, invalidated.Body.String())
	}
}

func TestEngineEnforcesBodyLimitAndMapsPipelineFailure(t *testing.T) {
	failFast := true
	cfg := &config.Config{
		Version: "1",
		Server:  config.ServerConfig{MaxBodyBytes: 4},
		Upstreams: map[string]config.Upstream{
			"failure": {BaseURL: "https://example.test"},
		},
		Endpoints: []config.Endpoint{
			{Path: "/echo", Method: http.MethodPost, Pipeline: []config.Step{{ID: "echo", Type: "transform", Template: `{{ .body | toJSON }}`}}},
			{Path: "/failure", Method: http.MethodGet, FailFast: &failFast, Pipeline: []config.Step{{ID: "fetch", Type: "fetch", Upstream: "failure"}}},
			{Path: "/partial", Method: http.MethodGet, Pipeline: []config.Step{{ID: "fetch", Type: "fetch", Upstream: "failure"}}},
		},
	}
	app, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tooLarge := httptest.NewRecorder()
	app.ServeHTTP(tooLarge, httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader("12345")))
	if tooLarge.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413 for oversized body, got %d", tooLarge.Code)
	}
	upstreamFailure := httptest.NewRecorder()
	app.ServeHTTP(upstreamFailure, httptest.NewRequest(http.MethodGet, "/failure", nil))
	if upstreamFailure.Code != http.StatusBadGateway {
		t.Fatalf("expected 502 for upstream failure, got %d", upstreamFailure.Code)
	}
	if upstreamFailure.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("expected structured error response, got %q", upstreamFailure.Header().Get("Content-Type"))
	}
	partial := httptest.NewRecorder()
	app.ServeHTTP(partial, httptest.NewRequest(http.MethodGet, "/partial", nil))
	if partial.Code != http.StatusOK || !strings.Contains(partial.Body.String(), `"_errors"`) {
		t.Fatalf("expected usable partial response with _errors, got %d: %s", partial.Code, partial.Body.String())
	}
}

func TestEngineEndpointConcurrencyLimitReturnsRetryable429(t *testing.T) {
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		started <- struct{}{}
		select {
		case <-release:
			_, _ = w.Write([]byte(`{"ok":true}`))
		case <-req.Context().Done():
		}
	}))
	defer upstream.Close()
	app, err := New(&config.Config{
		Version:   "1",
		Upstreams: map[string]config.Upstream{"slow": {BaseURL: upstream.URL, Retries: intPointer(0)}},
		Endpoints: []config.Endpoint{{
			Path: "/slow", Method: http.MethodGet, MaxConcurrent: 1,
			Pipeline: []config.Step{{ID: "slow", Type: "fetch", Upstream: "slow"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		app.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/slow", nil))
		close(done)
	}()
	<-started
	second := httptest.NewRecorder()
	app.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/slow", nil))
	if second.Code != http.StatusTooManyRequests || second.Header().Get("Retry-After") != "1" || !strings.Contains(second.Body.String(), `"TOO_MANY_REQUESTS"`) {
		t.Fatalf("unexpected concurrency response: %d headers=%v body=%s", second.Code, second.Header(), second.Body.String())
	}
	close(release)
	<-done
	if first.Code != http.StatusOK {
		t.Fatalf("first request did not complete: %d %s", first.Code, first.Body.String())
	}
}

func TestEngineEnforcesPerStepTimeout(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	defer upstream.Close()
	failFast := true
	app, err := New(&config.Config{
		Version:   "1",
		Upstreams: map[string]config.Upstream{"slow": {BaseURL: upstream.URL, Retries: intPointer(0)}},
		Endpoints: []config.Endpoint{{
			Path: "/timeout", Method: http.MethodGet, FailFast: &failFast,
			Pipeline: []config.Step{{ID: "slow", Type: "fetch", Upstream: "slow", Timeout: config.Duration(15 * time.Millisecond)}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/timeout", nil))
	if response.Code != http.StatusGatewayTimeout || !strings.Contains(response.Body.String(), `"UPSTREAM_TIMEOUT"`) {
		t.Fatalf("expected structured step timeout, got %d: %s", response.Code, response.Body.String())
	}
}

func intPointer(value int) *int { return &value }

func TestEngineFallbackPolicies(t *testing.T) {
	failingUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer failingUpstream.Close()
	backupUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"source":"backup"}`))
	}))
	defer backupUpstream.Close()

	tests := []struct {
		name     string
		endpoint config.Endpoint
		caches   map[string]config.Cache
		wantBody string
	}{
		{
			name: "static",
			endpoint: config.Endpoint{Path: "/static", Method: http.MethodGet, Pipeline: []config.Step{{
				ID: "data", Type: "fetch", Upstream: "primary", OnError: "fallback",
				Fallback: &config.FallbackConfig{Static: `{"source":"static"}`},
			}}},
			wantBody: `{"source":"static"}`,
		},
		{
			name: "ignore",
			endpoint: config.Endpoint{Path: "/ignore", Method: http.MethodGet, Pipeline: []config.Step{{
				ID: "data", Type: "fetch", Upstream: "primary", OnError: "ignore",
			}}},
			wantBody: "null",
		},
		{
			name: "upstream",
			endpoint: config.Endpoint{Path: "/backup", Method: http.MethodGet, Pipeline: []config.Step{{
				ID: "data", Type: "fetch", Upstream: "primary", Path: "/data", OnError: "fallback",
				Fallback: &config.FallbackConfig{Upstream: "backup"},
			}}},
			wantBody: `{"source":"backup"}`,
		},
		{
			name:   "cache key",
			caches: map[string]config.Cache{"memory": {Type: "in-memory", MaxSize: 100, DefaultTTL: config.Duration(time.Minute)}},
			endpoint: config.Endpoint{Path: "/cached", Method: http.MethodGet, Pipeline: []config.Step{
				{ID: "value", Type: "transform", Template: `{"source":"cache"}`},
				{ID: "store", Type: "cache", Action: "set", Backend: "memory", Key: "user:{{ .query.id }}", Value: "value", DependsOn: []string{"value"}},
				{ID: "data", Type: "fetch", Upstream: "primary", DependsOn: []string{"store"}, OnError: "fallback", Fallback: &config.FallbackConfig{CacheKey: "user:{{ .query.id }}"}},
			}},
			wantBody: `{"source":"cache"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app, err := New(&config.Config{
				Version: "1",
				Caches:  test.caches,
				Upstreams: map[string]config.Upstream{
					"primary": {BaseURL: failingUpstream.URL, Retries: intPointer(0)},
					"backup":  {BaseURL: backupUpstream.URL, Retries: intPointer(0)},
				},
				Endpoints: []config.Endpoint{test.endpoint},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer app.Close()
			response := httptest.NewRecorder()
			app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.endpoint.Path+"?id=7", nil))
			if response.Code != http.StatusOK || strings.TrimSpace(response.Body.String()) != test.wantBody {
				t.Fatalf("unexpected fallback response %d: %s", response.Code, response.Body.String())
			}
		})
	}
}

func TestEngineSharesCircuitBreakerAcrossEndpoints(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer upstream.Close()
	failFast := true
	app, err := New(&config.Config{
		Version: "1",
		Upstreams: map[string]config.Upstream{
			"shared": {
				BaseURL: upstream.URL, Retries: intPointer(0),
				Breaker: &config.BreakerConfig{MaxRequests: 1, Interval: config.Duration(time.Minute), Timeout: config.Duration(time.Minute), FailureThreshold: 1},
			},
		},
		Endpoints: []config.Endpoint{
			{Path: "/one", Method: http.MethodGet, FailFast: &failFast, Pipeline: []config.Step{{ID: "call", Type: "fetch", Upstream: "shared"}}},
			{Path: "/two", Method: http.MethodGet, FailFast: &failFast, Pipeline: []config.Step{{ID: "call", Type: "fetch", Upstream: "shared"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	app.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/one", nil))
	second := httptest.NewRecorder()
	app.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/two", nil))
	if first.Code != http.StatusBadGateway || second.Code != http.StatusServiceUnavailable || !strings.Contains(second.Body.String(), `"CIRCUIT_OPEN"`) || calls.Load() != 1 {
		t.Fatalf("breaker was not shared: first=%d second=%d calls=%d body=%s", first.Code, second.Code, calls.Load(), second.Body.String())
	}
}

func TestEngineReturnsSuccessfulDataWithPartialErrors(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"name":"Ada"}`))
	}))
	defer good.Close()
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer bad.Close()
	app, err := New(&config.Config{
		Version: "1",
		Upstreams: map[string]config.Upstream{
			"good": {BaseURL: good.URL, Retries: intPointer(0)},
			"bad":  {BaseURL: bad.URL, Retries: intPointer(0)},
		},
		Endpoints: []config.Endpoint{{Path: "/partial-data", Method: http.MethodGet, Pipeline: []config.Step{
			{ID: "good", Type: "fetch", Upstream: "good"},
			{ID: "bad", Type: "fetch", Upstream: "bad"},
			{ID: "combined", Type: "transform", DependsOn: []string{"good", "bad"}, Template: `{"user": {{ .good | toJSON }}}`},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/partial-data", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("expected partial 200, got %d: %s", response.Code, response.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	user, ok := result["user"].(map[string]any)
	if !ok || user["name"] != "Ada" {
		t.Fatalf("successful upstream result was lost: %#v", result)
	}
	partialErrors, ok := result["_errors"].(map[string]any)
	if !ok || partialErrors["bad"] == nil {
		t.Fatalf("missing failing step details: %#v", result["_errors"])
	}
}

func TestUpstreamTimeoutDoesNotCancelIndependentStep(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"available":true}`))
	}))
	defer good.Close()
	slow := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	defer slow.Close()
	app, err := New(&config.Config{
		Version: "1",
		Upstreams: map[string]config.Upstream{
			"good": {BaseURL: good.URL, Retries: intPointer(0)},
			"slow": {BaseURL: slow.URL, Retries: intPointer(0)},
		},
		Endpoints: []config.Endpoint{{Path: "/resilient", Method: http.MethodGet, Pipeline: []config.Step{
			{ID: "slow", Type: "fetch", Upstream: "slow", Timeout: config.Duration(15 * time.Millisecond)},
			{ID: "good", Type: "fetch", Upstream: "good"},
			{ID: "combined", Type: "transform", DependsOn: []string{"slow", "good"}, Template: `{"result": {{ .good | toJSON }}}`},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/resilient", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"available":true`) || !strings.Contains(response.Body.String(), `"slow"`) {
		t.Fatalf("timeout cascaded or partial result was lost: %d %s", response.Code, response.Body.String())
	}
}
