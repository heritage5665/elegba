// File path: elegba/internal/step/registry.go

package step

import (
	"errors"
	"sync"

	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/pipeline"
	httptransport "github.com/elegba-dev/elegba/internal/transport"
)

type StepFactory func(config.Step, config.Upstream, httptransport.Transport) (pipeline.Step, error)

// Registry holds the registered step types.
type Registry struct {
	mu    sync.RWMutex
	steps map[string]StepFactory
}

// NewRegistry creates a new step registry.
func NewRegistry() *Registry {
	return &Registry{
		steps: make(map[string]StepFactory),
	}
}

// Register adds a new step type to the registry.
func (r *Registry) Register(name string, factory StepFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.steps[name]; exists {
		return errors.New("step type already registered: " + name)
	}
	r.steps[name] = factory
	return nil
}

func (r *Registry) Create(stepConfig config.Step, upstream config.Upstream, transport httptransport.Transport) (pipeline.Step, error) {
	factory, err := r.Get(stepConfig.Type)
	if err != nil {
		return nil, err
	}
	return factory(stepConfig, upstream, transport)
}

// Get retrieves a step factory by name.
func (r *Registry) Get(name string) (StepFactory, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.steps[name]
	if !exists {
		return nil, errors.New("step type not found: " + name)
	}
	return factory, nil
}
