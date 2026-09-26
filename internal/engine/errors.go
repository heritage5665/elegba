package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"

	"github.com/elegba-dev/elegba/internal/transport"
)

type apiError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func requestIDFrom(req *http.Request) string {
	requestID, _ := req.Context().Value(requestIDKey{}).(string)
	return requestID
}

func writeAPIError(w http.ResponseWriter, status int, code, message, requestID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]apiError{"error": {
		Code: code, Message: message, RequestID: requestID,
	}})
}

func pipelineErrorStatus(err error) (int, string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "UPSTREAM_TIMEOUT", "pipeline execution timed out"
	case errors.Is(err, transport.ErrCircuitBreakerOpen):
		return http.StatusServiceUnavailable, "CIRCUIT_OPEN", "upstream circuit breaker is open"
	case errors.Is(err, transport.ErrRetryExhausted), errors.Is(err, transport.ErrUpstreamFailure):
		return http.StatusBadGateway, "UPSTREAM_FAILURE", "upstream request failed"
	default:
		return http.StatusBadGateway, "PIPELINE_FAILURE", "pipeline execution failed"
	}
}

func partialErrorDetails(stepErrors map[string]error) map[string]apiError {
	stepIDs := make([]string, 0, len(stepErrors))
	for stepID := range stepErrors {
		stepIDs = append(stepIDs, stepID)
	}
	sort.Strings(stepIDs)
	details := make(map[string]apiError, len(stepErrors))
	for _, stepID := range stepIDs {
		_, code, message := pipelineErrorStatus(stepErrors[stepID])
		switch code {
		case "UPSTREAM_TIMEOUT":
			message = "step timed out"
		case "CIRCUIT_OPEN":
			message = "upstream circuit breaker is open"
		case "UPSTREAM_FAILURE":
			message = "upstream request failed"
		default:
			code = "STEP_FAILURE"
			message = "pipeline step failed"
		}
		details[stepID] = apiError{Code: code, Message: message}
	}
	return details
}

func addPartialErrors(result any, results map[string]any, stepErrors map[string]error) any {
	details := partialErrorDetails(stepErrors)
	switch value := result.(type) {
	case map[string]any:
		copy := make(map[string]any, len(value)+1)
		for key, item := range value {
			copy[key] = item
		}
		copy["_errors"] = details
		return copy
	case string:
		var object map[string]any
		if json.Unmarshal([]byte(value), &object) == nil && object != nil {
			object["_errors"] = details
			return object
		}
		return map[string]any{"result": value, "_errors": details}
	case nil:
		return map[string]any{"results": results, "_errors": details}
	default:
		return map[string]any{"result": value, "_errors": details}
	}
}
