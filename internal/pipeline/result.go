package pipeline

import "sync"

// Result stores the results of pipeline execution, allowing for concurrent access.
type Result struct {
	mu      sync.RWMutex
	results map[string]interface{}
	errors  map[string]error
}

// NewResult creates a new Result instance.
func NewResult() *Result {
	return &Result{
		results: make(map[string]interface{}),
		errors:  make(map[string]error),
	}
}

// SetResult sets the result for a given step ID.
func (r *Result) SetResult(stepID string, result interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results[stepID] = result
}

// GetResult retrieves the result for a given step ID.
func (r *Result) GetResult(stepID string) (interface{}, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result, exists := r.results[stepID]
	return result, exists
}

// SetError sets the error for a given step ID.
func (r *Result) SetError(stepID string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors[stepID] = err
}

// GetErrors retrieves all errors recorded during execution.
func (r *Result) GetErrors() map[string]error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	errors := make(map[string]error, len(r.errors))
	for stepID, err := range r.errors {
		errors[stepID] = err
	}
	return errors
}
