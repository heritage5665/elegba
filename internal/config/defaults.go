package config

import "time"

const (
	DefaultTimeout          = 2 * time.Second
	DefaultMaxResponseBytes = 10 << 20
	DefaultMaxBodyBytes     = 1 << 20
	DefaultShutdownTimeout  = 30 * time.Second
)

func NewConfig() *Config {
	return &Config{
		Version: "1",
		Server: ServerConfig{
			Address:           ":8080",
			ReadTimeout:       Duration(5 * time.Second),
			WriteTimeout:      Duration(10 * time.Second),
			IdleTimeout:       Duration(120 * time.Second),
			ReadHeaderTimeout: Duration(5 * time.Second),
			ShutdownTimeout:   Duration(DefaultShutdownTimeout),
			ReadinessTimeout:  Duration(5 * time.Second),
			MaxBodyBytes:      DefaultMaxBodyBytes,
			Admin: AdminConfig{
				Address: ":9090",
			},
		},
		Upstreams: make(map[string]Upstream),
		Caches:    make(map[string]Cache),
	}
}

func (c *Config) ApplyDefaults() {
	defaults := NewConfig()
	if c.Version == "" {
		c.Version = defaults.Version
	}
	if c.Server.Address == "" {
		c.Server.Address = defaults.Server.Address
	}
	if c.Server.ReadTimeout == 0 {
		c.Server.ReadTimeout = defaults.Server.ReadTimeout
	}
	if c.Server.WriteTimeout == 0 {
		c.Server.WriteTimeout = defaults.Server.WriteTimeout
	}
	if c.Server.IdleTimeout == 0 {
		c.Server.IdleTimeout = defaults.Server.IdleTimeout
	}
	if c.Server.ReadHeaderTimeout == 0 {
		c.Server.ReadHeaderTimeout = defaults.Server.ReadHeaderTimeout
	}
	if c.Server.ShutdownTimeout == 0 {
		c.Server.ShutdownTimeout = defaults.Server.ShutdownTimeout
	}
	if c.Server.ReadinessTimeout == 0 {
		c.Server.ReadinessTimeout = defaults.Server.ReadinessTimeout
	}
	if c.Server.Admin.Address == "" {
		c.Server.Admin.Address = defaults.Server.Admin.Address
	}
	if c.Server.MaxBodyBytes == 0 {
		c.Server.MaxBodyBytes = defaults.Server.MaxBodyBytes
	}
	if c.Upstreams == nil {
		c.Upstreams = make(map[string]Upstream)
	}
	if c.Caches == nil {
		c.Caches = make(map[string]Cache)
	}
	if c.Tracing.ServiceName == "" {
		c.Tracing.ServiceName = "elegba"
	}
	for name, cache := range c.Caches {
		if cache.Type == "" {
			cache.Type = "in-memory"
		}
		if cache.Type == "in-memory" && cache.MaxSize == 0 {
			cache.MaxSize = 10000
		}
		if cache.DefaultTTL == 0 {
			cache.DefaultTTL = Duration(time.Minute)
		}
		if cache.Type == "redis" {
			if cache.PoolSize == 0 {
				cache.PoolSize = 10
			}
			if cache.MaxRetries == 0 {
				cache.MaxRetries = 3
			}
			if cache.DialTimeout == 0 {
				cache.DialTimeout = Duration(5 * time.Second)
			}
			if cache.ReadTimeout == 0 {
				cache.ReadTimeout = Duration(3 * time.Second)
			}
			if cache.WriteTimeout == 0 {
				cache.WriteTimeout = Duration(3 * time.Second)
			}
		}
		c.Caches[name] = cache
	}
	for name, upstream := range c.Upstreams {
		if upstream.Timeout == 0 {
			upstream.Timeout = Duration(DefaultTimeout)
		}
		if upstream.MaxResponseBytes == 0 {
			upstream.MaxResponseBytes = DefaultMaxResponseBytes
		}
		if upstream.Headers == nil {
			upstream.Headers = make(map[string]string)
		}
		if upstream.MaxConcurrent == 0 {
			upstream.MaxConcurrent = 100
		}
		if upstream.Retries == nil {
			retries := 2
			upstream.Retries = &retries
		}
		if upstream.Retry.InitialInterval == 0 {
			upstream.Retry.InitialInterval = Duration(100 * time.Millisecond)
		}
		if upstream.Retry.MaxInterval == 0 {
			upstream.Retry.MaxInterval = Duration(2 * time.Second)
		}
		if upstream.Retry.Multiplier == 0 {
			upstream.Retry.Multiplier = 2
		}
		if upstream.Retry.MaxElapsedTime == 0 {
			upstream.Retry.MaxElapsedTime = Duration(10 * time.Second)
		}
		if upstream.Retry.Jitter == nil {
			jitter := true
			upstream.Retry.Jitter = &jitter
		}
		if upstream.Breaker != nil {
			if upstream.Breaker.MaxRequests == 0 {
				upstream.Breaker.MaxRequests = 5
			}
			if upstream.Breaker.Interval == 0 {
				upstream.Breaker.Interval = Duration(time.Minute)
			}
			if upstream.Breaker.Timeout == 0 {
				upstream.Breaker.Timeout = Duration(30 * time.Second)
			}
			if upstream.Breaker.FailureThreshold == 0 {
				upstream.Breaker.FailureThreshold = 5
			}
		}
		c.Upstreams[name] = upstream
	}
	for i := range c.Endpoints {
		if c.Endpoints[i].Method == "" {
			c.Endpoints[i].Method = "GET"
		}
		if c.Endpoints[i].Timeout == 0 {
			c.Endpoints[i].Timeout = Duration(10 * time.Second)
		}
		if c.Endpoints[i].MaxConcurrent == 0 {
			c.Endpoints[i].MaxConcurrent = 100
		}
		for j := range c.Endpoints[i].Pipeline {
			if c.Endpoints[i].Pipeline[j].OnError == "" {
				c.Endpoints[i].Pipeline[j].OnError = "fail"
			}
		}
	}
}
