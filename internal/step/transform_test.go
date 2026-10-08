package step

import (
	"context"
	"strings"
	"testing"

	"github.com/elegba-dev/elegba/internal/config"
)

func TestTransformTemplateFunctions(t *testing.T) {
	transform, err := NewTransformStep(config.Step{
		ID:       "result",
		Type:     "transform",
		Template: `{"count": {{ len .items }}, "total": {{ sum .items }}, "next": {{ add .first 2 }}, "parsed": {{ fromJSON .raw | toJSON }}, "fallback": {{ .missing | default "none" | toJSON }}, "tokenHash": {{ sha256 "token" | toJSON }}, "mapped": {{ dict "user" .user | toJSON }}, "token": {{ replacePrefix .token "Bearer " "" | toJSON }}, "redacted": {{ redact .secret | toJSON }}}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := transform.Execute(context.Background(), map[string]any{
		"items":   []int{2, 3},
		"first":   4,
		"raw":     `{"ok":true}`,
		"missing": "",
		"user":    map[string]any{"id": "42"},
		"token":   "Bearer client-token",
		"secret":  "super-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	gotString, ok := got.(string)
	if !ok {
		t.Fatalf("transform result = %T, want string", got)
	}
	if !strings.Contains(gotString, `"tokenHash": "3c469e9d6c5875d37a43f353d4f88e61fcf812c66eee3457465a40b0da4153e0"`) {
		t.Fatalf("sha256 function missing from transformation: %s", gotString)
	}
	if !strings.Contains(gotString, `"mapped": {"user":{"id":"42"}}`) {
		t.Fatalf("dict function missing from transformation: %s", gotString)
	}
	if !strings.Contains(gotString, `"token": "client-token"`) {
		t.Fatalf("replacePrefix function missing from transformation: %s", gotString)
	}
	if !strings.Contains(gotString, `"redacted": "[REDACTED]"`) {
		t.Fatalf("redact function missing from transformation: %s", gotString)
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
