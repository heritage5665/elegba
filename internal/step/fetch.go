// File path: elegba/internal/step/fetch.go

package step

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"text/template"
	"time"

	cachebackend "github.com/elegba-dev/elegba/internal/cache"
	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/observability"
	"github.com/elegba-dev/elegba/internal/pipeline"
	httptransport "github.com/elegba-dev/elegba/internal/transport"
	"golang.org/x/sync/singleflight"
)

type FetchStep struct {
	config         config.Step
	upstream       config.Upstream
	client         httptransport.Transport
	pathTemplate   *template.Template
	bodyTemplate   *template.Template
	headerTemplate map[string]*template.Template
	cache          cachebackend.Cache
	cacheKey       *template.Template
	cacheTTL       time.Duration
	metrics        *observability.Metrics
	refreshes      singleflight.Group
	closeOnce      sync.Once
}

func NewFetchStep(stepConfig config.Step, upstream config.Upstream, cache cachebackend.Cache, defaultCacheTTL time.Duration) (*FetchStep, error) {
	if upstream.Timeout <= 0 {
		upstream.Timeout = config.Duration(config.DefaultTimeout)
	}
	pool := upstream.ConnectionPool
	return NewFetchStepWithTransport(stepConfig, upstream, httptransport.NewHTTPTransport(time.Duration(upstream.Timeout), httptransport.HTTPTransportOptions{
		MaxIdleConns:        pool.MaxIdleConns,
		MaxIdleConnsPerHost: pool.MaxIdleConnsPerHost,
		IdleConnTimeout:     time.Duration(pool.IdleConnTimeout),
	}), cache, defaultCacheTTL)
}

func NewFetchStepWithTransport(stepConfig config.Step, upstream config.Upstream, client httptransport.Transport, cache cachebackend.Cache, defaultCacheTTL time.Duration) (*FetchStep, error) {
	if upstream.Timeout <= 0 {
		upstream.Timeout = config.Duration(config.DefaultTimeout)
	}
	pathTemplate, err := parseTemplate("fetch-path", stepConfig.Path)
	if err != nil {
		return nil, err
	}
	bodyTemplate, err := parseTemplate("fetch-body", stepConfig.Body)
	if err != nil {
		return nil, err
	}
	step := &FetchStep{
		config:         stepConfig,
		upstream:       upstream,
		client:         client,
		pathTemplate:   pathTemplate,
		bodyTemplate:   bodyTemplate,
		headerTemplate: make(map[string]*template.Template, len(stepConfig.Headers)+len(upstream.Headers)),
	}
	if stepConfig.Cache != nil {
		step.cache = cache
		step.cacheTTL = time.Duration(stepConfig.Cache.TTL)
		if step.cacheTTL == 0 {
			step.cacheTTL = defaultCacheTTL
		}
		step.cacheKey, err = parseTemplate("cache-key", stepConfig.Cache.Key)
		if err != nil {
			return nil, err
		}
	}
	for name, value := range upstream.Headers {
		step.headerTemplate[name], err = parseTemplate("upstream-header-"+name, value)
		if err != nil {
			return nil, err
		}
	}
	for name, value := range stepConfig.Headers {
		step.headerTemplate[name], err = parseTemplate("step-header-"+name, value)
		if err != nil {
			return nil, err
		}
	}
	return step, nil
}

func (f *FetchStep) ID() string                                { return f.config.ID }
func (f *FetchStep) DependsOn() []string                       { return f.config.DependsOn }
func (f *FetchStep) SetMetrics(metrics *observability.Metrics) { f.metrics = metrics }

func (f *FetchStep) Execute(ctx context.Context, data map[string]any) (any, error) {
	return f.execute(ctx, data, false)
}

