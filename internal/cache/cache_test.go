package cache

import (
	"context"
	"testing"
	"time"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestRistrettoEntryExpiresAfterTTL(t *testing.T) {
	backend, err := NewRistretto(100)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.Set(context.Background(), "short-lived", "value", 30*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, ok, err := backend.Get(context.Background(), "short-lived"); err != nil {
			t.Fatal(err)
		} else if !ok {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("Ristretto entry did not expire")
		}
	}
}

func TestRistrettoSetGetAndClose(t *testing.T) {
	backend, err := NewRistretto(100)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	if err := backend.Set(context.Background(), "key", "value", time.Minute); err != nil {
		t.Fatal(err)
	}
	value, ok, err := backend.Get(context.Background(), "key")
	if err != nil || !ok || value != "value" {
		t.Fatalf("unexpected cache result %v, %v, %v", value, ok, err)
	}
	if err := backend.Set(context.Background(), "expired", "value", 0); err == nil {
		t.Fatal("expected non-positive TTL error")
	}
	if err := backend.Delete(context.Background(), "key"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := backend.Get(context.Background(), "key"); err != nil || ok {
		t.Fatalf("expected deleted cache entry to miss, got ok=%v err=%v", ok, err)
	}
}
