package step

import (
	"context"
	"errors"
	"testing"
	"time"

	cachebackend "github.com/elegba-dev/elegba/internal/cache"
	"github.com/elegba-dev/elegba/internal/config"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestCacheStepsGetAndSet(t *testing.T) {
	backend, err := cachebackend.NewRistretto(100)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	setStep, err := NewCacheStep(config.Step{
		ID: "store", Type: "cache", Action: "set", Key: "user:1", Value: "user", DependsOn: []string{"user"},
	}, backend, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	data := map[string]any{"user": map[string]any{"name": "Ada"}}
	if _, err := setStep.Execute(context.Background(), data); err != nil {
		t.Fatal(err)
	}
	getStep, err := NewCacheStep(config.Step{ID: "load", Type: "cache", Action: "get", Key: "user:1"}, backend, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	value, err := getStep.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if value.(map[string]any)["name"] != "Ada" {
		t.Fatalf("unexpected cache value %#v", value)
	}
	deleteStep, err := NewCacheStep(config.Step{ID: "remove", Type: "cache", Action: "delete", Key: "user:1"}, backend, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deleteStep.Execute(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	value, err = getStep.Execute(context.Background(), nil)
	if err != nil || value != nil {
		t.Fatalf("expected cache miss to produce nil without error, got value=%#v err=%v", value, err)
	}
}

func TestCacheStepOperationsFailOpenOnBackendErrors(t *testing.T) {
	backend := &testCache{
		getErr: errors.New("cache offline"),
		setErr: errors.New("cache offline"),
	}
	for _, action := range []string{"get", "set", "delete"} {
		t.Run(action, func(t *testing.T) {
			stepConfig := config.Step{ID: "cache", Type: "cache", Action: action, Key: "key", Value: "value"}
			cacheStep, err := NewCacheStep(stepConfig, backend, time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			result, err := cacheStep.Execute(context.Background(), map[string]any{"value": "cached"})
			if err != nil {
				t.Fatalf("cache %s error should fail open: %v", action, err)
			}
			if action == "set" && result != "cached" {
				t.Fatalf("cache set should pass through the value, got %#v", result)
			}
		})
	}
}
