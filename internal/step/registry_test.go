package step

import (
	"testing"

	"github.com/elegba-dev/elegba/internal/config"
	"github.com/elegba-dev/elegba/internal/pipeline"
	httptransport "github.com/elegba-dev/elegba/internal/transport"
)

func TestRegistryRegistersCreatesAndRejectsDuplicates(t *testing.T) {
	registry := NewRegistry()
	factory := func(config.Step, config.Upstream, httptransport.Transport) (pipeline.Step, error) { return nil, nil }
	if err := registry.Register("custom", factory); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("custom", factory); err == nil {
		t.Fatal("expected duplicate registration error")
	}
	if _, err := registry.Create(config.Step{Type: "custom"}, config.Upstream{}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Get("missing"); err == nil {
		t.Fatal("expected unknown step type error")
	}
}
