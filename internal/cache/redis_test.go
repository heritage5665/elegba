package cache

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/elegba-dev/elegba/internal/config"
)

func TestRedisSetGetDeleteAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	backend, err := NewRedis(config.Cache{
		Address: server.Addr(), PoolSize: 4, DefaultTTL: config.Duration(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	value := map[string]any{"count": json.Number("7"), "ok": true}
	if err := backend.Set(context.Background(), "user:7", value, time.Second); err != nil {
		t.Fatal(err)
	}
	got, ok, err := backend.Get(context.Background(), "user:7")
	if err != nil || !ok {
		t.Fatalf("expected Redis cache hit, got ok=%v err=%v", ok, err)
	}
	decoded := got.(map[string]any)
	if decoded["count"] != json.Number("7") || decoded["ok"] != true {
		t.Fatalf("unexpected Redis value %#v", got)
	}
	server.FastForward(2 * time.Second)
	if _, ok, err := backend.Get(context.Background(), "user:7"); err != nil || ok {
		t.Fatalf("expected Redis TTL expiry, got ok=%v err=%v", ok, err)
	}
	if err := backend.Set(context.Background(), "delete-me", "value", time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := backend.Delete(context.Background(), "delete-me"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := backend.Get(context.Background(), "delete-me"); err != nil || ok {
		t.Fatalf("expected Redis delete to remove entry, got ok=%v err=%v", ok, err)
	}
}

func TestRedisPreservesStaleValueMetadata(t *testing.T) {
	server := miniredis.RunT(t)
	backend, err := NewRedis(config.Cache{Address: server.Addr(), PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	freshUntil := time.Now().Add(-time.Second).UTC()
	if err := backend.Set(context.Background(), "stale", StaleValue{Value: "cached", FreshUntil: freshUntil}, time.Minute); err != nil {
		t.Fatal(err)
	}
	got, ok, err := backend.Get(context.Background(), "stale")
	if err != nil || !ok {
		t.Fatalf("expected stale cache hit, got ok=%v err=%v", ok, err)
	}
	stale, ok := got.(StaleValue)
	if !ok || stale.Value != "cached" || !stale.FreshUntil.Equal(freshUntil) {
		t.Fatalf("stale metadata was not preserved: %#v", got)
	}
}

func TestRedisReturnsBackendErrors(t *testing.T) {
	server := miniredis.RunT(t)
	backend, err := NewRedis(config.Cache{Address: server.Addr(), PoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	backend.Close()
	if _, _, err := backend.Get(context.Background(), "key"); err == nil {
		t.Fatal("expected Redis backend error")
	}
}

func TestRedisTLSConnection(t *testing.T) {
	certificateServer := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	certificate := certificateServer.TLS.Certificates[0]
	certificateServer.Close()
	server := miniredis.NewMiniRedis()
	if err := server.StartTLS(&tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}); err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	backend, err := NewRedis(config.Cache{
		Address: server.Addr(), PoolSize: 2,
		TLS: &config.CacheTLSConfig{InsecureSkipVerify: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.Set(context.Background(), "tls-key", "secure", time.Minute); err != nil {
		t.Fatal(err)
	}
	if value, ok, err := backend.Get(context.Background(), "tls-key"); err != nil || !ok || value != "secure" {
		t.Fatalf("Redis TLS read failed: value=%v ok=%v err=%v", value, ok, err)
	}
}
