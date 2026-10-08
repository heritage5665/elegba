package pipeline

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
)

func Execute(ctx context.Context, steps []Step, input map[string]any, failFast bool) (map[string]any, error) {
	results, _, err := ExecuteWithErrors(ctx, steps, input, failFast)
	return results, err
}

func ExecuteWithErrors(ctx context.Context, steps []Step, input map[string]any, failFast bool) (map[string]any, map[string]error, error) {
	ordered, err := TopologicalSort(steps)
	if err != nil {
		return nil, nil, err
	}
	results := make(map[string]any, len(steps))
	stepErrors := make(map[string]error)
	completed := make(map[string]bool, len(steps))
	for len(completed) < len(ordered) {
		var wave []Step
		for _, candidate := range ordered {
			if completed[candidate.ID()] {
				continue
			}
			ready := true
			for _, dependency := range candidate.DependsOn() {
				if !completed[dependency] {
					ready = false
					break
				}
			}
			if ready {
				wave = append(wave, candidate)
			}
		}
		if len(wave) == 0 {
			return nil, nil, fmt.Errorf("pipeline cannot make progress")
		}

		data := make(map[string]any, len(input)+len(results))
		for key, value := range input {
			data[key] = value
		}
		for key, value := range results {
			data[key] = value
		}
		type outcome struct {
			id     string
			result any
			err    error
		}
		outcomes := make(chan outcome, len(wave))
		group, waveContext := errgroup.WithContext(ctx)
		for _, current := range wave {
			group.Go(func() error {
				result, stepErr := current.Execute(waveContext, data)
				if stepErr != nil {
					if handler, ok := current.(FailureHandler); ok {
						result, stepErr = handler.Recover(waveContext, data, stepErr)
					}
				}
				outcomes <- outcome{id: current.ID(), result: result, err: stepErr}
				if failFast {
					return stepErr
				}
				return nil
			})
		}
		_ = group.Wait()
		close(outcomes)
		var firstErr error
		for result := range outcomes {
			completed[result.id] = true
			if result.err != nil {
				stepErrors[result.id] = result.err
				if firstErr == nil {
					firstErr = fmt.Errorf("step %q: %w", result.id, result.err)
				}
				continue
			}
			results[result.id] = result.result
		}
		if ctx.Err() != nil {
			return results, stepErrors, ctx.Err()
		}
		if failFast && firstErr != nil {
			return results, stepErrors, firstErr
		}
	}
	return results, stepErrors, nil
}
