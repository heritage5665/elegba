package pipeline

import (
	"errors"
	"testing"
)

func TestResultStoresResultsAndCopiesErrors(t *testing.T) {
	result := NewResult()
	result.SetResult("user", map[string]any{"id": "7"})
	if value, ok := result.GetResult("user"); !ok || value.(map[string]any)["id"] != "7" {
		t.Fatalf("unexpected stored result %#v, %v", value, ok)
	}
	result.SetError("user", errors.New("failed"))
	errorsCopy := result.GetErrors()
	errorsCopy["other"] = errors.New("external mutation")
	if len(result.GetErrors()) != 1 {
		t.Fatal("GetErrors exposed the internal error map")
	}
}