func (f *FetchStep) execute(ctx context.Context, data map[string]any, skipCache bool) (any, error) {
	cacheKey, err := renderTemplate(f.cacheKey, data)
	if err != nil {
		return nil, err
	}
	if f.cache != nil && !skipCache {
		value, ok, cacheErr := f.cache.Get(ctx, cacheKey)
		if cacheErr != nil {
			slog.WarnContext(ctx, "cache read failed; fetching upstream", "error", cacheErr, "backend", f.config.Cache.Backend, "request_id", data["requestID"])
			ok = false
		}
		if f.metrics != nil {
			f.metrics.ObserveCache(f.config.Cache.Backend, ok)
		}
		if ok {
			if stale, isStale := value.(cachebackend.StaleValue); isStale {
				if time.Now().Before(stale.FreshUntil) {
					return stale.Value, nil
				}
				if f.config.Cache.StaleWhileRevalidate {
					f.refreshStale(cacheKey, data)
					return stale.Value, nil
				}
			} else {
				return value, nil
			}
		}
	}
	path, err := renderTemplate(f.pathTemplate, data)
	if err != nil {
		return nil, err
	}
	baseURL, err := url.Parse(f.upstream.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("upstream %q base URL: %w", f.config.Upstream, err)
	}
	target := baseURL.ResolveReference(&url.URL{Path: path})
	method := f.config.Method
	if method == "" {
		method = http.MethodGet
	}
	body, err := renderTemplate(f.bodyTemplate, data)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, target.String(), strings.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("upstream %q request: %w", f.config.Upstream, err)
	}
	for name, headerTemplate := range f.headerTemplate {
		rendered, renderErr := renderTemplate(headerTemplate, data)
		if renderErr != nil {
			return nil, renderErr
		}
		req.Header.Set(name, rendered)
	}
	if requestID, ok := data["requestID"].(string); ok && requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	if err := applyAuth(req, f.upstream.Auth, data); err != nil {
		return nil, err
	}
	response, err := f.client.Do(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("upstream %q request: %w", f.config.Upstream, err)
	}
	defer response.Body.Close()
	maxBytes := f.upstream.MaxResponseBytes
	if maxBytes <= 0 {
		maxBytes = config.DefaultMaxResponseBytes
	}
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("upstream %q response body: %w", f.config.Upstream, err)
	}
	if int64(len(responseBody)) > maxBytes {
		return nil, fmt.Errorf("upstream response exceeds %d byte limit", maxBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: upstream returned %s", httptransport.ErrUpstreamFailure, response.Status)
	}
	var decoded any
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err == nil {
		f.storeCache(ctx, cacheKey, decoded, data)
		return decoded, nil
	}
	decoded = string(responseBody)
	f.storeCache(ctx, cacheKey, decoded, data)
	return decoded, nil
}

func (f *FetchStep) storeCache(ctx context.Context, key string, value any, data map[string]any) {
	if f.cache == nil {
		return
	}
	cacheValue := value
	ttl := f.cacheTTL
	if f.config.Cache.StaleWhileRevalidate {
		cacheValue = cachebackend.StaleValue{Value: value, FreshUntil: time.Now().Add(f.cacheTTL)}
		if ttl <= time.Duration(1<<62) {
			ttl *= 2
		}
	}
	if err := f.cache.Set(ctx, key, cacheValue, ttl); err != nil {
		slog.WarnContext(ctx, "cache write failed; continuing without cache", "error", err, "backend", f.config.Cache.Backend, "request_id", data["requestID"])
	}
}

func (f *FetchStep) refreshStale(key string, data map[string]any) {
	f.refreshes.DoChan(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(f.upstream.Timeout))
		defer cancel()
		if _, err := f.execute(ctx, data, true); err != nil {
			slog.WarnContext(ctx, "stale cache refresh failed", "error", err, "upstream", f.config.Upstream, "request_id", data["requestID"])
			return nil, err
		}
		return nil, nil
	})
}

var _ pipeline.Step = (*FetchStep)(nil)

var ErrClientAuthMissing = errors.New("client authentication header is missing")

func applyAuth(req *http.Request, auth config.AuthConfig, data map[string]any) error {
	switch auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+auth.Token)
	case "basic":
		req.SetBasicAuth(auth.User, auth.Pass)
	case "apikey":
		name := auth.Header
		if name == "" {
			name = "X-API-Key"
		}
		req.Header.Set(name, auth.Key)
	case "client":
		incomingHeader := auth.Header
		if incomingHeader == "" {
			incomingHeader = "Authorization"
		}
		forwardHeader := auth.Forward
		if forwardHeader == "" {
			forwardHeader = incomingHeader
		}
		scheme := auth.Scheme
		if scheme == "" {
			scheme = "Bearer"
		}
		value, ok := requestHeader(data, incomingHeader)
		if !ok || value == "" {
			return fmt.Errorf("%w: %s", ErrClientAuthMissing, incomingHeader)
		}
		token := strings.TrimSpace(value)
		if strings.HasPrefix(strings.ToLower(token), strings.ToLower(scheme)+" ") {
			token = strings.TrimSpace(token[len(scheme):])
		}
		if scheme != "" {
			token = scheme + " " + token
		}
		req.Header.Set(forwardHeader, token)
	}
	return nil
}

func requestHeader(data map[string]any, name string) (string, bool) {
	headers, ok := data["headers"].(map[string]any)
	if !ok {
		return "", false
	}
	value, ok := headers[http.CanonicalHeaderKey(name)]
	if !ok {
		return "", false
	}
	if value == nil {
		return "", false
	}
	if stringValue, ok := value.(string); ok {
		return stringValue, true
	}
	return fmt.Sprint(value), true
}

func parseTemplate(name, source string) (*template.Template, error) {
	if source == "" {
		return nil, nil
	}
	return template.New(name).Funcs(templateFunctions()).Option("missingkey=error").Parse(source)
}

func renderTemplate(tmpl *template.Template, data map[string]any) (string, error) {
	if tmpl == nil {
		return "", nil
	}
	var result bytes.Buffer
	if err := tmpl.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
}

func RenderTemplate(source string, data map[string]any) (string, error) {
	tmpl, err := parseTemplate("runtime-template", source)
	if err != nil {
		return "", err
	}
	return renderTemplate(tmpl, data)
}
