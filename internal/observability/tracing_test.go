package observability

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elegba-dev/elegba/internal/config"
	"go.opentelemetry.io/otel"
)

func TestConfigureTracingExportsOTLPHTTP(t *testing.T) {
	requests := make(chan string, 1)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		requests <- req.Method + " " + req.URL.Path
		_, _ = io.Copy(io.Discard, req.Body)
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	tracing, err := ConfigureTracing(context.Background(), config.TracingConfig{
		Enabled: true, Endpoint: collector.URL, Insecure: true, ServiceName: "elegba-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, span := otel.Tracer("test").Start(context.Background(), "export-me")
	span.End()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := tracing.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case request := <-requests:
		if request != "POST /v1/traces" {
			t.Fatalf("unexpected OTLP request %q", request)
		}
	case <-ctx.Done():
		t.Fatal("OTLP exporter did not send spans before shutdown")
	}
}
