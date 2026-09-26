package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	cachebackend "github.com/elegba-dev/elegba/internal/cache"
	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/observability"
	"github.com/elegba-dev/elegba/internal/pipeline"
	"github.com/elegba-dev/elegba/internal/step"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type configuredStep struct {
	inner      pipeline.Step
	config     config.Step
	fallback   pipeline.Step
	caches     map[string]cachebackend.Cache
	cacheNames []string
	endpoint   string
	metrics    *observability.Metrics
	tracer     trace.Tracer
}

func newConfiguredStep(stepConfig config.Step, inner, fallback pipeline.Step, caches map[string]cachebackend.Cache, endpoint string, metrics *observability.Metrics, tracer trace.Tracer) *configuredStep {
	names := make([]string, 0, len(caches))
	for name := range caches {
		names = append(names, name)
	}
	sort.Strings(names)
	return &configuredStep{
		inner: inner, config: stepConfig, fallback: fallback, caches: caches,
		cacheNames: names, endpoint: endpoint, metrics: metrics, tracer: tracer,
	}
}

func (s *configuredStep) ID() string          { return s.inner.ID() }
func (s *configuredStep) DependsOn() []string { return s.inner.DependsOn() }

func (s *configuredStep) Execute(ctx context.Context, data map[string]any) (any, error) {
	ctx, span := s.tracer.Start(ctx, "elegba.step", trace.WithAttributes(
		attribute.String("elegba.endpoint", s.endpoint),
		attribute.String("elegba.step.id", s.ID()),
		attribute.String("elegba.step.type", s.config.Type),
		attribute.String("elegba.upstream", s.config.Upstream),
	))
	started := time.Now()
	defer span.End()
	if s.config.Timeout <= 0 {
		result, err := s.inner.Execute(ctx, data)
		if err != nil && (s.config.OnError == "ignore" || s.config.OnError == "fallback") {
			result, err = s.recover(ctx, data, err)
		}
		s.recordStep(time.Since(started), err, span)
		return result, err
	}
	stepContext, cancel := context.WithTimeout(ctx, time.Duration(s.config.Timeout))
	defer cancel()
	result, err := s.inner.Execute(stepContext, data)
	if err != nil && (s.config.OnError == "ignore" || s.config.OnError == "fallback") {
		result, err = s.recover(stepContext, data, err)
	}
	s.recordStep(time.Since(started), err, span)
	return result, err
}

func (s *configuredStep) recordStep(duration time.Duration, err error, span trace.Span) {
	if s.metrics != nil {
		s.metrics.ObserveStep(s.endpoint, s.ID(), s.config.Upstream, duration)
	}
	if err != nil {
		span.SetStatus(codes.Error, "step failed")
	}
}

func (s *configuredStep) recover(ctx context.Context, data map[string]any, cause error) (any, error) {
	switch s.config.OnError {
	case "ignore":
		return nil, nil
	case "fallback":
	default:
		return nil, cause
	}
	fallback := s.config.Fallback
	switch {
	case fallback.Static != "":
		var value any
		decoder := json.NewDecoder(strings.NewReader(fallback.Static))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("decode static fallback: %w", err)
		}
		return value, nil
	case fallback.Upstream != "":
		return s.fallback.Execute(ctx, data)
	case fallback.CacheKey != "":
		key, err := step.RenderTemplate(fallback.CacheKey, data)
		if err != nil {
			return nil, fmt.Errorf("render cache fallback key: %w", err)
		}
		for _, name := range s.cacheNames {
			if value, ok, err := s.caches[name].Get(ctx, key); err == nil && ok {
				return value, nil
			}
		}
		return nil, fmt.Errorf("fallback cache key %q was not found", key)
	default:
		return nil, cause
	}
}

var _ pipeline.Step = (*configuredStep)(nil)
