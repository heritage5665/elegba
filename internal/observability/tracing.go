package observability

import (
	"context"
	"fmt"
	"net/url"

	"github.com/elegba-dev/elegba/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type Tracing struct {
	provider *sdktrace.TracerProvider
}

func ConfigureTracing(ctx context.Context, config config.TracingConfig) (*Tracing, error) {
	otel.SetTextMapPropagator(propagation.TraceContext{})
	if !config.Enabled {
		return &Tracing{}, nil
	}
	endpoint, err := url.Parse(config.Endpoint)
	if err != nil || endpoint.Host == "" || endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return nil, fmt.Errorf("invalid OTLP endpoint %q", config.Endpoint)
	}
	options := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint.Host)}
	if endpoint.Path != "" && endpoint.Path != "/" {
		options = append(options, otlptracehttp.WithURLPath(endpoint.Path))
	}
	if config.Insecure || endpoint.Scheme == "http" {
		options = append(options, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", config.ServiceName))),
	)
	otel.SetTracerProvider(provider)
	return &Tracing{provider: provider}, nil
}

func (t *Tracing) Shutdown(ctx context.Context) error {
	if t == nil || t.provider == nil {
		return nil
	}
	return t.provider.Shutdown(ctx)
}
