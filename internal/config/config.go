package config

type Config struct {
	Version   string              `yaml:"version" json:"version"`
	Server    ServerConfig        `yaml:"server" json:"server"`
	Upstreams map[string]Upstream `yaml:"upstreams" json:"upstreams"`
	Caches    map[string]Cache    `yaml:"caches" json:"caches"`
	Endpoints []Endpoint          `yaml:"endpoints" json:"endpoints"`
	Tracing   TracingConfig       `yaml:"tracing" json:"tracing"`
}

type ServerConfig struct {
	Address           string      `yaml:"address" json:"address"`
	ReadTimeout       Duration    `yaml:"readTimeout" json:"readTimeout"`
	WriteTimeout      Duration    `yaml:"writeTimeout" json:"writeTimeout"`
	IdleTimeout       Duration    `yaml:"idleTimeout" json:"idleTimeout"`
	ReadHeaderTimeout Duration    `yaml:"readHeaderTimeout" json:"readHeaderTimeout"`
	ShutdownTimeout   Duration    `yaml:"shutdownTimeout" json:"shutdownTimeout"`
	ReadinessTimeout  Duration    `yaml:"readinessTimeout" json:"readinessTimeout"`
	MaxBodyBytes      int64       `yaml:"maxBodyBytes" json:"maxBodyBytes"`
	Admin             AdminConfig `yaml:"admin" json:"admin"`
}

type AdminConfig struct {
	Address string `yaml:"address" json:"address"`
	Enabled bool   `yaml:"enabled" json:"enabled"`
}

type TracingConfig struct {
	Enabled     bool   `yaml:"enabled" json:"enabled"`
	Endpoint    string `yaml:"endpoint" json:"endpoint"`
	Insecure    bool   `yaml:"insecure" json:"insecure"`
	ServiceName string `yaml:"serviceName" json:"serviceName"`
}

type Upstream struct {
	Transport        string            `yaml:"transport" json:"transport"`
	BaseURL          string            `yaml:"baseURL" json:"baseURL"`
	Timeout          Duration          `yaml:"timeout" json:"timeout"`
	MaxResponseBytes int64             `yaml:"maxResponseBytes" json:"maxResponseBytes"`
	MaxConcurrent    int               `yaml:"maxConcurrent" json:"maxConcurrent"`
	ConnectionPool   ConnectionPool    `yaml:"connectionPool" json:"connectionPool"`
	Retries          *int              `yaml:"retries" json:"retries"`
	Retry            RetryConfig       `yaml:"retry" json:"retry"`
	Breaker          *BreakerConfig    `yaml:"breaker" json:"breaker"`
	RateLimit        string            `yaml:"rateLimit" json:"rateLimit"`
	Headers          map[string]string `yaml:"headers" json:"headers"`
	Auth             AuthConfig        `yaml:"auth" json:"auth"`
}

type ConnectionPool struct {
	MaxIdleConns        int      `yaml:"maxIdleConns" json:"maxIdleConns"`
	MaxIdleConnsPerHost int      `yaml:"maxIdleConnsPerHost" json:"maxIdleConnsPerHost"`
	IdleConnTimeout     Duration `yaml:"idleConnTimeout" json:"idleConnTimeout"`
}

type RetryConfig struct {
	InitialInterval Duration `yaml:"initialInterval" json:"initialInterval"`
	MaxInterval     Duration `yaml:"maxInterval" json:"maxInterval"`
	Multiplier      float64  `yaml:"multiplier" json:"multiplier"`
	MaxElapsedTime  Duration `yaml:"maxElapsedTime" json:"maxElapsedTime"`
	Jitter          *bool    `yaml:"jitter" json:"jitter"`
}

type BreakerConfig struct {
	MaxRequests      uint32   `yaml:"maxRequests" json:"maxRequests"`
	Interval         Duration `yaml:"interval" json:"interval"`
	Timeout          Duration `yaml:"timeout" json:"timeout"`
	FailureThreshold uint32   `yaml:"failureThreshold" json:"failureThreshold"`
}

