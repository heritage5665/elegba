package engine

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	cachebackend "github.com/elegba-dev/elegba/internal/cache"
	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/observability"
	"github.com/elegba-dev/elegba/internal/pipeline"
	"github.com/elegba-dev/elegba/internal/step"
	httptransport "github.com/elegba-dev/elegba/internal/transport"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

type Engine struct {
	config      *config.Config
	router      *Router
	caches      map[string]cachebackend.Cache
	metrics     *observability.Metrics
	tracer      trace.Tracer
	lifecycleMu sync.Mutex
	active      sync.WaitGroup
	closed      bool
}

func New(cfg *config.Config) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	metrics := observability.NewMetrics()
	tracer := otel.Tracer("github.com/elegba-dev/elegba")
	registry := step.NewRegistry()
	cacheRegistry := cachebackend.NewDefaultRegistry()
	caches := make(map[string]cachebackend.Cache, len(cfg.Caches))
	for name, cacheConfig := range cfg.Caches {
		backend, err := cacheRegistry.Create(name, cacheConfig)
		if err != nil {
			for _, initialized := range caches {
				initialized.Close()
			}
			return nil, err
		}
		caches[name] = backend
	}
	constructionComplete := false
	defer func() {
		if !constructionComplete {
			for _, backend := range caches {
				backend.Close()
			}
		}
	}()
	upstreamTransports := make(map[string]httptransport.Transport, len(cfg.Upstreams))
	for name, upstream := range cfg.Upstreams {
		retryCount := 0
		if upstream.Retries != nil {
			retryCount = *upstream.Retries
		}
		retry := httptransport.RetryOptions{
			MaxRetries:      retryCount,
			InitialInterval: time.Duration(upstream.Retry.InitialInterval),
			MaxInterval:     time.Duration(upstream.Retry.MaxInterval),
			Multiplier:      upstream.Retry.Multiplier,
			MaxElapsedTime:  time.Duration(upstream.Retry.MaxElapsedTime),
			Jitter:          upstream.Retry.Jitter != nil && *upstream.Retry.Jitter,
		}
		var breaker *httptransport.BreakerOptions
		if upstream.Breaker != nil {
			breaker = &httptransport.BreakerOptions{
				MaxRequests:      upstream.Breaker.MaxRequests,
				Interval:         time.Duration(upstream.Breaker.Interval),
				Timeout:          time.Duration(upstream.Breaker.Timeout),
				FailureThreshold: upstream.Breaker.FailureThreshold,
			}
		}
		pool := upstream.ConnectionPool
		base := httptransport.NewHTTPTransport(time.Duration(upstream.Timeout), httptransport.HTTPTransportOptions{
			MaxIdleConns:        pool.MaxIdleConns,
			MaxIdleConnsPerHost: pool.MaxIdleConnsPerHost,
			IdleConnTimeout:     time.Duration(pool.IdleConnTimeout),
		})
		resilient, err := httptransport.NewResilientTransport(name, base, httptransport.ResilienceOptions{
			Retries: retry, Breaker: breaker, RateLimit: upstream.RateLimit, MaxConcurrent: upstream.MaxConcurrent, Metrics: metrics,
		})
		if err != nil {
			return nil, err
		}
		upstreamTransports[name] = resilient
	}
	if err := registry.Register("fetch", func(stepConfig config.Step, upstream config.Upstream, client httptransport.Transport) (pipeline.Step, error) {
		var backend cachebackend.Cache
		var ttl time.Duration
		if stepConfig.Cache != nil {
			backend = caches[stepConfig.Cache.Backend]
			ttl = time.Duration(cfg.Caches[stepConfig.Cache.Backend].DefaultTTL)
		}
		return step.NewFetchStepWithTransport(stepConfig, upstream, client, backend, ttl)
	}); err != nil {
		return nil, err
	}
	if err := registry.Register("transform", func(stepConfig config.Step, _ config.Upstream, _ httptransport.Transport) (pipeline.Step, error) {
		return step.NewTransformStep(stepConfig)
	}); err != nil {
		return nil, err
	}
	if err := registry.Register("cache", func(stepConfig config.Step, _ config.Upstream, _ httptransport.Transport) (pipeline.Step, error) {
		cacheConfig := cfg.Caches[stepConfig.Backend]
		return step.NewCacheStep(stepConfig, caches[stepConfig.Backend], time.Duration(cacheConfig.DefaultTTL))
	}); err != nil {
		return nil, err
	}

	engine := &Engine{config: cfg, router: NewRouter(), caches: caches, metrics: metrics, tracer: tracer}
	for _, endpoint := range cfg.Endpoints {
		compiled := make([]pipeline.Step, 0, len(endpoint.Pipeline))
		for _, stepConfig := range endpoint.Pipeline {
			effectiveStepConfig := stepConfig
			if effectiveStepConfig.Timeout == 0 {
				effectiveStepConfig.Timeout = endpoint.Timeout
			}
			pipelineStep, err := registry.Create(stepConfig, cfg.Upstreams[stepConfig.Upstream], upstreamTransports[stepConfig.Upstream])
			if err != nil {
				return nil, fmt.Errorf("compile endpoint %s step %s: %w", endpoint.Path, stepConfig.ID, err)
			}
			if instrumented, ok := pipelineStep.(interface{ SetMetrics(*observability.Metrics) }); ok {
				instrumented.SetMetrics(metrics)
			}
			var fallbackStep pipeline.Step
			if stepConfig.Fallback != nil && stepConfig.Fallback.Upstream != "" {
				fallbackConfig := stepConfig
				fallbackConfig.ID += "-fallback"
				fallbackConfig.Upstream = stepConfig.Fallback.Upstream
				fallbackConfig.Cache = nil
				fallbackConfig.OnError = "fail"
				fallbackConfig.Fallback = nil
				fallbackStep, err = step.NewFetchStepWithTransport(
					fallbackConfig,
					cfg.Upstreams[stepConfig.Fallback.Upstream],
					upstreamTransports[stepConfig.Fallback.Upstream],
					nil,
					0,
				)
				if err != nil {
					return nil, fmt.Errorf("compile fallback for endpoint %s step %s: %w", endpoint.Path, stepConfig.ID, err)
				}
				if instrumented, ok := fallbackStep.(interface{ SetMetrics(*observability.Metrics) }); ok {
					instrumented.SetMetrics(metrics)
				}
			}
			compiled = append(compiled, newConfiguredStep(effectiveStepConfig, pipelineStep, fallbackStep, caches, endpoint.Path, metrics, tracer))
		}
		if _, err := pipeline.TopologicalSort(compiled); err != nil {
			return nil, fmt.Errorf("compile endpoint %s: %w", endpoint.Path, err)
		}
		endpointCopy := endpoint
		stepsCopy := compiled
		endpointLimit := make(chan struct{}, endpointCopy.MaxConcurrent)
		engine.router.Handle(endpoint.Method, endpoint.Path, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			select {
			case endpointLimit <- struct{}{}:
				defer func() { <-endpointLimit }()
				metrics.AddInflight(endpointCopy.Path, 1)
				defer metrics.AddInflight(endpointCopy.Path, -1)
			default:
				w.Header().Set("Retry-After", "1")
				writeAPIError(w, http.StatusTooManyRequests, "TOO_MANY_REQUESTS", "endpoint concurrency limit reached", requestIDFrom(req))
				return
			}
			engine.handleEndpoint(w, req, endpointCopy, stepsCopy)
		}))
	}
	constructionComplete = true
	return engine, nil
}

