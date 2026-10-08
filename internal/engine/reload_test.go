package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/elegba-dev/elegba/internal/config"
)

func TestServerReloadsConfigFromFileUsingMockRESTServer(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"` + strings.TrimPrefix(r.URL.Path, "/") + `"}`))
	}))
	defer mock.Close()

	configPath := filepath.Join(t.TempDir(), "elegba.yaml")
	writeReloadConfig(t, configPath, mock.URL, "/original")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/data", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("initial response status = %d, want 200", response.Code)
	}
	if got := responseBody(t, response); got != "original" {
		t.Fatalf("initial response = %q, want original", got)
	}

	writeReloadConfig(t, configPath, mock.URL, "/replacement")
	if err := server.ReloadFile(configPath); err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/data", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("reloaded response status = %d, want 200", response.Code)
	}
	if got := responseBody(t, response); got != "replacement" {
		t.Fatalf("reloaded response = %q, want replacement", got)
	}
}

func TestServerWatchReloadsConfigFromFile(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"` + strings.TrimPrefix(r.URL.Path, "/") + `"}`))
	}))
	defer mock.Close()

	configPath := filepath.Join(t.TempDir(), "elegba.yaml")
	writeReloadConfig(t, configPath, mock.URL, "/original")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Watch(ctx, configPath, 25*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	writeReloadConfig(t, configPath, mock.URL, "/replacement")

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response := httptest.NewRecorder()
		server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/data", nil))
		if response.Code == http.StatusOK && responseBody(t, response) == "replacement" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("configuration file event did not reload the engine")
}

func TestServerRejectsInvalidReloadWithoutReplacingCurrentEngine(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"version":"original"}`))
	}))
	defer mock.Close()

	configPath := filepath.Join(t.TempDir(), "elegba.yaml")
	writeReloadConfig(t, configPath, mock.URL, "/original")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()

	response := httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/data", nil))
	if got := responseBody(t, response); got != "original" {
		t.Fatalf("initial response = %q, want original", got)
	}

	invalidPath := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("version: \"1\"\nendpoints: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := server.ReloadFile(invalidPath); err == nil {
		t.Fatal("expected invalid reload to fail")
	}

	response = httptest.NewRecorder()
	server.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/data", nil))
	if got := responseBody(t, response); got != "original" {
		t.Fatalf("response after invalid reload = %q, want original", got)
	}
}

func writeReloadConfig(t *testing.T, path, upstreamURL, stepPath string) {
	t.Helper()
	content := `version: "1"
server:
  address: "127.0.0.1:0"
  readinessTimeout: 1s
upstreams:
  source:
    baseURL: ` + upstreamURL + `
    timeout: 1s
    maxResponseBytes: 1048576
    maxConcurrent: 10
endpoints:
  - path: /data
    method: GET
    maxConcurrent: 10
    pipeline:
      - id: source
        type: fetch
        upstream: source
        path: ` + stepPath + `
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func responseBody(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("response body is not JSON: %s", response.Body.String())
	}
	value, ok := result["version"].(string)
	if !ok {
		t.Fatalf("response has no version field: %#v", result)
	}
	return value
}

var _ = time.Second
