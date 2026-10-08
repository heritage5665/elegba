package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposeRequiredFamilies(t *testing.T) {
	metrics := NewMetrics()
	metrics.ObserveRequest("/users/{id}", "GET", 200, 20*time.Millisecond)
	metrics.ObserveStep("/users/{id}", "user", "users", 10*time.Millisecond)
	metrics.ObserveUpstreamError("users", "timeout")
	metrics.ObserveCache("memory", true)
	metrics.ObserveCache("memory", false)
	metrics.SetBreakerState("users", 1)
	metrics.AddInflight("/users/{id}", 1)
	response := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(response, httptest.NewRequest("GET", "/metrics", nil))
	for _, family := range []string{
		"elegba_requests_total",
		"elegba_request_duration_seconds",
		"elegba_step_duration_seconds",
		"elegba_upstream_errors_total",
		"elegba_cache_hits_total",
		"elegba_cache_misses_total",
		"elegba_circuit_breaker_state",
		"elegba_inflight_requests",
	} {
		if !strings.Contains(response.Body.String(), family) {
			t.Errorf("metric family %q missing from scrape output", family)
		}
	}
}

func TestRedactingHandlerScrubsSecretAttributes(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&output, nil)))
	ctx := ContextWithRequestID(context.Background(), "request-123")
	logger.InfoContext(ctx, "credentials", "token", "secret-token", slog.Group("auth", "password", "secret-password"), "api_key", "secret-key", "public", "visible")
	line := output.String()
	if strings.Contains(line, "secret-token") || strings.Contains(line, "secret-password") || strings.Contains(line, "secret-key") || !strings.Contains(line, "[REDACTED]") || !strings.Contains(line, "visible") || !strings.Contains(line, `"request_id":"request-123"`) {
		t.Fatalf("unexpected redacted log output %s", line)
	}
	var record map[string]any
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("log is not valid JSON: %v", err)
	}
	if record["request_id"] != "request-123" || record["msg"] != "credentials" {
		t.Fatalf("unexpected structured log fields %#v", record)
	}
}

func TestRedactingHandlerAddsSystemRequestIDOutsideRequestContext(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewJSONHandler(&output, nil)))
	logger.Info("startup")
	if !strings.Contains(output.String(), `"request_id":"system"`) {
		t.Fatalf("expected a system correlation ID, got %s", output.String())
	}
}
