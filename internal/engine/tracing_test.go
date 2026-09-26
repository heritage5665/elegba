package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elegba-dev/elegba/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestRequestTraceHierarchyAndUpstreamPropagation(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(resource.Empty()),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("shutdown tracer provider: %v", err)
		}
	}()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	var upstreamTraceparent, upstreamRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		upstreamTraceparent = req.Header.Get("traceparent")
		upstreamRequestID = req.Header.Get("X-Request-ID")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()
	app, err := New(&config.Config{
		Version:   "1",
		Upstreams: map[string]config.Upstream{"api": {BaseURL: upstream.URL, Retries: intPointer(0)}},
		Endpoints: []config.Endpoint{{Path: "/trace", Method: http.MethodGet, Pipeline: []config.Step{{ID: "fetch", Type: "fetch", Upstream: "api"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	parentContext, parentSpan := otel.Tracer("test").Start(context.Background(), "caller")
	var inbound propagation.MapCarrier = make(propagation.MapCarrier)
	propagation.TraceContext{}.Inject(parentContext, inbound)
	request := httptest.NewRequest(http.MethodGet, "/trace", nil)
	for key, value := range inbound {
		request.Header.Set(key, value)
	}
	response := httptest.NewRecorder()
	app.ServeHTTP(response, request)
	parentSpan.End()
	if response.Code != http.StatusOK {
		t.Fatalf("request returned %d: %s", response.Code, response.Body.String())
	}
	if upstreamRequestID == "" || upstreamRequestID != response.Header().Get("X-Request-ID") {
		t.Fatalf("request ID was not propagated: upstream=%q response=%q", upstreamRequestID, response.Header().Get("X-Request-ID"))
	}
	upstreamContext := propagation.TraceContext{}.Extract(context.Background(), propagation.HeaderCarrier(http.Header{"Traceparent": []string{upstreamTraceparent}}))
	upstreamSpanContext := trace.SpanContextFromContext(upstreamContext)
	if !upstreamSpanContext.IsValid() || upstreamSpanContext.TraceID() != trace.SpanContextFromContext(parentContext).TraceID() {
		t.Fatalf("traceparent was not propagated: %q", upstreamTraceparent)
	}
	spans := exporter.GetSpans()
	spanByName := make(map[string]tracetest.SpanStub)
	for _, span := range spans {
		spanByName[span.Name] = span
	}
	requestSpan, requestOK := spanByName["elegba.request"]
	stepSpan, stepOK := spanByName["elegba.step"]
	upstreamSpan, upstreamOK := spanByName["elegba.upstream"]
	if !requestOK || !stepOK || !upstreamOK {
		t.Fatalf("missing expected spans: %#v", spanByName)
	}
	if requestSpan.Parent.SpanID() != trace.SpanContextFromContext(parentContext).SpanID() || stepSpan.Parent.SpanID() != requestSpan.SpanContext.SpanID() || upstreamSpan.Parent.SpanID() != stepSpan.SpanContext.SpanID() {
		t.Fatalf("unexpected span hierarchy: request=%v step=%v upstream=%v", requestSpan.Parent, stepSpan.Parent, upstreamSpan.Parent)
	}
}
