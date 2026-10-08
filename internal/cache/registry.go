package cache

import (
	"fmt"
	"sync"

	"github.com/elegba-dev/elegba/internal/config"
)

type Factory func(config.Cache) (Cache, error)

type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]Factory)}
}

func NewDefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.factories["in-memory"] = func(cfg config.Cache) (Cache, error) {
		return NewRistretto(cfg.MaxSize)
	}
	registry.factories["redis"] = func(cfg config.Cache) (Cache, error) {
		return NewRedis(cfg)
	}
	return registry
}

func (r *Registry) Register(name string, factory Factory) error {
	if name == "" || factory == nil {
		return fmt.Errorf("cache backend name and factory are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.factories[name]; exists {
		return fmt.Errorf("cache backend %q is already registered", name)
	}
	r.factories[name] = factory
	return nil
}

func (r *Registry) Create(name string, cfg config.Cache) (Cache, error) {
	r.mu.RLock()
	factory, ok := r.factories[cfg.Type]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("cache %q uses unknown backend type %q", name, cfg.Type)
	}
	backend, err := factory(cfg)
	if err != nil {
		return nil, fmt.Errorf("initialize cache %q: %w", name, err)
	}
	return backend, nil
}
