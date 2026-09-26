package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry      *prometheus.Registry
	requests      *prometheus.CounterVec
	requestTime   *prometheus.HistogramVec
	stepTime      *prometheus.HistogramVec
	upstreamError *prometheus.CounterVec
	cacheHits     *prometheus.CounterVec
	cacheMisses   *prometheus.CounterVec
	breakerState  *prometheus.GaugeVec
	inflight      *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
	metrics := &Metrics{registry: prometheus.NewRegistry()}
	metrics.requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "elegba_requests_total", Help: "Total requests handled by Elegba.",
	}, []string{"endpoint", "method", "status"})
	metrics.requestTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "elegba_request_duration_seconds", Help: "Request duration in seconds.",
	}, []string{"endpoint"})
	metrics.stepTime = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name: "elegba_step_duration_seconds", Help: "Pipeline step duration in seconds.",
	}, []string{"endpoint", "step", "upstream"})
	metrics.upstreamError = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "elegba_upstream_errors_total", Help: "Upstream request failures by reason.",
	}, []string{"upstream", "reason"})
	metrics.cacheHits = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "elegba_cache_hits_total", Help: "Fetch cache hits by backend.",
	}, []string{"backend"})
	metrics.cacheMisses = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "elegba_cache_misses_total", Help: "Fetch cache misses by backend.",
	}, []string{"backend"})
	metrics.breakerState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "elegba_circuit_breaker_state", Help: "Circuit state: 0 closed, 0.5 half-open, 1 open.",
	}, []string{"upstream"})
	metrics.inflight = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "elegba_inflight_requests", Help: "Currently executing requests by endpoint.",
	}, []string{"endpoint"})
	metrics.registry.MustRegister(
		metrics.requests,
		metrics.requestTime,
		metrics.stepTime,
		metrics.upstreamError,
		metrics.cacheHits,
		metrics.cacheMisses,
		metrics.breakerState,
		metrics.inflight,
	)
	return metrics
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

func (m *Metrics) ObserveRequest(endpoint, method string, status int, duration time.Duration) {
	m.requests.WithLabelValues(endpoint, method, strconv.Itoa(status)).Inc()
	m.requestTime.WithLabelValues(endpoint).Observe(duration.Seconds())
}

func (m *Metrics) ObserveStep(endpoint, step, upstream string, duration time.Duration) {
	m.stepTime.WithLabelValues(endpoint, step, upstream).Observe(duration.Seconds())
}

func (m *Metrics) ObserveUpstreamError(upstream, reason string) {
	m.upstreamError.WithLabelValues(upstream, reason).Inc()
}

func (m *Metrics) ObserveCache(backend string, hit bool) {
	if hit {
		m.cacheHits.WithLabelValues(backend).Inc()
		return
	}
	m.cacheMisses.WithLabelValues(backend).Inc()
}

func (m *Metrics) SetBreakerState(upstream string, state float64) {
	m.breakerState.WithLabelValues(upstream).Set(state)
}

func (m *Metrics) AddInflight(endpoint string, delta float64) {
	m.inflight.WithLabelValues(endpoint).Add(delta)
}
