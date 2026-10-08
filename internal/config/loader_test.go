package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigInterpolatesAndAppliesDefaults(t *testing.T) {
	t.Setenv("ELEGBA_TEST_BASE_URL", "https://example.test")
	path := filepath.Join(t.TempDir(), "elegba.yaml")
	data := `version: "1"
upstreams:
  users:
    baseURL: ${ELEGBA_TEST_BASE_URL}
    timeout: 250ms
endpoints:
  - path: /users/{id}
    method: GET
    pipeline:
      - id: user
        type: fetch
        upstream: users
        path: /users/{{ .path.id }}
`
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Address != ":8080" || time.Duration(cfg.Upstreams["users"].Timeout) != 250*time.Millisecond || cfg.Upstreams["users"].ConnectionPool.MaxIdleConns != 100 {
		t.Fatalf("defaults or duration not applied: %#v", cfg)
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "elegba.yaml")
	data := "version: \"1\"\nendpoints: []\nunknown: true\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected strict unknown-field error, got %v", err)
	}
}

func TestInterpolateRequiresVariables(t *testing.T) {
	_, err := interpolate(map[string]any{"token": "${ELEGBA_MISSING_TEST_VAR}"})
	if err == nil || !strings.Contains(err.Error(), "ELEGBA_MISSING_TEST_VAR") {
		t.Fatalf("expected missing variable error, got %v", err)
	}
	got, err := interpolate("${ELEGBA_MISSING_TEST_VAR:-fallback}")
	if err != nil || got != "fallback" {
		t.Fatalf("expected default interpolation, got %v, %v", got, err)
	}
	got, err = interpolate("${ELEGBA_MISSING_TEST_VAR:-}")
	if err != nil || got != "" {
		t.Fatalf("expected empty default interpolation, got %v, %v", got, err)
	}
}

func TestLoadExampleConfiguration(t *testing.T) {
	if _, err := LoadConfig(filepath.Join("..", "..", "examples", "elegba.yaml")); err != nil {
		t.Fatal(err)
	}
}
