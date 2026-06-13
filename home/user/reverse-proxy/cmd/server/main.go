package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"reverse-proxy/internal/admin"
	"reverse-proxy/internal/balancer"
	"reverse-proxy/internal/health"
	"reverse-proxy/internal/logger"
	"reverse-proxy/internal/metrics"
	"reverse-proxy/internal/proxy"
	"reverse-proxy/internal/ratelimit"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Port                int      `yaml:"port"`
	Backends            []string `yaml:"backends"`
	HealthCheckInterval string   `yaml:"health_check_interval"`
	HealthCheckPath     string   `yaml:"health_check_path"`
	RateLimit           struct {
		RequestsPerMinute int `yaml:"requests_per_minute"`
	} `yaml:"rate_limit"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func main() {
	cfg, err := loadConfig("config/config.yaml")
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Create backends
	var backends []*balancer.Backend
	for _, rawURL := range cfg.Backends {
		b, err := balancer.NewBackend(rawURL)
		if err != nil {
			log.Fatalf("failed to create backend %s: %v", rawURL, err)
		}
		backends = append(backends, b)
	}

	if len(backends) == 0 {
		log.Fatal("no backends configured")
	}

	// Round Robin balancer
	rr := balancer.NewRoundRobin(backends)

	// Health checker (configurable path + HEAD + only state changes + Prometheus)
	interval, err := time.ParseDuration(cfg.HealthCheckInterval)
	if err != nil {
		interval = 5 * time.Second
	}
	checker := health.NewChecker(backends, interval, cfg.HealthCheckPath)
	checker.Start()

	// Rate limiter (records rejects to Prometheus)
	rateLimiter := ratelimit.NewLimiter(cfg.RateLimit.RequestsPerMinute)

	// Reverse proxy (records detailed metrics)
	reverseProxy := proxy.NewReverseProxy(rr)

	// Middleware chain
	handler := logger.Logger(
		rateLimiter.Middleware(reverseProxy),
	)

	// Admin API
	adminAPI := admin.NewAdmin(backends)

	// Router
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.Handle("/metrics", promhttp.Handler())
	mux.Handle("/admin/", http.StripPrefix("/admin", adminAPI))

	port := cfg.Port
	if port == 0 {
		port = 8080
	}
	addr := fmt.Sprintf(":%d", port)

	log.Printf("Starting reverse proxy on %s", addr)
	log.Printf("Admin API: %s/admin/backends", addr)
	log.Printf("Metrics:   %s/metrics", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
