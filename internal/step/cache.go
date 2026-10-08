package step

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"text/template"

	cachebackend "github.com/elegba-dev/elegba/internal/cache"
	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/observability"
	"github.com/elegba-dev/elegba/internal/pipeline"
)

type CacheStep struct {
	config  config.Step
	cache   cachebackend.Cache
	key     *template.Template
	ttl     time.Duration
	metrics *observability.Metrics
}

func NewCacheStep(stepConfig config.Step, cache cachebackend.Cache, defaultTTL time.Duration) (*CacheStep, error) {
	key, err := parseTemplate("cache-key-"+stepConfig.ID, stepConfig.Key)
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(stepConfig.TTL)
	if ttl == 0 {
		ttl = defaultTTL
	}
	return &CacheStep{config: stepConfig, cache: cache, key: key, ttl: ttl}, nil
}

func (s *CacheStep) ID() string                                { return s.config.ID }
func (s *CacheStep) DependsOn() []string                       { return s.config.DependsOn }
func (s *CacheStep) SetMetrics(metrics *observability.Metrics) { s.metrics = metrics }

func (s *CacheStep) Execute(ctx context.Context, data map[string]any) (any, error) {
	key, err := renderTemplate(s.key, data)
	if err != nil {
		return nil, err
	}
	switch s.config.Action {
	case "get":
		value, ok, err := s.cache.Get(ctx, key)
		if s.metrics != nil {
			s.metrics.ObserveCache(s.config.Backend, err == nil && ok)
		}
		if err != nil {
			slog.WarnContext(ctx, "cache read failed; treating as a miss", "error", err, "backend", s.config.Backend)
			return nil, nil
		}
		if !ok {
			return nil, nil
		}
		return value, nil
	case "set":
		value, ok := data[s.config.Value]
		if !ok {
			return nil, fmt.Errorf("cache value step %q has no result", s.config.Value)
		}
		if err := s.cache.Set(ctx, key, value, s.ttl); err != nil {
			slog.WarnContext(ctx, "cache write failed; continuing without cache", "error", err, "backend", s.config.Backend)
		}
		return value, nil
	case "delete":
		if err := s.cache.Delete(ctx, key); err != nil {
			slog.WarnContext(ctx, "cache delete failed; continuing", "error", err, "backend", s.config.Backend)
		}
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported cache operation %q", s.config.Action)
	}
}

var _ pipeline.Step = (*CacheStep)(nil)