func (e *Engine) Close() {
	e.Drain(context.Background())
}

func (e *Engine) Drain(ctx context.Context) {
	e.lifecycleMu.Lock()
	if !e.closed {
		e.closed = true
	}
	e.lifecycleMu.Unlock()

	done := make(chan struct{})
	go func() {
		e.active.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		slog.Warn("engine drain timed out; closing cached resources", "error", ctx.Err())
	}
	for _, backend := range e.caches {
		backend.Close()
	}
}

func (e *Engine) beginRequest() bool {
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	if e.closed {
		return false
	}
	e.active.Add(1)
	return true
}

func (e *Engine) endRequest() {
	e.active.Done()
}

func (e *Engine) MetricsHandler() http.Handler {
	return e.metrics.Handler()
}

func (e *Engine) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if !e.beginRequest() {
		writeAPIError(w, http.StatusServiceUnavailable, "ENGINE_CLOSED", "the active engine is being replaced", "")
		return
	}
	defer e.endRequest()
	endpoint := e.router.EndpointLabel(req.Method, req.URL.Path)
	if req.URL.Path == "/healthz" || req.URL.Path == "/readyz" {
		endpoint = req.URL.Path
	}
	response := &statusRecorder{ResponseWriter: w}
	started := time.Now()
	defer func() { e.metrics.ObserveRequest(endpoint, req.Method, response.status, time.Since(started)) }()
	requestID, err := newRequestID()
	if err != nil {
		writeAPIError(response, http.StatusInternalServerError, "REQUEST_ID_FAILURE", "could not create request id", "")
		return
	}
	response.Header().Set("X-Request-ID", requestID)
	parentContext := otel.GetTextMapPropagator().Extract(req.Context(), propagation.HeaderCarrier(req.Header))
	ctx := observability.ContextWithRequestID(parentContext, requestID)
	ctx = context.WithValue(ctx, requestIDKey{}, requestID)
	ctx, span := e.tracer.Start(ctx, "elegba.request", trace.WithAttributes(
		attribute.String("http.request.method", req.Method),
		attribute.String("http.route", endpoint),
		attribute.String("request.id", requestID),
	))
	defer func() {
		if response.status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, "request failed")
		}
		span.End()
	}()
	req = req.WithContext(ctx)
	if req.URL.Path == "/healthz" {
		e.HealthHandler(response, req)
		return
	}
	if req.URL.Path == "/readyz" {
		e.ReadyHandler(response, req)
		return
	}
	slog.InfoContext(ctx, "request", "request_id", requestID, "method", req.Method, "path", req.URL.Path)
	e.router.ServeHTTP(response, req)
}

