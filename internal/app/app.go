package app

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"reverse-proxy/internal/admin"
	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/breaker"
	"reverse-proxy/internal/clientip"
	"reverse-proxy/internal/config"
	"reverse-proxy/internal/health"
	"reverse-proxy/internal/httpx"
	"reverse-proxy/internal/limiter"
	"reverse-proxy/internal/logging"
	"reverse-proxy/internal/metrics"
	"reverse-proxy/internal/proxy"
)

const limiterIdleTTL = 10 * time.Minute

type App struct {
	Handler   http.Handler
	Checker   *health.Checker
	Limiter   limiter.Limiter
	Transport *http.Transport
}

func New(cfg *config.Config, logger *slog.Logger) (*App, error) {
	transport := proxy.NewTransport(cfg.Timeouts)
	backends := make([]*balancer.Backend, len(cfg.Backends))
	for i, bc := range cfg.Backends {
		brk := breaker.New(
			cfg.CircuitBreaker.FailureThreshold,
			cfg.CircuitBreaker.OpenTimeout.Duration,
			cfg.CircuitBreaker.HalfOpenProbes,
		)
		backendURL := bc.URL
		brk.OnStateChange(func(s breaker.State) {
			open := s == breaker.Open
			metrics.SetBreakerOpen(backendURL, open)
			if open {
				logger.Warn("circuit breaker opened", "backend", backendURL)
			} else {
				logger.Info("circuit breaker closed", "backend", backendURL)
			}
		})
		b, err := balancer.New(bc.URL, bc.Weight, brk)
		if err != nil {
			return nil, err
		}
		metrics.SetBackendHealth(b.String(), true)
		backends[i] = b
	}

	picker, err := balancer.NewPicker(cfg.LoadBalancing.Strategy, backends)
	if err != nil {
		return nil, err
	}
	checker := health.New(
		backends,
		cfg.HealthCheck.Interval.Duration,
		cfg.HealthCheck.Timeout.Duration,
		cfg.HealthCheck.Path,
		cfg.HealthCheck.HealthyThreshold,
		cfg.HealthCheck.UnhealthyThreshold,
		logger,
	)
	resolver, err := clientip.New(cfg.RateLimit.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}

	var lim limiter.Limiter
	if cfg.Redis.Addr != "" {
		lim = limiter.NewRedis(
			cfg.Redis.Addr,
			os.Getenv(config.RedisSecretEnv),
			cfg.RateLimit.RequestsPerMinute,
			cfg.RateLimit.Burst,
		)
	} else {
		lim = limiter.NewMemory(cfg.RateLimit.RequestsPerMinute, cfg.RateLimit.Burst, limiterIdleTTL)
	}

	proxyHandler := proxy.New(picker, transport, logger, cfg.Retry.MaxAttempts)
	handler := buildHandler(logger, resolver, lim, proxyHandler)

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	}))
	mux.Handle("/metrics", metrics.Handler())
	if token := os.Getenv(config.AdminTokenEnv); token != "" {
		mux.Handle("/admin/", admin.New(backends, token, logger))
	} else {
		mux.Handle("/admin/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteError(w, r, http.StatusNotFound, "not_found", "admin API is disabled")
		}))
	}
	mux.Handle("/", handler)

	return &App{
		Handler:   mux,
		Checker:   checker,
		Limiter:   lim,
		Transport: transport,
	}, nil
}

func (a *App) Close() error {
	a.Checker.Close()
	a.Transport.CloseIdleConnections()
	return a.Limiter.Close()
}

func buildHandler(logger *slog.Logger, resolver *clientip.Resolver, lim limiter.Limiter, next http.Handler) http.Handler {
	h := logging.Access(logger, resolver)(limiter.NewMiddleware(lim, resolver).Wrap(next))
	return httpx.RequestID(h)
}
