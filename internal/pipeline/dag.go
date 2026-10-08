// File path: elegba/internal/pipeline/dag.go

package pipeline

import (
	"fmt"
)

func TopologicalSort(steps []Step) ([]Step, error) {
	byID := make(map[string]Step, len(steps))
	positions := make(map[string]int, len(steps))
	children := make(map[string][]string)
	indegree := make(map[string]int, len(steps))
	for index, step := range steps {
		if step == nil || step.ID() == "" {
			return nil, fmt.Errorf("pipeline contains a step with an empty id")
		}
		if _, exists := byID[step.ID()]; exists {
			return nil, fmt.Errorf("duplicate step id %q", step.ID())
		}
		byID[step.ID()] = step
		positions[step.ID()] = index
		indegree[step.ID()] = len(step.DependsOn())
	}
	for _, step := range steps {
		seen := make(map[string]bool)
		for _, dependency := range step.DependsOn() {
			if _, exists := byID[dependency]; !exists {
				return nil, fmt.Errorf("step %q depends on unknown step %q", step.ID(), dependency)
			}
			if dependency == step.ID() || seen[dependency] {
				return nil, fmt.Errorf("step %q has invalid duplicate or self dependency %q", step.ID(), dependency)
			}
			seen[dependency] = true
			children[dependency] = append(children[dependency], step.ID())
		}
	}

	ready := make([]string, 0, len(steps))
	for _, step := range steps {
		if indegree[step.ID()] == 0 {
			ready = append(ready, step.ID())
		}
	}
	ordered := make([]Step, 0, len(steps))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byID[id])
		for _, child := range children[id] {
			indegree[child]--
			if indegree[child] == 0 {
				insertAt := len(ready)
				for i, candidate := range ready {
					if positions[child] < positions[candidate] {
						insertAt = i
						break
					}
				}
				ready = append(ready, "")
				copy(ready[insertAt+1:], ready[insertAt:])
				ready[insertAt] = child
			}
		}
	}
	if len(ordered) != len(steps) {
		return nil, fmt.Errorf("pipeline contains a dependency cycle")
	}
	return ordered, nil
}