func (e *Engine) HealthHandler(w http.ResponseWriter, req *http.Request) {
	e.serveHealth(w, req, false)
}

func (e *Engine) ReadyHandler(w http.ResponseWriter, req *http.Request) {
	e.serveHealth(w, req, true)
}

func (e *Engine) serveHealth(w http.ResponseWriter, req *http.Request, readiness bool) {
	requestID := requestIDFrom(req)
	if requestID == "" {
		var err error
		requestID, err = newRequestID()
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, "REQUEST_ID_FAILURE", "could not create request id", "")
			return
		}
		w.Header().Set("X-Request-ID", requestID)
	}
	if req.Method != http.MethodGet && req.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeAPIError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed", requestID)
		return
	}
	if readiness {
		if err := e.Readiness(req.Context()); err != nil {
			writeAPIError(w, http.StatusServiceUnavailable, "NOT_READY", "one or more upstreams are unavailable", requestID)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok\n")
}

func (e *Engine) Readiness(ctx context.Context) error {
	timeout := time.Duration(e.config.Server.ReadinessTimeout)
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	group, ctx := errgroup.WithContext(ctx)
	for name, upstream := range e.config.Upstreams {
		name, baseURL := name, upstream.BaseURL
		group.Go(func() error {
			client := &http.Client{Timeout: timeout}
			request, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL, nil)
			if err != nil {
				return fmt.Errorf("readiness request for upstream %q: %w", name, err)
			}
			response, err := client.Do(request)
			if response != nil {
				_ = response.Body.Close()
			}
			if err != nil {
				return fmt.Errorf("upstream %q is unreachable: %w", name, err)
			}
			if response.StatusCode >= http.StatusInternalServerError {
				return fmt.Errorf("upstream %q returned HTTP %d to readiness probe", name, response.StatusCode)
			}
			return nil
		})
	}
	return group.Wait()
}

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (w *statusRecorder) WriteHeader(status int) {
	if w.written {
		return
	}
	w.status = status
	w.written = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusRecorder) Write(data []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *statusRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

type requestIDKey struct{}

func newRequestID() (string, error) {
	var id [16]byte
	timestamp := uint64(time.Now().UnixMilli())
	for index := 5; index >= 0; index-- {
		id[index] = byte(timestamp)
		timestamp >>= 8
	}
	if _, err := rand.Read(id[6:]); err != nil {
		return "", err
	}
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"
	var encoded [26]byte
	for index := range encoded {
		var value byte
		for bit := 0; bit < 5; bit++ {
			sourceBit := index*5 + bit - 2
			value <<= 1
			if sourceBit >= 0 && sourceBit < 128 {
				value |= (id[sourceBit/8] >> (7 - sourceBit%8)) & 1
			}
		}
		encoded[index] = alphabet[value]
	}
	return string(encoded[:]), nil
}

func (e *Engine) handleEndpoint(w http.ResponseWriter, req *http.Request, endpoint config.Endpoint, steps []pipeline.Step) {
	ctx := req.Context()
	if endpoint.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(endpoint.Timeout))
		defer cancel()
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, req.Body, e.config.Server.MaxBodyBytes))
	if err != nil {
		writeAPIError(w, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", "request body is too large or unreadable", requestIDFrom(req))
		return
	}
	var decodedBody any
	if len(body) > 0 {
		decoder := json.NewDecoder(strings.NewReader(string(body)))
		decoder.UseNumber()
		if err := decoder.Decode(&decodedBody); err != nil {
			decodedBody = string(body)
		}
	}
	query := make(map[string]any, len(req.URL.Query()))
	for key, values := range req.URL.Query() {
		if len(values) == 1 {
			query[key] = values[0]
		} else {
			query[key] = values
		}
	}
	headers := make(map[string]any, len(req.Header))
	for key, values := range req.Header {
		if len(values) == 1 {
			headers[http.CanonicalHeaderKey(key)] = values[0]
		} else {
			headers[http.CanonicalHeaderKey(key)] = values
		}
	}
	input := map[string]any{
		"path":      stringParams(pathParams(req.Context())),
		"query":     query,
		"headers":   headers,
		"header":    headers,
		"body":      decodedBody,
		"requestID": ctx.Value(requestIDKey{}),
	}
	if clientAuthMissing(e.config, endpoint, input) {
		writeAPIError(w, http.StatusUnauthorized, "CLIENT_AUTH_MISSING", "client authentication header is missing", requestIDFrom(req))
		return
	}
	for _, keyTemplate := range endpoint.CacheInvalidate {
		key, err := step.RenderTemplate(keyTemplate, input)
		if err != nil {
			slog.WarnContext(ctx, "cache invalidation key could not be rendered", "error", err)
			continue
		}
		for backendName, backend := range e.caches {
			if err := backend.Delete(ctx, key); err != nil {
				slog.WarnContext(ctx, "cache invalidation failed; continuing", "error", err, "backend", backendName)
			}
		}
	}
	failFast := endpoint.FailFast != nil && *endpoint.FailFast
	results, stepErrors, err := pipeline.ExecuteWithErrors(ctx, steps, input, failFast)
	if err != nil {
		status, code, message := pipelineErrorStatus(err)
		slog.ErrorContext(ctx, "pipeline failed", "error_code", code, "error_message", message, "request_id", ctx.Value(requestIDKey{}))
		writeAPIError(w, status, code, message, requestIDFrom(req))
		return
	}
	result, exists := results[steps[len(steps)-1].ID()]
	if len(stepErrors) > 0 {
		result = addPartialErrors(result, results, stepErrors)
		exists = true
	}
	if !exists {
		writeAPIError(w, http.StatusInternalServerError, "MISSING_PIPELINE_RESULT", "pipeline produced no result", requestIDFrom(req))
		return
	}
	var response []byte
	switch value := result.(type) {
	case string:
		response = []byte(value)
		if json.Valid(response) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		} else {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
	default:
		response, err = json.Marshal(result)
		if err != nil {
			http.Error(w, "could not encode pipeline result", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(response)
}

func clientAuthMissing(cfg *config.Config, endpoint config.Endpoint, input map[string]any) bool {
	headers, ok := input["headers"].(map[string]any)
	if !ok {
		return true
	}
	for _, stepConfig := range endpoint.Pipeline {
		if stepConfig.Type != "fetch" {
			continue
		}
		upstream, ok := cfg.Upstreams[stepConfig.Upstream]
		if !ok || upstream.Auth.Type != "client" {
			continue
		}
		header := upstream.Auth.Header
		if header == "" {
			header = "Authorization"
		}
		value, ok := headers[http.CanonicalHeaderKey(header)]
		if !ok || strings.TrimSpace(fmt.Sprint(value)) == "" {
			return true
		}
	}
	return false
}

func stringParams(params map[string]string) map[string]any {
	result := make(map[string]any, len(params))
	for key, value := range params {
		result[key] = value
	}
	return result
}
