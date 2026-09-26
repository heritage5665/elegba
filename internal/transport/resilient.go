package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/elegba-dev/elegba/internal/observability"
	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

var (
	ErrCircuitBreakerOpen = errors.New("upstream circuit breaker is open")
	ErrRetryExhausted     = errors.New("upstream retry budget exhausted")
	ErrUpstreamFailure    = errors.New("upstream returned a failure status")
)

type RetryOptions struct {
	MaxRetries      int
	InitialInterval time.Duration
	MaxInterval     time.Duration
	Multiplier      float64
	MaxElapsedTime  time.Duration
	Jitter          bool
}

type BreakerOptions struct {
	MaxRequests      uint32
	Interval         time.Duration
	Timeout          time.Duration
	FailureThreshold uint32
}

type ResilienceOptions struct {
	Retries       RetryOptions
	Breaker       *BreakerOptions
	RateLimit     string
	MaxConcurrent int
	Metrics       *observability.Metrics
}

type ResilientTransport struct {
	name    string
	base    Transport
	retry   RetryOptions
	limiter *rate.Limiter
	active  chan struct{}
	breaker *gobreaker.CircuitBreaker[*http.Response]
	metrics *observability.Metrics
}

type statusError struct {
	status int
}

type releaseBody struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (b *releaseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}

func (e *statusError) Error() string { return fmt.Sprintf("upstream returned HTTP %d", e.status) }
func (e *statusError) Unwrap() error { return ErrUpstreamFailure }

func NewResilientTransport(name string, base Transport, options ResilienceOptions) (*ResilientTransport, error) {
	if base == nil {
		return nil, fmt.Errorf("upstream %q transport is required", name)
	}
	if options.Retries.MaxRetries < 0 {
		return nil, fmt.Errorf("upstream %q retries must be non-negative", name)
	}
	if options.Retries.InitialInterval <= 0 {
		options.Retries.InitialInterval = 100 * time.Millisecond
	}
	if options.Retries.MaxInterval <= 0 {
		options.Retries.MaxInterval = 2 * time.Second
	}
	if options.Retries.Multiplier < 1 {
		options.Retries.Multiplier = 2
	}
	if options.Retries.MaxElapsedTime <= 0 {
		options.Retries.MaxElapsedTime = 10 * time.Second
	}
	resilient := &ResilientTransport{name: name, base: base, retry: options.Retries, metrics: options.Metrics}
	if options.MaxConcurrent > 0 {
		resilient.active = make(chan struct{}, options.MaxConcurrent)
	}
	if options.RateLimit != "" {
		limiter, err := newRateLimiter(options.RateLimit)
		if err != nil {
			return nil, fmt.Errorf("upstream %q: %w", name, err)
		}
		resilient.limiter = limiter
	}
	if options.Breaker != nil {
		breakerOptions := options.Breaker
		if breakerOptions.FailureThreshold == 0 || breakerOptions.MaxRequests == 0 || breakerOptions.Interval <= 0 || breakerOptions.Timeout <= 0 {
			return nil, fmt.Errorf("upstream %q breaker values must be positive", name)
		}
		resilient.breaker = gobreaker.NewCircuitBreaker[*http.Response](gobreaker.Settings{
			Name:        name,
			MaxRequests: breakerOptions.MaxRequests,
			Interval:    breakerOptions.Interval,
			Timeout:     breakerOptions.Timeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= breakerOptions.FailureThreshold
			},
			OnStateChange: func(name string, from, to gobreaker.State) {
				slog.Info("upstream circuit breaker state changed", "upstream", name, "from", from.String(), "to", to.String())
				if resilient.metrics != nil {
					resilient.metrics.SetBreakerState(name, breakerStateValue(to))
				}
			},
		})
		if resilient.metrics != nil {
			resilient.metrics.SetBreakerState(name, breakerStateValue(gobreaker.StateClosed))
		}
	}
	return resilient, nil
}

func (r *ResilientTransport) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	ctx, span := otel.Tracer("github.com/elegba-dev/elegba/internal/transport").Start(ctx, "elegba.upstream", trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(
		attribute.String("elegba.upstream", r.name),
		attribute.String("http.request.method", req.Method),
	))
	finish := func(err error, response *http.Response) {
		if err != nil {
			span.SetStatus(codes.Error, "upstream request failed")
			span.RecordError(fmt.Errorf("upstream request failed"))
		} else if response != nil {
			span.SetAttributes(attribute.Int("http.response.status_code", response.StatusCode))
			if response.StatusCode >= http.StatusBadRequest {
				span.SetStatus(codes.Error, "upstream returned error status")
			}
		}
		if err != nil || response != nil && response.StatusCode >= http.StatusBadRequest {
			if r.metrics != nil {
				r.metrics.ObserveUpstreamError(r.name, upstreamFailureReason(err, response))
			}
		}
		span.End()
	}
	call := func() (*http.Response, error) {
		return r.doWithRetry(ctx, req)
	}
	var response *http.Response
	var err error
	if r.breaker == nil {
		response, err = call()
	} else {
		response, err = r.breaker.Execute(call)
	}
	if err != nil {
		if r.breaker != nil && (errors.Is(err, gobreaker.ErrOpenState) || errors.Is(err, gobreaker.ErrTooManyRequests)) {
			err = fmt.Errorf("%w: %w", ErrCircuitBreakerOpen, err)
		}
	}
	spanErr := err
	var upstreamStatus *statusError
	if response != nil && errors.As(err, &upstreamStatus) {
		err = nil
	}
	if response != nil && response.Body != nil {
		response.Body = &spanBody{ReadCloser: response.Body, response: response, spanErr: spanErr, span: span, finish: finish}
		return response, err
	}
	finish(spanErr, response)
	return response, err
}

