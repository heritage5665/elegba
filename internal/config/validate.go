package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func (c *Config) Validate() error {
	if c.Version != "1" {
		return fmt.Errorf("unsupported configuration version %q; supported version is \"1\"", c.Version)
	}
	if len(c.Endpoints) == 0 {
		return fmt.Errorf("at least one endpoint is required")
	}
	if c.Server.ReadinessTimeout < 0 {
		return fmt.Errorf("server readinessTimeout must be non-negative")
	}
	if c.Server.Admin.Enabled && strings.TrimSpace(c.Server.Admin.Address) == "" {
		return fmt.Errorf("server.admin.address is required when the admin server is enabled")
	}
	if c.Tracing.Enabled {
		endpoint, err := url.ParseRequestURI(c.Tracing.Endpoint)
		if err != nil || endpoint.Host == "" || endpoint.Scheme != "http" && endpoint.Scheme != "https" {
			return fmt.Errorf("tracing.endpoint must be an absolute HTTP or HTTPS OTLP endpoint")
		}
	}
	for name, upstream := range c.Upstreams {
		if upstream.Transport != "" && upstream.Transport != "http" {
			return fmt.Errorf("upstream %q has unsupported transport %q", name, upstream.Transport)
		}
		parsed, err := url.ParseRequestURI(upstream.BaseURL)
		if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" {
			return fmt.Errorf("upstream %q has invalid absolute HTTP baseURL %q", name, upstream.BaseURL)
		}
		if upstream.Timeout < 0 || upstream.MaxResponseBytes <= 0 || upstream.MaxConcurrent < 0 {
			return fmt.Errorf("upstream %q timeout must be non-negative and maxResponseBytes and maxConcurrent must be positive", name)
		}
		if upstream.ConnectionPool.MaxIdleConns < 0 || upstream.ConnectionPool.MaxIdleConnsPerHost < 0 || upstream.ConnectionPool.IdleConnTimeout < 0 {
			return fmt.Errorf("upstream %q connectionPool values must be non-negative", name)
		}
		if upstream.Retries != nil && *upstream.Retries < 0 {
			return fmt.Errorf("upstream %q retries must be non-negative", name)
		}
		if upstream.Retries != nil && *upstream.Retries > 0 {
			if upstream.Retry.InitialInterval <= 0 || upstream.Retry.MaxInterval < upstream.Retry.InitialInterval || upstream.Retry.Multiplier < 1 || upstream.Retry.MaxElapsedTime <= 0 {
				return fmt.Errorf("upstream %q has invalid retry intervals, multiplier, or maxElapsedTime", name)
			}
		}
		if upstream.Breaker != nil && (upstream.Breaker.MaxRequests == 0 || upstream.Breaker.Interval <= 0 || upstream.Breaker.Timeout <= 0 || upstream.Breaker.FailureThreshold == 0) {
			return fmt.Errorf("upstream %q breaker limits and durations must be positive", name)
		}
		if upstream.RateLimit != "" {
			if err := ValidateRateLimit(upstream.RateLimit); err != nil {
				return fmt.Errorf("upstream %q has invalid rateLimit: %w", name, err)
			}
		}
		switch upstream.Auth.Type {
		case "", "bearer", "basic", "apikey", "client":
		default:
			return fmt.Errorf("upstream %q has unsupported auth type %q", name, upstream.Auth.Type)
		}
		if upstream.Auth.Type == "bearer" && upstream.Auth.Token == "" {
			return fmt.Errorf("upstream %q bearer auth requires a token", name)
		}
		if upstream.Auth.Type == "basic" && (upstream.Auth.User == "" || upstream.Auth.Pass == "") {
			return fmt.Errorf("upstream %q basic auth requires username and password", name)
		}
		if upstream.Auth.Type == "apikey" && upstream.Auth.Key == "" {
			return fmt.Errorf("upstream %q apikey auth requires a key", name)
		}
		if upstream.Auth.Type == "client" && upstream.Auth.Header == "" {
			return fmt.Errorf("upstream %q client auth requires an incoming header", name)
		}
		if upstream.Auth.Type == "client" && upstream.Auth.Forward == "" {
			return fmt.Errorf("upstream %q client auth requires an outgoing forward header", name)
		}
	}
	for _, endpoint := range c.Endpoints {
		for _, step := range endpoint.Pipeline {
			if step.Type != "fetch" || step.Cache == nil {
				continue
			}
			upstream, ok := c.Upstreams[step.Upstream]
			if !ok || upstream.Auth.Type != "client" {
				continue
			}
			cacheKey := step.Cache.Key
			if !cacheKeyHashesClientToken(cacheKey, upstream.Auth.Header) {
				return fmt.Errorf("step %q cache key must include a token hash", step.ID)
			}
		}
	}
	for name, cache := range c.Caches {
		switch cache.Type {
		case "in-memory":
			if cache.MaxSize <= 0 || cache.DefaultTTL <= 0 {
				return fmt.Errorf("cache %q maxSize and defaultTTL must be positive", name)
			}
		case "redis":
			if strings.TrimSpace(cache.Address) == "" || cache.PoolSize <= 0 || cache.DB < 0 || cache.MaxRetries < 0 || cache.DefaultTTL <= 0 || cache.DialTimeout <= 0 || cache.ReadTimeout <= 0 || cache.WriteTimeout <= 0 {
				return fmt.Errorf("redis cache %q requires address and positive pool, TTL, and timeout settings", name)
			}
			if cache.TLS != nil && (cache.TLS.CertFile == "") != (cache.TLS.KeyFile == "") {
				return fmt.Errorf("redis cache %q tls certFile and keyFile must be specified together", name)
			}
		default:
			return fmt.Errorf("cache %q has unsupported type %q", name, cache.Type)
		}
	}
	seenEndpoints := make(map[string]bool)
	for _, endpoint := range c.Endpoints {
		if endpoint.Path == "" || !strings.HasPrefix(endpoint.Path, "/") {
			return fmt.Errorf("endpoint path %q must start with /", endpoint.Path)
		}
		if endpoint.Method == "" {
			return fmt.Errorf("endpoint %q has no method", endpoint.Path)
		}
		if endpoint.Timeout < 0 || endpoint.MaxConcurrent < 0 {
			return fmt.Errorf("endpoint %s timeout and maxConcurrent must be non-negative", endpoint.Path)
		}
		for _, key := range endpoint.CacheInvalidate {
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("endpoint %s cacheInvalidate keys must be non-empty", endpoint.Path)
			}
		}
		key := strings.ToUpper(endpoint.Method) + " " + endpoint.Path
		if seenEndpoints[key] {
			return fmt.Errorf("duplicate endpoint %s", key)
		}
		seenEndpoints[key] = true
		if len(endpoint.Pipeline) == 0 {
			return fmt.Errorf("endpoint %s has an empty pipeline", key)
		}
		stepIDs := make(map[string]bool)
		for _, step := range endpoint.Pipeline {
			if step.ID == "" || stepIDs[step.ID] {
				return fmt.Errorf("endpoint %s has an empty or duplicate step id %q", key, step.ID)
			}
			if step.Timeout < 0 {
				return fmt.Errorf("step %q timeout must be non-negative", step.ID)
			}
			if step.OnError != "" && step.OnError != "fail" && step.OnError != "ignore" && step.OnError != "fallback" {
				return fmt.Errorf("step %q has unsupported onError policy %q", step.ID, step.OnError)
			}
			if step.Fallback != nil && step.OnError != "fallback" {
				return fmt.Errorf("step %q fallback requires onError: fallback", step.ID)
			}
			if step.OnError == "fallback" {
				if step.Fallback == nil {
					return fmt.Errorf("step %q onError: fallback requires a fallback configuration", step.ID)
				}
				fallbacks := 0
				if step.Fallback.Static != "" {
					fallbacks++
					if !json.Valid([]byte(step.Fallback.Static)) {
						return fmt.Errorf("step %q static fallback must be valid JSON", step.ID)
					}
				}
				if step.Fallback.Upstream != "" {
					fallbacks++
					if _, ok := c.Upstreams[step.Fallback.Upstream]; !ok {
						return fmt.Errorf("step %q references undefined fallback upstream %q", step.ID, step.Fallback.Upstream)
					}
				}
				if step.Fallback.CacheKey != "" {
					fallbacks++
				}
				if fallbacks != 1 {
					return fmt.Errorf("step %q fallback must specify exactly one of static, upstream, or cacheKey", step.ID)
				}
			}
			stepIDs[step.ID] = true
			switch step.Type {
			case "fetch":
				if _, ok := c.Upstreams[step.Upstream]; !ok {
					return fmt.Errorf("step %q references undefined upstream %q", step.ID, step.Upstream)
				}
				if step.Cache != nil {
					if _, ok := c.Caches[step.Cache.Backend]; !ok {
						return fmt.Errorf("step %q references undefined cache %q", step.ID, step.Cache.Backend)
					}
					if step.Cache.Key == "" || step.Cache.TTL < 0 {
						return fmt.Errorf("step %q cache requires a key and non-negative ttl", step.ID)
					}
				}
			case "transform":
				if step.Template == "" {
					return fmt.Errorf("transform step %q requires a template", step.ID)
				}
			case "cache":
				if _, ok := c.Caches[step.Backend]; !ok {
					return fmt.Errorf("step %q references undefined cache %q", step.ID, step.Backend)
				}
				if step.Key == "" || step.TTL < 0 {
					return fmt.Errorf("step %q cache requires a key and non-negative ttl", step.ID)
				}
				switch step.Action {
				case "get", "delete":
				case "set":
					if step.Value == "" {
						return fmt.Errorf("cache set step %q requires a value step id", step.ID)
					}
				default:
					return fmt.Errorf("cache step %q action must be get or set", step.ID)
				}
			default:
				return fmt.Errorf("step %q has unsupported type %q", step.ID, step.Type)
			}
		}
		for _, step := range endpoint.Pipeline {
			for _, dependency := range step.DependsOn {
				if !stepIDs[dependency] {
					return fmt.Errorf("step %q depends on unknown step %q", step.ID, dependency)
				}
			}
			if step.Type == "cache" && step.Action == "set" {
				if !stepIDs[step.Value] {
					return fmt.Errorf("cache step %q references unknown value step %q", step.ID, step.Value)
				}
				dependsOnValue := false
				for _, dependency := range step.DependsOn {
					if dependency == step.Value {
						dependsOnValue = true
						break
					}
				}
				if !dependsOnValue {
					return fmt.Errorf("cache step %q must depend on value step %q", step.ID, step.Value)
				}
			}
		}
	}
	return nil
}

func cacheKeyHashesClientToken(cacheKey, header string) bool {
	input := ".header." + header
	for _, action := range strings.Split(cacheKey, "{{") {
		if strings.Contains(action, "}}") {
			action = strings.SplitN(action, "}}", 2)[0]
		}
		if strings.Contains(action, input) &&
			(strings.Contains(action, "| sha256") || strings.Contains(action, "sha256 "+input)) {
			return true
		}
	}
	return false
}

func ValidateRateLimit(value string) error {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return fmt.Errorf("expected N/s, N/m, or N/h")
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil || count <= 0 {
		return fmt.Errorf("request count must be a positive integer")
	}
	switch parts[1] {
	case "s", "m", "h":
		return nil
	default:
		return fmt.Errorf("duration must be s, m, or h")
	}
}
