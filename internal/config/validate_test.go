package config

import (
	"testing"
	"time"
)

func validConfig() *Config {
	return &Config{
		Version: "1",
		Upstreams: map[string]Upstream{
			"users": {BaseURL: "https://example.test", Timeout: Duration(time.Second), MaxResponseBytes: 1024},
		},
		Caches: map[string]Cache{
			"memory": {Type: "in-memory", MaxSize: 100, DefaultTTL: Duration(time.Minute)},
		},
		Endpoints: []Endpoint{{Path: "/users/{id}", Method: "GET", Pipeline: []Step{{ID: "user", Type: "fetch", Upstream: "users"}}}},
	}
}

func TestValidateRejectsInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "version", change: func(c *Config) { c.Version = "2" }},
		{name: "no endpoints", change: func(c *Config) { c.Endpoints = nil }},
		{name: "invalid upstream URL", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.BaseURL = "/relative"
			c.Upstreams["users"] = upstream
		}},
		{name: "upstream timeout", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.Timeout = -1
			c.Upstreams["users"] = upstream
		}},
		{name: "negative retries", change: func(c *Config) {
			retries := -1
			upstream := c.Upstreams["users"]
			upstream.Retries = &retries
			c.Upstreams["users"] = upstream
		}},
		{name: "invalid retry policy", change: func(c *Config) {
			retries := 1
			upstream := c.Upstreams["users"]
			upstream.Retries = &retries
			upstream.Retry.InitialInterval = Duration(time.Second)
			upstream.Retry.MaxInterval = Duration(time.Millisecond)
			c.Upstreams["users"] = upstream
		}},
		{name: "invalid breaker", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.Breaker = &BreakerConfig{}
			c.Upstreams["users"] = upstream
		}},
		{name: "invalid rate limit", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.RateLimit = "10/day"
			c.Upstreams["users"] = upstream
		}},
		{name: "unsupported auth", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.Auth.Type = "oauth"
			c.Upstreams["users"] = upstream
		}},
		{name: "missing bearer token", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.Auth.Type = "bearer"
			c.Upstreams["users"] = upstream
		}},
		{name: "missing basic credentials", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.Auth.Type = "basic"
			c.Upstreams["users"] = upstream
		}},
		{name: "missing api key", change: func(c *Config) {
			upstream := c.Upstreams["users"]
			upstream.Auth.Type = "apikey"
			c.Upstreams["users"] = upstream
		}},
		{name: "unsupported cache", change: func(c *Config) { cache := c.Caches["memory"]; cache.Type = "memcached"; c.Caches["memory"] = cache }},
		{name: "invalid cache defaults", change: func(c *Config) { cache := c.Caches["memory"]; cache.MaxSize = 0; c.Caches["memory"] = cache }},
		{name: "relative endpoint", change: func(c *Config) { c.Endpoints[0].Path = "users" }},
		{name: "missing method", change: func(c *Config) { c.Endpoints[0].Method = "" }},
		{name: "negative endpoint timeout", change: func(c *Config) { c.Endpoints[0].Timeout = -1 }},
		{name: "negative endpoint concurrency", change: func(c *Config) { c.Endpoints[0].MaxConcurrent = -1 }},
		{name: "negative readiness timeout", change: func(c *Config) { c.Server.ReadinessTimeout = -1 }},
		{name: "admin without address", change: func(c *Config) { c.Server.Admin.Enabled = true; c.Server.Admin.Address = "" }},
		{name: "invalid tracing endpoint", change: func(c *Config) { c.Tracing.Enabled = true; c.Tracing.Endpoint = "collector:4318" }},
		{name: "empty pipeline", change: func(c *Config) { c.Endpoints[0].Pipeline = nil }},
		{name: "empty step id", change: func(c *Config) { c.Endpoints[0].Pipeline[0].ID = "" }},
		{name: "undefined upstream", change: func(c *Config) { c.Endpoints[0].Pipeline[0].Upstream = "missing" }},
		{name: "missing fetch cache", change: func(c *Config) { c.Endpoints[0].Pipeline[0].Cache = &CacheRef{Backend: "missing", Key: "k"} }},
		{name: "invalid fetch cache", change: func(c *Config) { c.Endpoints[0].Pipeline[0].Cache = &CacheRef{Backend: "memory", TTL: -1} }},
		{name: "missing transform template", change: func(c *Config) { c.Endpoints[0].Pipeline[0] = Step{ID: "user", Type: "transform"} }},
		{name: "unknown step type", change: func(c *Config) { c.Endpoints[0].Pipeline[0].Type = "unknown" }},
		{name: "negative step timeout", change: func(c *Config) { c.Endpoints[0].Pipeline[0].Timeout = -1 }},
		{name: "invalid onError", change: func(c *Config) { c.Endpoints[0].Pipeline[0].OnError = "retry" }},
		{name: "fallback without policy", change: func(c *Config) { c.Endpoints[0].Pipeline[0].Fallback = &FallbackConfig{Static: `null`} }},
		{name: "fallback policy without config", change: func(c *Config) { c.Endpoints[0].Pipeline[0].OnError = "fallback" }},
		{name: "invalid static fallback", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0].OnError = "fallback"
			c.Endpoints[0].Pipeline[0].Fallback = &FallbackConfig{Static: "not json"}
		}},
		{name: "multiple fallback modes", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0].OnError = "fallback"
			c.Endpoints[0].Pipeline[0].Fallback = &FallbackConfig{Static: `null`, CacheKey: "key"}
		}},
		{name: "undefined fallback upstream", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0].OnError = "fallback"
			c.Endpoints[0].Pipeline[0].Fallback = &FallbackConfig{Upstream: "missing"}
		}},
		{name: "unknown dependency", change: func(c *Config) { c.Endpoints[0].Pipeline[0].DependsOn = []string{"missing"} }},
		{name: "undefined cache backend", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0] = Step{ID: "read", Type: "cache", Backend: "missing", Action: "get", Key: "k"}
		}},
		{name: "invalid cache action", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0] = Step{ID: "read", Type: "cache", Backend: "memory", Action: "purge", Key: "k"}
		}},
		{name: "cache set without value", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0] = Step{ID: "store", Type: "cache", Backend: "memory", Action: "set", Key: "k"}
		}},
		{name: "cache set unknown value", change: func(c *Config) {
			c.Endpoints[0].Pipeline[0] = Step{ID: "store", Type: "cache", Backend: "memory", Action: "set", Key: "k", Value: "missing"}
		}},
		{name: "cache set missing dependency", change: func(c *Config) {
			c.Endpoints[0].Pipeline = []Step{{ID: "source", Type: "transform", Template: "ok"}, {ID: "store", Type: "cache", Backend: "memory", Action: "set", Key: "k", Value: "source"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validConfig()
			test.change(cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestValidateRejectsDuplicateEndpointAndStepIDs(t *testing.T) {
	t.Run("endpoint", func(t *testing.T) {
		cfg := validConfig()
		cfg.Endpoints = append(cfg.Endpoints, cfg.Endpoints[0])
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected duplicate endpoint error")
		}
	})
	t.Run("step", func(t *testing.T) {
		cfg := validConfig()
		cfg.Endpoints[0].Pipeline = append(cfg.Endpoints[0].Pipeline, cfg.Endpoints[0].Pipeline[0])
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected duplicate step error")
		}
	})
}

func TestApplyDefaultsResilienceValues(t *testing.T) {
	cfg := validConfig()
	cfg.ApplyDefaults()
	upstream := cfg.Upstreams["users"]
	if upstream.MaxConcurrent != 100 || upstream.Retries == nil || *upstream.Retries != 2 || upstream.Retry.InitialInterval != Duration(100*time.Millisecond) || upstream.Retry.MaxElapsedTime != Duration(10*time.Second) || upstream.Retry.Jitter == nil || !*upstream.Retry.Jitter {
		t.Fatalf("unexpected upstream defaults: %#v", upstream)
	}
	if cfg.Endpoints[0].Timeout != Duration(10*time.Second) || cfg.Endpoints[0].MaxConcurrent != 100 || cfg.Endpoints[0].Pipeline[0].OnError != "fail" {
		t.Fatalf("unexpected endpoint/step defaults: %#v", cfg.Endpoints[0])
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestApplyDefaultsAdminAndTracingValues(t *testing.T) {
	cfg := validConfig()
	cfg.ApplyDefaults()
	if cfg.Server.ReadinessTimeout != Duration(5*time.Second) || cfg.Server.Admin.Address != ":9090" || cfg.Tracing.ServiceName != "elegba" {
		t.Fatalf("unexpected observability defaults: server=%#v tracing=%#v", cfg.Server, cfg.Tracing)
	}
}

func TestRedisCacheDefaultsAndValidation(t *testing.T) {
	cfg := validConfig()
	cfg.Caches["redis"] = Cache{Type: "redis", Address: "127.0.0.1:6379"}
	cfg.ApplyDefaults()
	redisCache := cfg.Caches["redis"]
	if redisCache.PoolSize != 10 || redisCache.MaxRetries != 3 || redisCache.DialTimeout != Duration(5*time.Second) || redisCache.ReadTimeout != Duration(3*time.Second) || redisCache.WriteTimeout != Duration(3*time.Second) {
		t.Fatalf("unexpected Redis defaults: %#v", redisCache)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	redisCache.Address = ""
	cfg.Caches["redis"] = redisCache
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected missing Redis address error")
	}
	redisCache.Address = "127.0.0.1:6379"
	redisCache.TLS = &CacheTLSConfig{CertFile: "client.pem"}
	cfg.Caches["redis"] = redisCache
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected Redis TLS cert/key pairing error")
	}
}

func TestValidateRateLimit(t *testing.T) {
	for _, value := range []string{"1/s", "100/m", "3/h"} {
		if err := ValidateRateLimit(value); err != nil {
			t.Errorf("%q should be valid: %v", value, err)
		}
	}
	for _, value := range []string{"0/s", "10/day", "bad", "2/s/extra"} {
		if err := ValidateRateLimit(value); err == nil {
			t.Errorf("%q should be invalid", value)
		}
	}
}