func (r *ResilientTransport) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	retryableMethod := req.Method == http.MethodGet || req.Method == http.MethodHead || req.Method == http.MethodOptions
	policy := backoff.NewExponentialBackOff()
	policy.InitialInterval = r.retry.InitialInterval
	policy.MaxInterval = r.retry.MaxInterval
	policy.Multiplier = r.retry.Multiplier
	policy.MaxElapsedTime = r.retry.MaxElapsedTime
	if r.retry.Jitter {
		policy.RandomizationFactor = 0.5
	} else {
		policy.RandomizationFactor = 0
	}
	policy.Reset()
	started := time.Now()

	for attempt := 0; ; attempt++ {
		response, err := r.doAttempt(ctx, req)
		if !isRetryable(response, err) || !retryableMethod || attempt >= r.retry.MaxRetries {
			if isRetryable(response, err) && retryableMethod && attempt >= r.retry.MaxRetries {
				return response, fmt.Errorf("%w: %w", ErrRetryExhausted, retryError(response, err))
			}
			if response != nil && response.StatusCode >= http.StatusInternalServerError || response != nil && response.StatusCode == http.StatusTooManyRequests {
				return response, &statusError{status: response.StatusCode}
			}
			return response, err
		}

		delay := policy.NextBackOff()
		if response != nil && response.StatusCode == http.StatusTooManyRequests {
			if retryAfter := parseRetryAfter(response.Header.Get("Retry-After"), time.Now()); retryAfter > 0 {
				delay = retryAfter
			}
		}
		if delay == backoff.Stop || time.Since(started)+delay > r.retry.MaxElapsedTime {
			if response != nil && (response.StatusCode >= http.StatusInternalServerError || response.StatusCode == http.StatusTooManyRequests) {
				return response, fmt.Errorf("%w: %w", ErrRetryExhausted, &statusError{status: response.StatusCode})
			}
			return nil, fmt.Errorf("%w: %w", ErrRetryExhausted, err)
		}
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
			_ = response.Body.Close()
		}
		trace.SpanFromContext(ctx).AddEvent("upstream retry", trace.WithAttributes(attribute.Int("retry.attempt", attempt+1)))
		if err := waitContext(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func (r *ResilientTransport) doAttempt(ctx context.Context, req *http.Request) (*http.Response, error) {
	if r.limiter != nil {
		if err := r.limiter.Wait(ctx); err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if _, ok := ctx.Deadline(); ok {
				return nil, context.DeadlineExceeded
			}
			return nil, err
		}
	}
	var release func()
	if r.active != nil {
		select {
		case r.active <- struct{}{}:
			release = func() { <-r.active }
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	request := req.Clone(ctx)
	if req.GetBody != nil {
		body, err := req.GetBody()
		if err != nil {
			if release != nil {
				release()
			}
			return nil, err
		}
		request.Body = body
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(request.Header))
	response, err := r.base.Do(ctx, request)
	if err != nil || response == nil || response.Body == nil {
		if release != nil {
			release()
		}
		return response, err
	}
	if release != nil {
		response.Body = &releaseBody{ReadCloser: response.Body, release: release}
	}
	return response, nil
}

type spanBody struct {
	io.ReadCloser
	response *http.Response
	spanErr  error
	span     trace.Span
	finish   func(error, *http.Response)
	once     sync.Once
}

func (b *spanBody) Read(buffer []byte) (int, error) {
	n, err := b.ReadCloser.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) {
		b.spanErr = err
		b.span.SetStatus(codes.Error, "upstream response body read failed")
		b.span.RecordError(fmt.Errorf("upstream response body read failed"))
	}
	return n, err
}

func (b *spanBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { b.finish(b.spanErr, b.response) })
	return err
}

func breakerStateValue(state gobreaker.State) float64 {
	switch state {
	case gobreaker.StateOpen:
		return 1
	case gobreaker.StateHalfOpen:
		return 0.5
	default:
		return 0
	}
}

func upstreamFailureReason(err error, response *http.Response) string {
	if errors.Is(err, ErrCircuitBreakerOpen) {
		return "circuit_open"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if response != nil {
		switch {
		case response.StatusCode == http.StatusTooManyRequests:
			return "http_429"
		case response.StatusCode >= http.StatusInternalServerError:
			return "http_5xx"
		default:
			return "http_4xx"
		}
	}
	return "network"
}

func isRetryable(response *http.Response, err error) bool {
	if err != nil {
		return true
	}
	return response != nil && (response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= http.StatusInternalServerError)
}

func retryError(response *http.Response, err error) error {
	if response != nil {
		return &statusError{status: response.StatusCode}
	}
	return err
}

func parseRetryAfter(value string, now time.Time) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func newRateLimiter(value string) (*rate.Limiter, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("rateLimit must use N/s, N/m, or N/h")
	}
	count, err := strconv.Atoi(parts[0])
	if err != nil || count <= 0 {
		return nil, fmt.Errorf("rateLimit request count must be a positive integer")
	}
	period := time.Second
	switch parts[1] {
	case "s":
	case "m":
		period = time.Minute
	case "h":
		period = time.Hour
	default:
		return nil, fmt.Errorf("rateLimit duration must be s, m, or h")
	}
	return rate.NewLimiter(rate.Limit(float64(count)/period.Seconds()), count), nil
}
