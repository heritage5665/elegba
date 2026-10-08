// File path: elegba/internal/pipeline/step.go

package pipeline

import "context"

type Step interface {
	Execute(context.Context, map[string]any) (any, error)
	ID() string
	DependsOn() []string
}

type FailureHandler interface {
	Recover(context.Context, map[string]any, error) (any, error)
}
