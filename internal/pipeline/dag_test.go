package pipeline

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type testStep struct {
	id      string
	deps    []string
	value   any
	stepErr error
	wait    bool
}

func (s testStep) ID() string          { return s.id }
func (s testStep) DependsOn() []string { return s.deps }
func (s testStep) Execute(ctx context.Context, _ map[string]any) (any, error) {
	if s.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return s.value, s.stepErr
}

func TestTopologicalSortPreservesDependencies(t *testing.T) {
	steps := []Step{
		testStep{id: "combined", deps: []string{"user", "orders"}},
		testStep{id: "user"},
		testStep{id: "orders"},
	}
	ordered, err := TopologicalSort(steps)
	if err != nil {
		t.Fatal(err)
	}
	positions := make(map[string]int)
	for index, step := range ordered {
		positions[step.ID()] = index
	}
	if positions["combined"] <= positions["user"] || positions["combined"] <= positions["orders"] {
		t.Fatalf("dependencies were not ordered before combined: %#v", positions)
	}
}

func TestTopologicalSortRejectsInvalidGraphs(t *testing.T) {
	tests := []struct {
		name  string
		steps []Step
	}{
		{name: "cycle", steps: []Step{testStep{id: "a", deps: []string{"b"}}, testStep{id: "b", deps: []string{"a"}}}},
		{name: "unknown dependency", steps: []Step{testStep{id: "a", deps: []string{"missing"}}}},
		{name: "duplicate id", steps: []Step{testStep{id: "a"}, testStep{id: "a"}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := TopologicalSort(test.steps); err == nil {
				t.Fatal("expected graph validation error")
			}
		})
	}
}

func TestExecuteRunsDependenciesAndPropagatesResults(t *testing.T) {
	steps := []Step{
		testStep{id: "joined", deps: []string{"source"}, value: "done"},
		testStep{id: "source", value: "ready"},
	}
	results, err := Execute(context.Background(), steps, map[string]any{"query": map[string]any{"id": "7"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(results, map[string]any{"source": "ready", "joined": "done"}) {
		t.Fatalf("unexpected results %#v", results)
	}
	partial, stepErrors, err := ExecuteWithErrors(context.Background(), []Step{
		testStep{id: "broken", stepErr: errors.New("failed")},
		testStep{id: "independent", value: "available"},
	}, nil, false)
	if err != nil || stepErrors["broken"] == nil || partial["independent"] != "available" {
		t.Fatalf("expected partial result and collected error, got results=%#v errors=%#v err=%v", partial, stepErrors, err)
	}
	_, err = Execute(context.Background(), []Step{testStep{id: "broken", stepErr: errors.New("failed")}}, nil, true)
	if err == nil {
		t.Fatal("expected fail-fast step error")
	}
}

func TestExecuteFailFastCancelsWave(t *testing.T) {
	steps := []Step{
		testStep{id: "blocked", wait: true},
		testStep{id: "broken", stepErr: errors.New("failed")},
	}
	if _, err := Execute(context.Background(), steps, nil, true); err == nil {
		t.Fatal("expected fail-fast error")
	}
}
