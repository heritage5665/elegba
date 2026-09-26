package step

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/elegba-dev/elegba/internal/config"
)

func TestTransformTemplateFunctions(t *testing.T) {
	transform, err := NewTransformStep(config.Step{
		ID:       "result",
		Type:     "transform",
		Template: `{"count": {{ len .items }}, "total": {{ sum .items }}, "next": {{ add .first 2 }}, "parsed": {{ fromJSON .raw | toJSON }}, "fallback": {{ .missing | default "none" | toJSON }}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := transform.Execute(context.Background(), map[string]any{
		"items":   []int{2, 3},
		"first":   4,
		"raw":     `{"ok":true}`,
		"missing": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, err := os.ReadFile("testdata/transform.golden")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSpace(string(wantBytes))
	if got != want {
		t.Fatalf("unexpected transformation\n got: %s\nwant: %s", got, want)
	}
}

func TestTransformLenRejectsUnsupportedInput(t *testing.T) {
	transform, err := NewTransformStep(config.Step{ID: "length", Template: `{{ len .value }}`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Execute(context.Background(), map[string]any{"value": 7}); err == nil {
		t.Fatal("expected unsupported len input error")
	}
}