type AuthConfig struct {
	Type    string `yaml:"type" json:"type"`
	Token   string `yaml:"token" json:"token"`
	User    string `yaml:"username" json:"username"`
	Pass    string `yaml:"password" json:"password"`
	Key     string `yaml:"key" json:"key"`
	Header  string `yaml:"header" json:"header"`
	Forward string `yaml:"forward" json:"forward"`
	Scheme  string `yaml:"scheme" json:"scheme"`
}

type Cache struct {
	Type         string          `yaml:"type" json:"type"`
	MaxSize      int64           `yaml:"maxSize" json:"maxSize"`
	DefaultTTL   Duration        `yaml:"defaultTTL" json:"defaultTTL"`
	Address      string          `yaml:"address" json:"address"`
	Username     string          `yaml:"username" json:"username"`
	Password     string          `yaml:"password" json:"password"`
	DB           int             `yaml:"db" json:"db"`
	PoolSize     int             `yaml:"poolSize" json:"poolSize"`
	MaxRetries   int             `yaml:"maxRetries" json:"maxRetries"`
	DialTimeout  Duration        `yaml:"dialTimeout" json:"dialTimeout"`
	ReadTimeout  Duration        `yaml:"readTimeout" json:"readTimeout"`
	WriteTimeout Duration        `yaml:"writeTimeout" json:"writeTimeout"`
	TLS          *CacheTLSConfig `yaml:"tls" json:"tls"`
}

type CacheTLSConfig struct {
	CAFile             string `yaml:"caFile" json:"caFile"`
	CertFile           string `yaml:"certFile" json:"certFile"`
	KeyFile            string `yaml:"keyFile" json:"keyFile"`
	InsecureSkipVerify bool   `yaml:"insecureSkipVerify" json:"insecureSkipVerify"`
	ServerName         string `yaml:"serverName" json:"serverName"`
}

type CacheRef struct {
	Backend              string   `yaml:"backend" json:"backend"`
	Key                  string   `yaml:"key" json:"key"`
	TTL                  Duration `yaml:"ttl" json:"ttl"`
	StaleWhileRevalidate bool     `yaml:"staleWhileRevalidate" json:"staleWhileRevalidate"`
}

type Endpoint struct {
	Path            string   `yaml:"path" json:"path"`
	Method          string   `yaml:"method" json:"method"`
	Pipeline        []Step   `yaml:"pipeline" json:"pipeline"`
	CacheInvalidate []string `yaml:"cacheInvalidate" json:"cacheInvalidate"`
	Timeout         Duration `yaml:"timeout" json:"timeout"`
	MaxConcurrent   int      `yaml:"maxConcurrent" json:"maxConcurrent"`
	FailFast        *bool    `yaml:"failFast" json:"failFast"`
}

type Step struct {
	ID        string            `yaml:"id" json:"id"`
	Type      string            `yaml:"type" json:"type"`
	Upstream  string            `yaml:"upstream" json:"upstream"`
	Path      string            `yaml:"path" json:"path"`
	Method    string            `yaml:"method" json:"method"`
	Headers   map[string]string `yaml:"headers" json:"headers"`
	Body      string            `yaml:"body" json:"body"`
	DependsOn []string          `yaml:"dependsOn" json:"dependsOn"`
	Template  string            `yaml:"template" json:"template"`
	Cache     *CacheRef         `yaml:"cache" json:"cache"`
	Action    string            `yaml:"action" json:"action"`
	Backend   string            `yaml:"backend" json:"backend"`
	Key       string            `yaml:"key" json:"key"`
	Value     string            `yaml:"value" json:"value"`
	TTL       Duration          `yaml:"ttl" json:"ttl"`
	Timeout   Duration          `yaml:"timeout" json:"timeout"`
	OnError   string            `yaml:"onError" json:"onError"`
	Fallback  *FallbackConfig   `yaml:"fallback" json:"fallback"`
}

type FallbackConfig struct {
	Static   string `yaml:"static" json:"static"`
	Upstream string `yaml:"upstream" json:"upstream"`
	CacheKey string `yaml:"cacheKey" json:"cacheKey"`
}
