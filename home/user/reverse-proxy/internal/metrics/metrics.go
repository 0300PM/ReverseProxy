package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	requestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "proxy_requests_total",
			Help: "Total number of HTTP requests processed by the reverse proxy.",
		},
		[]string{"method", "path", "backend", "status"},
	)

	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "proxy_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path", "backend"},
	)

	backendHealthy = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "backend_healthy",
			Help: "Whether a backend is healthy (1 = healthy, 0 = unhealthy).",
		},
		[]string{"backend"},
	)

	rateLimitRejected = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rate_limit_rejected_total",
			Help: "Total number of requests rejected by rate limiter.",
		},
	)
)

func RecordRequest(method, path, backend string, status int, duration time.Duration) {
	statusStr := strconv.Itoa(status)
	requestsTotal.WithLabelValues(method, path, backend, statusStr).Inc()
	requestDuration.WithLabelValues(method, path, backend).Observe(duration.Seconds())
}

func SetBackendHealth(backendURL string, healthy bool) {
	value := 0.0
	if healthy {
		value = 1.0
	}
	backendHealthy.WithLabelValues(backendURL).Set(value)
}

func RecordRateLimitReject() {
	rateLimitRejected.Inc()
}

func Handler() http.Handler {
	return promhttp.Handler()
}
