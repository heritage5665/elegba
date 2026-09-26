package cache

import (
	"testing"

	"github.com/elegba-dev/elegba/internal/config"
)

func TestRegistryCreatesBackendsAndRejectsDuplicates(t *testing.T) {
	registry := NewDefaultRegistry()
	backend, err := registry.Create("memory", config.Cache{Type: "in-memory", MaxSize: 100})
	if err != nil {
		t.Fatal(err)
	}
	backend.Close()
	if _, err := registry.Create("unknown", config.Cache{Type: "missing"}); err == nil {
		t.Fatal("expected unknown backend error")
	}
	if err := registry.Register("custom", func(config.Cache) (Cache, error) { return &Ristretto{}, nil }); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("custom", func(config.Cache) (Cache, error) { return &Ristretto{}, nil }); err == nil {
		t.Fatal("expected duplicate backend error")
	}
}
