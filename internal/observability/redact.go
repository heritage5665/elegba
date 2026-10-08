package observability

import (
	"context"
	"log/slog"
	"strings"
)

type redactingHandler struct {
	next slog.Handler
}

type requestIDContextKey struct{}

func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDContextKey{}, requestID)
}

func NewRedactingHandler(next slog.Handler) slog.Handler {
	return &redactingHandler{next: next}
}

func (h *redactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *redactingHandler) Handle(ctx context.Context, record slog.Record) error {
	safe := slog.NewRecord(record.Time, record.Level, record.Message, record.PC)
	requestID, _ := ctx.Value(requestIDContextKey{}).(string)
	if requestID == "" {
		requestID = "system"
	}
	if !hasRequestID(record) {
		safe.AddAttrs(slog.String("request_id", requestID))
	}
	record.Attrs(func(attr slog.Attr) bool {
		safe.AddAttrs(redactAttr(attr))
		return true
	})
	return h.next.Handle(ctx, safe)
}

func hasRequestID(record slog.Record) bool {
	found := false
	record.Attrs(func(attr slog.Attr) bool {
		if attr.Key == "request_id" {
			found = true
			return false
		}
		return true
	})
	return found
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	safe := make([]slog.Attr, len(attrs))
	for index, attr := range attrs {
		safe[index] = redactAttr(attr)
	}
	return &redactingHandler{next: h.next.WithAttrs(safe)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{next: h.next.WithGroup(name)}
}

func redactAttr(attr slog.Attr) slog.Attr {
	attr.Value = attr.Value.Resolve()
	if isSecretKey(attr.Key) {
		attr.Value = slog.StringValue("[REDACTED]")
		return attr
	}
	if attr.Value.Kind() == slog.KindGroup {
		group := attr.Value.Group()
		for index := range group {
			group[index] = redactAttr(group[index])
		}
		attr.Value = slog.GroupValue(group...)
	}
	return attr
}

func isSecretKey(key string) bool {
	key = strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
	for _, secret := range []string{"token", "password", "secret", "apikey", "authorization", "credential"} {
		if strings.Contains(key, secret) {
			return true
		}
	}
	return false
}
