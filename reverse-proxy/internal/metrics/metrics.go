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
		prometheus.CounterOpts{Name: "proxy_requests_total", Help: "Total HTTP requests"},
		[]string{"method", "path", "backend", "status"},
	)
	requestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{Name: "proxy_request_duration_seconds", Help: "Request duration", Buckets: prometheus.DefBuckets},
		[]string{"method", "path", "backend"},
	)
	backendHealthy = promauto.NewGaugeVec(
		prometheus.GaugeOpts{Name: "backend_healthy", Help: "Backend health (1=up)"},
		[]string{"backend"},
	)
	rateLimitRejected = promauto.NewCounter(
		prometheus.CounterOpts{Name: "rate_limit_rejected_total", Help: "Rate limited requests"},
	)
)

func RecordRequest(method, path, backend string, status int, duration time.Duration) {
	requestsTotal.WithLabelValues(method, path, backend, strconv.Itoa(status)).Inc()
	requestDuration.WithLabelValues(method, path, backend).Observe(duration.Seconds())
}

func SetBackendHealth(backendURL string, healthy bool) {
	v := 0.0
	if healthy { v = 1.0 }
	backendHealthy.WithLabelValues(backendURL).Set(v)
}

func RecordRateLimitReject() {
	rateLimitRejected.Inc()
}

func Handler() http.Handler {
	return promhttp.Handler()
}
