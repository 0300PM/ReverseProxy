package metrics

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_requests_total",
			Help: "Total number of HTTP requests processed by the reverse proxy.",
		},
		[]string{"method", "route", "backend", "status"},
	)
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "proxy_request_duration_seconds",
			Help:    "Duration of HTTP requests processed by the reverse proxy.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "backend"},
	)
	InFlightRequests = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "proxy_inflight_requests",
			Help: "Number of requests currently being proxied.",
		},
	)
	BackendHealthy = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backend_healthy",
			Help: "Backend health status (1 = healthy, 0 = unhealthy).",
		},
		[]string{"backend"},
	)
	BackendRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "backend_requests_total",
			Help: "Total number of requests attempted against each backend.",
		},
		[]string{"backend"},
	)
	BackendErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "backend_errors_total",
			Help: "Total number of failed upstream requests per backend.",
		},
		[]string{"backend"},
	)
	BackendTimeoutsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "backend_timeouts_total",
			Help: "Total number of upstream requests that timed out per backend.",
		},
		[]string{"backend"},
	)
	BreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backend_circuit_breaker_open",
			Help: "Circuit breaker state per backend (1 = open, 0 = closed or half-open).",
		},
		[]string{"backend"},
	)
	HealthChecksTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "backend_healthchecks_total",
			Help: "Total number of health checks per backend and result.",
		},
		[]string{"backend", "result"},
	)
	RetriesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "proxy_retries_total",
			Help: "Total number of retried upstream requests.",
		},
	)
	RateLimitRejected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rate_limit_rejected_total",
			Help: "Total number of requests rejected by the rate limiter.",
		},
	)
	RateLimitErrors = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rate_limit_errors_total",
			Help: "Total number of rate limiter backend errors.",
		},
	)
)

func Route(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	rest := path[1:]
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return "/" + rest[:i]
	}
	return "/" + rest
}

func RecordRequest(method, route, backend string, status int, duration time.Duration) {
	RequestsTotal.WithLabelValues(method, route, backend, strconv.Itoa(status)).Inc()
	RequestDuration.WithLabelValues(method, route, backend).Observe(duration.Seconds())
}

func SetBackendHealth(backendURL string, healthy bool) {
	v := 0.0
	if healthy {
		v = 1.0
	}
	BackendHealthy.WithLabelValues(backendURL).Set(v)
}

func SetBreakerOpen(backendURL string, open bool) {
	v := 0.0
	if open {
		v = 1.0
	}
	BreakerState.WithLabelValues(backendURL).Set(v)
}

func Handler() http.Handler {
	return promhttp.Handler()
}
